# Mix Space desktop

The desktop client of the Mix Space admin, built with [MyGo](https://mygo.run) (pinned in `go.mod`). It adds no features: a native window, sidebar and menu bar wrap the web admin that the server serves at `/proxy/qaqdmin/`, which shows in a web view. Everything past the sidebar is the web admin, so the app follows the server's version without a new release.

macOS first; Windows and Linux build from the same code.

## Develop

Needs Go 1.27+.

```sh
cd apps/desktop
go tool mygo dev     # run with live reload
go test ./...        # tests, including headless renders of the screens
go tool mygo build   # build/darwin-arm64/Mix Space.app and a .dmg
```

`SCREENSHOTS=<dir> go test -run TestScreens` saves the rendered screens as PNGs.

## How it fits the web admin

- **Sidebar**: `nav_gen.go` and `icons/` are generated from the web admin's route metadata (`apps/admin/src/views/**/page.tsx`), its zh-CN/en-US titles and its Lucide icons. After an upstream update, run `go generate` here (it needs `pnpm install` for lucide-react) and review the diff.
- **Bridge** (`bridge.go`): a preload script, run only on the server's origin, collapses the web sidebar, hides its expand buttons, follows the app's appearance and language, and reports the hash route to Go, so the native sidebar highlights the page. It relies on the admin's storage keys (`admin:sidebar-collapsed`, `theme-mode`, `mx-admin-ui-locale`) and on two `aria-label`s. If upstream renames them, the web sidebar's buttons reappear; nothing breaks.
- **Menu bar** (`menu.go`): holds what the hidden web sidebar footer had: account settings, main site, appearance, language and sign out.

## Signing in

- **Password**: the web admin's own form, in the web view.
- **Passkey**: device authorization in the system browser (`auth.go`), like the mxs CLI and the iOS app. The browser signs in with the passkey and approves a code. The app stores the session token in the Keychain and trades it through `/device/web-handoff` for the web admin's cookie. On a later launch with an expired cookie, the Keychain token signs in again on its own.

  The app borrows the `mxs-cli` device client, because the server only accepts clients it lists (`apps/core/src/modules/auth/auth.constant.ts`). So the approval page says "Mix Space CLI".
