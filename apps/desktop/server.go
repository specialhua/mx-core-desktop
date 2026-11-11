package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// server is a Mix Space server, resolved from the root URL the user gave.
type server struct {
	// Root is the server's origin and base path, e.g. https://server.example.com.
	Root string
	// API is where the API is served: Root + "/api/v3" in production, Root
	// in development, where the API has no prefix.
	API string
}

// Auth is Better Auth's base, under the API.
func (s server) Auth() string { return s.API + "/auth" }

// Admin is the web admin the server serves (pageproxy.controller.ts). The
// route has no trailing slash: /proxy/qaqdmin/ is a 404.
func (s server) Admin() string { return s.Root + "/proxy/qaqdmin" }

// AdminPath is the path of Admin, which the preload script checks.
func (s server) AdminPath() string {
	u, err := url.Parse(s.Admin())
	if err != nil {
		return ""
	}
	return u.Path
}

// AdminRoute is the web admin at a hash route, e.g. "/dashboard".
func (s server) AdminRoute(route string) string { return s.Admin() + "#" + route }

// Origin is the scheme and host the web admin's pages are served from.
func (s server) Origin() string {
	u, err := url.Parse(s.Root)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

var httpClient = &http.Client{Timeout: 20 * time.Second}

// normalizeRoot turns what the user typed into a root URL: a scheme is
// added when missing, and a pasted API or admin path is dropped.
func normalizeRoot(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("empty")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("invalid URL %q", raw)
	}
	p := u.Path
	for _, suffix := range []string{"/proxy/qaqdmin", "/api/v3"} {
		if i := strings.Index(p, suffix); i >= 0 {
			p = p[:i]
		}
	}
	u.Path = strings.TrimRight(p, "/")
	u.RawQuery, u.Fragment = "", ""
	return u.String(), nil
}

// probeServer finds where the server answers, as the mxs CLI does: Better
// Auth's /ok under /api/v3/auth in production, or /auth in development.
func probeServer(ctx context.Context, raw string) (server, error) {
	root, err := normalizeRoot(raw)
	if err != nil {
		return server{}, err
	}
	var lastErr error
	for _, api := range []string{root + "/api/v3", root} {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, api+"/auth/ok", nil)
		req.Header.Set("Accept", "application/json")
		res, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		io.Copy(io.Discard, io.LimitReader(res.Body, 1<<16))
		res.Body.Close()
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			s := server{Root: root, API: api}
			return s, checkAdmin(ctx, s)
		}
		lastErr = fmt.Errorf("%s/auth/ok: HTTP %d", api, res.StatusCode)
	}
	return server{}, lastErr
}

// checkAdmin makes sure the server serves the web admin, which the app
// shows: a server whose admin is missing would show a blank page.
func checkAdmin(ctx context.Context, s server) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.Admin(), nil)
	req.Header.Set("Accept", "text/html")
	res, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(res.Header.Get("Content-Type"), "html") {
		return fmt.Errorf("the server has no web admin at %s (HTTP %d)", s.Admin(), res.StatusCode)
	}
	return nil
}

// apiError is a non-2xx answer of the server, with its message when the
// body is Mix Space's error envelope or an OAuth error.
type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e *apiError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("HTTP %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("HTTP %d", e.Status)
}

// doJSON sends body as JSON (nil for none) and decodes a 2xx answer into
// out. A bearer token, when given, authenticates the request.
func doJSON(ctx context.Context, method, url, bearer string, body, out any) error {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = strings.NewReader(string(data))
	}
	req, err := http.NewRequestWithContext(ctx, method, url, r)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		e := &apiError{Status: res.StatusCode}
		var env struct {
			Error            json.RawMessage `json:"error"`
			ErrorDescription string          `json:"error_description"`
			Message          string          `json:"message"`
		}
		if json.Unmarshal(data, &env) == nil {
			var inner struct{ Code, Message string }
			if json.Unmarshal(env.Error, &inner) == nil && inner.Code != "" {
				e.Code, e.Message = inner.Code, inner.Message
			} else {
				var code string
				_ = json.Unmarshal(env.Error, &code)
				e.Code = code
				e.Message = env.ErrorDescription
			}
			if e.Message == "" {
				e.Message = env.Message
			}
		}
		return e
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}
