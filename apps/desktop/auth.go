package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

// Passkey sign-in goes through the system browser, where passkeys work, with
// the RFC 8628 device authorization the server already offers the mxs CLI
// and the iOS app:
//
//  1. POST /auth/device/code gives a user code and a page to approve it;
//  2. the browser opens that page, where the owner signs in (passkey,
//     password, OAuth: whatever the web admin offers) and approves;
//  3. POST /auth/device/token, polled, gives a session token, which the
//     Keychain keeps;
//  4. GET /auth/one-time-token/generate, with the token, gives a one-time
//     token that /device/web-handoff trades for the web admin's session
//     cookie, in the app's web view.
//
// A web view cannot sign in with a passkey itself: WebKit offers passkeys
// only to apps with the associated domains entitlement of the site.

// deviceClientID is the device authorization client the app signs in as.
// The server accepts only the clients it lists (auth.constant.ts); the app
// borrows the mxs CLI's until the server lists one for it, so the approval
// page names the request "Mix Space CLI".
const deviceClientID = "mxs-cli"

const tokenSecret = "session-token"

type deviceCode struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

func requestDeviceCode(ctx context.Context, s server) (*deviceCode, error) {
	var code deviceCode
	err := doJSON(ctx, http.MethodPost, s.Auth()+"/device/code", "", map[string]string{
		"client_id": deviceClientID,
		"scope":     "openid profile email",
	}, &code)
	if err != nil {
		return nil, err
	}
	if code.DeviceCode == "" || code.UserCode == "" {
		return nil, errors.New("the server sent no device code")
	}
	return &code, nil
}

// verificationURL is the approval page, absolute: the server may give a
// path relative to itself.
func (c *deviceCode) verificationURL(s server) string {
	raw := c.VerificationURIComplete
	if raw == "" {
		raw = c.VerificationURI + "?user_code=" + url.QueryEscape(c.UserCode)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.IsAbs() {
		return raw
	}
	base, err := url.Parse(s.Origin() + "/")
	if err != nil {
		return raw
	}
	return base.ResolveReference(u).String()
}

var (
	errDenied  = errors.New("the request was denied")
	errExpired = errors.New("the code expired")
)

// pollDeviceToken waits for the approval and returns the session token.
func pollDeviceToken(ctx context.Context, s server, code *deviceCode) (string, error) {
	interval := time.Duration(max(code.Interval, 1)) * time.Second
	deadline := time.Now().Add(time.Duration(max(code.ExpiresIn, 60)) * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}
		var tok struct {
			AccessToken string `json:"access_token"`
		}
		err := doJSON(ctx, http.MethodPost, s.Auth()+"/device/token", "", map[string]string{
			"grant_type":  "urn:ietf:params:oauth:grant-type:device_code",
			"device_code": code.DeviceCode,
			"client_id":   deviceClientID,
		}, &tok)
		if err == nil && tok.AccessToken != "" {
			return tok.AccessToken, nil
		}
		var ae *apiError
		if !errors.As(err, &ae) {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			continue // a network error: try again
		}
		switch ae.Code {
		case "authorization_pending":
		case "slow_down":
			interval += 5 * time.Second
		case "access_denied":
			return "", errDenied
		case "expired_token":
			return "", errExpired
		default:
			return "", ae
		}
	}
	return "", errExpired
}

// handoffURL trades the session token for a one-time token and returns the
// page that sets the web admin's cookie and opens it. An *apiError of
// status 401 means the token is no longer valid.
func handoffURL(ctx context.Context, s server, token string) (string, error) {
	var ott struct {
		Token string `json:"token"`
	}
	if err := doJSON(ctx, http.MethodGet, s.Auth()+"/one-time-token/generate", token, nil, &ott); err != nil {
		return "", err
	}
	if ott.Token == "" {
		return "", errors.New("the server sent no one-time token")
	}
	q := url.Values{"token": {ott.Token}, "target": {"admin"}}
	return s.API + "/device/web-handoff?" + q.Encode(), nil
}

func isUnauthorized(err error) bool {
	var ae *apiError
	return errors.As(err, &ae) && (ae.Status == http.StatusUnauthorized || ae.Status == http.StatusForbidden)
}

func loadToken() string {
	b, err := mygo.Secrets.Get(tokenSecret)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func saveToken(token string) error { return mygo.Secrets.Set(tokenSecret, []byte(token)) }

func deleteToken() { _ = mygo.Secrets.Delete(tokenSecret) }

// revokeToken ends the session the token stands for, on sign out.
func revokeToken(s server, token string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = doJSON(ctx, http.MethodPost, s.Auth()+"/sign-out", token, map[string]any{}, nil)
}
