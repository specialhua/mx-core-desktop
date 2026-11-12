package main

import (
	"context"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestNormalizeRoot(t *testing.T) {
	for in, want := range map[string]string{
		"server.example.com":                          "https://server.example.com",
		"https://server.example.com/":                 "https://server.example.com",
		"https://server.example.com/api/v3":           "https://server.example.com",
		"https://server.example.com/proxy/qaqdmin/#/": "https://server.example.com",
		"http://localhost:2333":                       "http://localhost:2333",
		"https://example.com/mx/api/v3/auth":          "https://example.com/mx",
	} {
		got, err := normalizeRoot(in)
		if err != nil || got != want {
			t.Errorf("normalizeRoot(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "ftp://x", "https://"} {
		if _, err := normalizeRoot(bad); err == nil {
			t.Errorf("normalizeRoot(%q) succeeded", bad)
		}
	}
}

func TestActiveNavPath(t *testing.T) {
	for route, want := range map[string]string{
		"/dashboard":            "/dashboard",
		"/posts?page=2":         "/posts",
		"/posts/edit?id=1":      "/posts/edit",
		"/posts/category/123":   "/posts/category",
		"/comments/42":          "/comments",
		"/setting/user":         "/setting",
		"/files/comment-images": "/files/comment-images",
		"/maintenance/cron/abc": "/maintenance/cron",
		"/drafts/organize":      "/drafts",
		"/login":                "",
		"/postsx":               "",
	} {
		if got := activeNavPath(route); got != want {
			t.Errorf("activeNavPath(%q) = %q; want %q", route, got, want)
		}
	}
}

// TestNavIcons checks that every generated item has its icon.
func TestNavIcons(t *testing.T) {
	var walk func([]navItem)
	walk = func(items []navItem) {
		for _, it := range items {
			if it.Icon != "" {
				if navIcon(it.Icon) == nil {
					t.Errorf("%s: icon %s does not parse", it.Path, it.Icon)
				}
				data, _ := iconFS.ReadFile("icons/" + it.Icon + ".svg")
				if !strings.Contains(string(data), "/></svg>") {
					t.Errorf("%s: icon %s has no shapes", it.Path, it.Icon)
				}
			}
			walk(it.Children)
		}
	}
	for _, s := range navSections {
		walk(s.Items)
	}
}

func TestPreloadScript(t *testing.T) {
	s := preloadScript(`https://a.example"`, "/proxy/qaqdmin", "zh-CN")
	if !strings.Contains(s, `location.origin !== "https://a.example\""`) {
		t.Errorf("origin not quoted as JSON:\n%s", s)
	}
	if !strings.Contains(s, `!== "/proxy/qaqdmin"`) {
		t.Error("the script does not check the admin's path")
	}
	if !strings.Contains(s, `"Bridge.Route"`) {
		t.Error("the script does not report routes")
	}
}

// fakeServer answers the device authorization as Better Auth does: pending
// twice, then a token.
func fakeServer(t *testing.T) *httptest.Server {
	var polls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v3/auth/ok", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	})
	// As pageproxy.controller.ts: the admin has no trailing slash.
	mux.HandleFunc("GET /proxy/qaqdmin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<!doctype html><title>admin</title>`))
	})
	mux.HandleFunc("GET /proxy/qaqdmin/", http.NotFound)
	mux.HandleFunc("POST /api/v3/auth/device/code", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["client_id"] != deviceClientID {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"invalid_client"}`))
			return
		}
		w.Write([]byte(`{"device_code":"dev","user_code":"ABCD-EFGH","verification_uri":"/api/v3/device","verification_uri_complete":"/api/v3/device?user_code=ABCD-EFGH","expires_in":60,"interval":1}`))
	})
	mux.HandleFunc("POST /api/v3/auth/device/token", func(w http.ResponseWriter, r *http.Request) {
		if polls.Add(1) < 3 {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"authorization_pending","error_description":"pending"}`))
			return
		}
		w.Write([]byte(`{"access_token":"tok","token_type":"Bearer","expires_in":3600}`))
	})
	mux.HandleFunc("GET /api/v3/auth/one-time-token/generate", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			w.Write([]byte(`{"error":{"code":"AUTH_NOT_LOGGED_IN","message":"not logged in"}}`))
			return
		}
		w.Write([]byte(`{"token":"ott"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestProbeWithoutAdmin(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v3/auth/ok", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	_, err := probeServer(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "no web admin") {
		t.Errorf("probeServer without an admin: %v", err)
	}
}

func TestDeviceFlow(t *testing.T) {
	fs := fakeServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	s, err := probeServer(ctx, fs.URL)
	if err != nil {
		t.Fatal(err)
	}
	if s.API != fs.URL+"/api/v3" || s.Admin() != fs.URL+"/proxy/qaqdmin" {
		t.Fatalf("server = %+v", s)
	}
	code, err := requestDeviceCode(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if got := code.verificationURL(s); got != fs.URL+"/api/v3/device?user_code=ABCD-EFGH" {
		t.Errorf("verificationURL = %q", got)
	}
	token, err := pollDeviceToken(ctx, s, code)
	if err != nil || token != "tok" {
		t.Fatalf("pollDeviceToken = %q, %v", token, err)
	}
	url, err := handoffURL(ctx, s, token)
	if err != nil || url != fs.URL+"/api/v3/device/web-handoff?target=admin&token=ott" {
		t.Errorf("handoffURL = %q, %v", url, err)
	}
	if _, err := handoffURL(ctx, s, "stale"); !isUnauthorized(err) {
		t.Errorf("a stale token: %v, want 401", err)
	}
}

// TestScreens draws the screens without a window, and saves them to
// $SCREENSHOTS when set, to look at.
func TestScreens(t *testing.T) {
	shot := func(name string, tt *ui.Tester) {
		dir := os.Getenv("SCREENSHOTS")
		if dir == "" {
			return
		}
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		png.Encode(f, tt.Image())
	}

	a := &app{set: settings{Lang: "zh-CN", Appearance: "system"}}
	if a.pageKind() != pageSetup {
		t.Errorf("page = %q, want setup", a.pageKind())
	}
	tt := ui.NewTester(a.pageView, 1100, 700)
	if !tt.HasText("连接你的空间") {
		t.Errorf("setup: %q", tt.Texts())
	}
	shot("setup", tt)

	a.srv = server{Root: "https://server.example.com", API: "https://server.example.com/api/v3"}
	a.web = nil
	a.route = "/login?from=%2Fdashboard"
	tt = ui.NewTester(a.loginView, 1100, 700)
	if !tt.HasText("使用 Passkey 登录") || !tt.HasText("使用密码登录") {
		t.Errorf("login: %q", tt.Texts())
	}
	shot("login", tt)

	a.login.code = &deviceCode{UserCode: "ABCD-EFGH"}
	tt = ui.NewTester(a.loginView, 1100, 700)
	if !tt.HasText("ABCD-EFGH") {
		t.Errorf("login code: %q", tt.Texts())
	}
	shot("login-code", tt)

	a.routeChanged("/posts/edit?id=1")
	if a.nav != "/posts/edit" {
		t.Errorf("nav = %q", a.nav)
	}
	if len(a.toasts) != 1 || a.toasts[0].Title != "已登录" {
		t.Errorf("signing in from /login: toasts = %+v", a.toasts)
	}
	tt = ui.NewTester(a.sidebarView, 260, 900)
	for _, want := range []string{"仪表盘", "内容", "博文", "写博文", "分类", "手记", "系统", "设置"} {
		if !tt.HasText(want) {
			t.Errorf("sidebar lacks %q: %q", want, tt.Texts())
		}
	}
	if tt.HasText("写手记") {
		t.Error("a branch that is not active is expanded")
	}
	shot("sidebar", tt)

	tt = ui.NewTester(a.contentTitle, 600, 48)
	if !tt.HasText("博文") || !tt.HasText("写博文") {
		t.Errorf("title: %q", tt.Texts())
	}

	a.page = pageMain
	a.openPalette()
	tt = ui.NewTester(a.contentView, 1100, 700)
	if !tt.HasText("前往页面或操作") || !tt.HasText("写博文") {
		t.Errorf("palette: %q", tt.Texts())
	}
	shot("palette", tt)

	tt = ui.NewTester(a.prefsView, 720, 520)
	for _, want := range []string{"通用", "外观", "账户", "https://server.example.com", "更换服务器…"} {
		if !tt.HasText(want) {
			t.Errorf("settings lacks %q: %q", want, tt.Texts())
		}
	}
	shot("settings", tt)
}

func TestPageKind(t *testing.T) {
	a := &app{}
	if got := a.pageKind(); got != pageSetup {
		t.Errorf("no server: %q", got)
	}
	a.srv = server{Root: "https://s.example"}
	if got := a.pageKind(); got != pageConnecting {
		t.Errorf("no web view: %q", got)
	}
}

func TestNavTrail(t *testing.T) {
	trail := navTrail("/posts/edit")
	if len(trail) != 2 || trail[0].Path != "/posts" || trail[1].Path != "/posts/edit" {
		t.Errorf("navTrail(/posts/edit) = %+v", trail)
	}
	if navTrail("") != nil || navTrail("/nope") != nil {
		t.Error("navTrail of no item")
	}
}

func TestPaletteItems(t *testing.T) {
	a := &app{set: settings{Lang: "en-US"}}
	ids := map[string]bool{}
	for _, it := range a.paletteItems() {
		if it.ID == "" || ids[it.ID] {
			t.Errorf("empty or duplicate ID %q", it.ID)
		}
		ids[it.ID] = true
	}
	for _, want := range []string{"nav:/dashboard", "nav:/posts/edit", "act:prefs", "act:sign-out", "act:lang-zh"} {
		if !ids[want] {
			t.Errorf("palette lacks %s", want)
		}
	}
	a.runCommand("nav:/comments")
	if a.nav != "/comments" {
		t.Errorf("nav after the palette = %q", a.nav)
	}
}

// TestReadmeScreenshots renders the README's screenshots, at 2x, into
// $README_SCREENSHOTS (docs/screenshots):
//
//	README_SCREENSHOTS=docs/screenshots go test -run TestReadmeScreenshots
//
// They are headless renders of the native views: the web admin, which the
// web view shows, is left out.
func TestReadmeScreenshots(t *testing.T) {
	dir := os.Getenv("README_SCREENSHOTS")
	if dir == "" {
		t.Skip("README_SCREENSHOTS is not set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	shot := func(name string, view func(*ui.Context), w, h int, dark bool) {
		tt := ui.NewTester(view, w, h)
		tt.SetScale(2)
		tt.SetDark(dark)
		for i := 0; i < 10; i++ {
			tt.Advance(100 * time.Millisecond)
			tt.Frame()
		}
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}

	a := &app{set: settings{Lang: "zh-CN", Appearance: "system"}}
	shot("setup", a.pageView, 900, 560, false)

	a.srv = server{Root: "https://server.example.com", API: "https://server.example.com/api/v3"}
	a.route = "/login"
	a.login.code = &deviceCode{UserCode: "ABCD-EFGH"}
	shot("login", a.loginView, 900, 560, false)

	// The sidebar beside the content area, as the shell lays them out, with
	// the command palette open over them.
	a.login = loginState{}
	a.routeChanged("/posts/edit?id=1")
	a.toasts = nil
	a.page = pageMain
	main := func(c *ui.Context) {
		th := c.Theme()
		ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Width(220).Shrink(0).Background(th.Surface).Children(func() { a.sidebarView(c) })
			ui.Box(c).Width(1).Background(th.Border)
			ui.Column(c).Grow(1).MinWidth(0).Children(func() { a.contentView(c) })
		})
	}
	a.openPalette()
	a.palette.Query = "写"
	shot("palette", main, 1100, 700, false)
	a.paletteOpen = false

	shot("settings", a.prefsView, 720, 480, false)
	a.prefsState.Category = "appearance"
	shot("settings-dark", a.prefsView, 720, 480, true)
}
