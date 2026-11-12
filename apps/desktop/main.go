// Mix Space is the desktop client of the Mix Space admin: a native window,
// sidebar and menus around the web admin the server serves, which shows in
// a web view. It adds no features of its own; everything past the sidebar is
// the web admin, so it follows the server's version without a new release.
package main

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/components/command"
	"github.com/oneo-me/onevra/source/shells/sidebar"
)

// app is the state the window shows. The view and the methods called from
// it run on the main thread; goroutines change the state through
// win.Update.
type app struct {
	shell *sidebar.Shell
	win   *mygo.Window
	web   *mygo.WebView
	set   settings
	srv   server

	// route is the web admin's hash route, as the bridge reported it; ""
	// until the first page reported one.
	route string
	// nav is the sidebar item chosen: the path of the item route is under.
	nav string

	// page is the page the shell shows (pageKind), as last set.
	page string

	// The setup screen.
	serverInput string
	probing     bool

	// The sign-in screen.
	login loginState

	// loadErr is why the web admin's page failed to load, e.g. offline.
	loadErr string
	// handingOff is set while the web view loads /device/web-handoff.
	handingOff bool

	// toasts wait for the main window's next frame (notify).
	toasts []ui.Toast

	// The command palette (palette.go).
	paletteOpen bool
	palette     command.State

	// The settings window (prefs.go), nil while closed.
	prefs      *mygo.Window
	prefsState components.SettingsState
}

type loginState struct {
	busy     bool
	code     *deviceCode
	url      string
	cancel   context.CancelFunc
	password bool // the user chose the web admin's own password form
	// tried is set once the Keychain's token was traded for a session, or
	// after a sign out, so that a stale token is not tried in a loop.
	tried bool
}

// tr picks the string of the app's language.
func (a *app) tr(zh, en string) string {
	if a.set.Lang == "en-US" {
		return en
	}
	return zh
}

func main() {
	a := &app{set: loadSettings()}
	a.serverInput = a.set.Server
	mygo.Theme.SetSource(themeSource(a.set.Appearance))
	mygo.Bind(&Bridge{a})

	mygo.App.WhenReady(func() {
		a.shell = sidebar.NewShell(sidebar.ShellOptions{
			Title:        "Mix Space",
			Width:        1360,
			Height:       880,
			MinWidth:     900,
			MinHeight:    560,
			StateKey:     "main",
			LeftWidth:    220,
			LeftMinWidth: 180,
			LeftMaxWidth: 320,
			Left:         a.sidebarView,
			LeftToolbar:  a.sidebarToolbar,
			Content:      a.contentView,
			ContentTitle: a.contentTitle,
			// Not a palette: the app keeps MyGo's theme, closer to the web
			// admin's than Onevra's. The hook switches the page.
			Theme: a.beginFrame,
		})
		a.win = a.shell.Window()
		a.buildMenu()
		if a.set.Server != "" {
			a.connect(a.set.Server, false)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// connect resolves the server and opens the web admin. Called on the main
// thread; the network goes on a goroutine.
func (a *app) connect(raw string, save bool) {
	a.probing = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		srv, err := probeServer(ctx, raw)
		a.win.Update(func() {
			a.probing = false
			if err != nil {
				a.notify(components.ToastError, "connect", a.tr("无法连接服务器", "Cannot reach the server"), err.Error())
				if !save {
					// A saved server that is down: keep it, let the user retry or change it.
					a.srv = server{}
				}
				return
			}
			a.srv = srv
			if save {
				a.set.Server = srv.Root
				_ = a.set.save()
			}
			a.openWeb(srv.AdminRoute("/dashboard"))
		})
	}()
}

// openWeb (re)creates the web view for the current server and language,
// which the preload script and the trusted origin depend on.
func (a *app) openWeb(url string) {
	if a.web != nil {
		a.web.Destroy()
		a.web = nil
	}
	a.route, a.nav = "", ""
	web, err := a.win.NewWebView(mygo.WebViewOptions{
		URL: url,
		Page: mygo.PageOptions{
			PreloadScript:  preloadScript(a.srv.Origin(), a.srv.AdminPath(), a.set.Lang),
			TrustedOrigins: []string{a.srv.Origin()},
			// Keep the page's scripts running while the window is hidden, for
			// the admin's WebSocket notifications.
			BackgroundThrottling: mygo.ThrottlingOff,
		},
	})
	if err != nil {
		a.notify(components.ToastError, "connect", a.tr("无法打开后台", "Cannot open the admin"), err.Error())
		return
	}
	a.web = web
	a.loadErr = ""
	page := web.Page()
	page.OnDidFailLoad(func(err *mygo.LoadError) {
		a.win.Update(func() {
			if a.web == web {
				a.loadErr = err.Description
			}
		})
	})
	page.OnDidFinishLoad(func() {
		a.win.Update(func() {
			if a.web == web {
				a.loadErr = ""
			}
		})
	})
	// /device/web-handoff sets the cookie and redirects to the adminUrl the
	// server is configured with, which may not be the admin the app shows
	// (or may be a 404): once it has landed, show the app's own.
	page.OnDidNavigate(func(url string) {
		a.win.Update(func() {
			if a.web != web || !a.handingOff {
				return
			}
			a.handingOff = false
			if url != a.srv.Admin() && !strings.HasPrefix(url, a.srv.Admin()+"#") {
				_ = page.LoadURL(a.srv.AdminRoute("/dashboard"))
			}
		})
	})
	// The web content process crashed or was killed: bring the page back.
	page.OnRenderProcessGone(func(string) { page.Reload() })
	a.win.Invalidate()
}

// routeChanged follows the web admin's navigation.
func (a *app) routeChanged(route string) {
	prev := a.route
	a.route = route
	a.nav = activeNavPath(route)
	if strings.HasPrefix(route, "/login") {
		// Signed out, or the session expired: try the Keychain's token once.
		if token := loadToken(); token != "" && !a.login.tried && !a.login.busy {
			a.login.tried = true
			a.handoff(token)
		}
		return
	}
	if !isPublicRoute(route) {
		// Signed in.
		a.login = loginState{tried: a.login.tried}
		if strings.HasPrefix(prev, "/login") {
			a.notify(components.ToastSuccess, "login", a.tr("已登录", "Signed in"), "")
		}
	}
}

func isPublicRoute(route string) bool {
	for _, p := range []string{"/login", "/setup", "/collab"} {
		if route == p || strings.HasPrefix(route, p+"/") || strings.HasPrefix(route, p+"?") {
			return true
		}
	}
	return false
}

// navigate shows a page of the web admin, as its own sidebar would.
func (a *app) navigate(path string) {
	if a.web == nil {
		return
	}
	p, _ := json.Marshal("#" + path)
	page := a.web.Page()
	go page.Eval("location.hash = " + string(p))
}

// startPasskeyLogin runs the device authorization in the system browser.
func (a *app) startPasskeyLogin() {
	if a.login.cancel != nil {
		a.login.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.login = loginState{busy: true, cancel: cancel, tried: true}
	srv := a.srv
	go func() {
		code, err := requestDeviceCode(ctx, srv)
		if err != nil {
			a.win.Update(func() { a.loginFailed(err) })
			return
		}
		verify := code.verificationURL(srv)
		a.win.Update(func() { a.login.code, a.login.url = code, verify })
		_ = mygo.Shell.OpenExternal(verify)

		token, err := pollDeviceToken(ctx, srv, code)
		if err != nil {
			if ctx.Err() == nil {
				a.win.Update(func() { a.loginFailed(err) })
			}
			return
		}
		if err := saveToken(token); err != nil {
			log.Printf("keychain: %v", err)
		}
		a.win.Update(func() { a.handoff(token) })
	}()
}

func (a *app) cancelLogin() {
	if a.login.cancel != nil {
		a.login.cancel()
	}
	a.login = loginState{tried: true}
}

func (a *app) loginFailed(err error) {
	msg := err.Error()
	switch err {
	case errDenied:
		msg = a.tr("已在浏览器中拒绝了这次登录。", "The sign-in was denied in the browser.")
	case errExpired:
		msg = a.tr("验证码已过期，请重新登录。", "The code expired. Sign in again.")
	}
	a.login = loginState{tried: true}
	a.notify(components.ToastError, "login", a.tr("登录失败", "Sign-in failed"), msg)
	mygo.App.Focus()
}

// handoff trades the session token for the web admin's cookie, in the web
// view. A token the server no longer accepts is forgotten.
func (a *app) handoff(token string) {
	a.login.busy = true
	srv := a.srv
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		url, err := handoffURL(ctx, srv, token)
		a.win.Update(func() {
			a.login.busy, a.login.code = false, nil
			if err != nil {
				if isUnauthorized(err) {
					deleteToken()
					a.notify(components.ToastError, "login", a.tr("登录已过期", "The session expired"), a.tr("请重新登录。", "Sign in again."))
				} else {
					a.notify(components.ToastError, "login", a.tr("登录失败", "Sign-in failed"), err.Error())
				}
				return
			}
			mygo.App.Focus()
			if a.web != nil {
				a.handingOff = true
				_ = a.web.Page().LoadURL(url)
			}
		})
	}()
}

// signOut ends the web admin's session and the Keychain's.
func (a *app) signOut() {
	a.login = loginState{tried: true}
	a.notify(components.ToastInfo, "login", a.tr("已退出登录", "Signed out"), "")
	srv, web := a.srv, a.web
	token := loadToken()
	deleteToken()
	go func() {
		if token != "" {
			revokeToken(srv, token)
		}
		if web == nil {
			return
		}
		api, _ := json.Marshal(srv.API)
		admin, _ := json.Marshal(srv.AdminRoute("/login"))
		_, _ = web.Page().Eval(`
await fetch(` + string(api) + ` + "/auth/sign-out", {method: "POST", credentials: "include", headers: {"content-type": "application/json"}, body: "{}"}).catch(() => {});
try { sessionStorage.removeItem("session-with-login"); } catch {}
location.replace(` + string(admin) + `);
location.reload();`)
	}()
}

// changeServer forgets the server and its session and shows the setup.
func (a *app) changeServer() {
	if token := loadToken(); token != "" && a.srv.API != "" {
		go revokeToken(a.srv, token)
	}
	deleteToken()
	if a.web != nil {
		a.web.Destroy()
		a.web = nil
	}
	a.cancelLogin()
	a.login.tried = false
	a.srv, a.route, a.nav = server{}, "", ""
	a.serverInput = a.set.Server
	a.set.Server = ""
	_ = a.set.save()
	a.win.Invalidate()
	a.invalidatePrefs()
}

func (a *app) openMainSite() {
	if a.web == nil {
		return
	}
	page := a.web.Page()
	go func() {
		url, err := mygo.EvalAs[string](page, `return (window.injectData && window.injectData.WEB_URL) || ""`)
		if err == nil && strings.HasPrefix(url, "http") {
			_ = mygo.Shell.OpenExternal(url)
		}
	}()
}

func (a *app) setAppearance(v string) {
	a.set.Appearance = v
	_ = a.set.save()
	mygo.Theme.SetSource(themeSource(v))
	a.invalidatePrefs()
}

func (a *app) setLang(v string) {
	if a.set.Lang == v {
		return
	}
	a.set.Lang = v
	_ = a.set.save()
	a.buildMenu()
	a.invalidatePrefs()
	if a.web != nil {
		route := a.route
		if route == "" {
			route = "/dashboard"
		}
		a.openWeb(a.srv.AdminRoute(route))
	}
}
