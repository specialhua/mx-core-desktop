package main

import (
	"context"
	"encoding/json"
	"strings"
)

// Bridge is what the web admin's page calls in Go, through the preload
// script below. Its pages may call it because the window trusts the
// server's origin (PageOptions.TrustedOrigins); keep it to what the script
// needs.
type Bridge struct{ a *app }

// Route tells the app the web admin's hash route, as "/posts?page=2", after
// every navigation, so the native sidebar follows the page.
func (b *Bridge) Route(ctx context.Context, route string) {
	if len(route) > 2048 {
		return
	}
	b.a.win.Update(func() { b.a.routeChanged(route) })
}

// preloadScript runs before the web admin's own scripts, on its page. It
//   - collapses the web admin's sidebar, which the native one replaces, and
//     hides the buttons that would bring it back;
//   - lets the theme follow the app (prefers-color-scheme follows
//     mygo.Theme) and sets the locale the app uses;
//   - reports the hash route: React Router's HashRouter changes it with
//     history.pushState, which fires no hashchange.
//
// It reads the web admin's storage keys (apps/admin: SIDEBAR_COLLAPSED_STORAGE_KEY,
// the theme store's "theme-mode", UI_LOCALE_STORAGE_KEY) and the aria-label
// of its two sidebar buttons, which an upstream change may rename.
func preloadScript(origin, adminPath, lang string) string {
	o, _ := json.Marshal(origin)
	p, _ := json.Marshal(adminPath)
	l, _ := json.Marshal(lang)
	return strings.NewReplacer("__ORIGIN__", string(o), "__PATH__", string(p), "__LANG__", string(l)).Replace(`(() => {
  // The web admin's page only: not an error page or the API's pages.
  if (location.origin !== __ORIGIN__ || location.pathname.replace(/\/$/, "") !== __PATH__) return;
  try {
    localStorage.setItem("admin:sidebar-collapsed", "true");
    localStorage.removeItem("theme-mode");
    localStorage.setItem("mx-admin-ui-locale", __LANG__);
  } catch {}
  const style = document.createElement("style");
  style.textContent = [
    'button[aria-label="展开侧边栏"]', 'button[aria-label="Expand sidebar"]',
    'button[aria-label="打开导航"]', 'button[aria-label="Open navigation"]',
  ].join(",") + "{display:none!important}";
  (document.head || document.documentElement).appendChild(style);

  let last = null;
  const report = () => {
    const route = location.hash.replace(/^#/, "") || "/";
    if (route === last) return;
    last = route;
    try { window.mygo && window.mygo.call("Bridge.Route", route); } catch {}
  };
  for (const name of ["pushState", "replaceState"]) {
    const original = history[name];
    history[name] = function (...args) {
      const result = original.apply(this, args);
      queueMicrotask(report);
      return result;
    };
  }
  addEventListener("hashchange", report);
  addEventListener("popstate", report);
  addEventListener("DOMContentLoaded", report);
})();`)
}
