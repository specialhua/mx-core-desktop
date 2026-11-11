package main

import (
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *app) view(c *ui.Context) {
	if a.loadErr != "" && a.web != nil {
		a.loadErrorView(c)
		return
	}
	switch {
	case a.srv.Root == "":
		a.setupView(c)
	case a.web == nil:
		ui.Column(c).Fill().Center().Children(func() { ui.Spinner(c) })
	case strings.HasPrefix(a.route, "/login") && !a.login.password:
		a.loginView(c)
	case a.route == "" || isPublicRoute(a.route):
		// Loading, or a page of the web admin without its shell (the
		// password form, setup): the page alone.
		ui.Column(c).Fill().AlignItems(ui.Stretch).Children(func() {
			if a.login.password {
				ui.Row(c).Padding(8, 12).Children(func() {
					if ui.Button(c, a.tr("‹ 返回", "‹ Back")).Clicked() {
						a.login.password = false
					}
				})
			}
			ui.WebView(c, a.web).Grow(1)
		})
	default:
		a.mainView(c)
	}
}

// mainView is the native sidebar beside the web admin.
func (a *app) mainView(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		sel := a.nav
		if ui.Sidebar(c, &sel, func() { a.sidebarItems(c) }).Width(208).Changed() && sel != "" {
			a.nav = sel
			a.navigate(sel)
		}
		ui.Box(c).Width(1).Background(t.Border)
		if a.web != nil {
			ui.WebView(c, a.web).Grow(1).MinWidth(0)
		} else {
			ui.Box(c).Grow(1) // tests draw the view without a web view
		}
	})
}

func (a *app) sidebarItems(c *ui.Context) {
	for _, s := range navSections {
		items := func() {
			for _, it := range s.Items {
				a.sidebarItem(c, it, 0)
			}
		}
		if s.Title == "" {
			items()
			continue
		}
		ui.SidebarSection(c, a.tr(s.Title, s.TitleEN), nil, items)
	}
}

// sidebarItem shows an item and, while it or one of them is chosen, its
// children, as the web sidebar expands the active branch.
func (a *app) sidebarItem(c *ui.Context, it navItem, depth int) {
	e := ui.SidebarItem(c.Key(it.Path), it.Path, navIcon(it.Icon), a.tr(it.Title, it.TitleEN))
	if depth > 0 {
		e.MarginStart(float32(14 * depth))
	}
	if len(it.Children) == 0 || !(a.nav == it.Path || strings.HasPrefix(a.nav, it.Path+"/")) {
		return
	}
	for _, child := range it.Children {
		a.sidebarItem(c, child, depth+1)
	}
}

// loadErrorView shows why the web admin did not load, with a retry.
func (a *app) loadErrorView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Children(func() {
		ui.Column(c).Width(400).Gap(12).Children(func() {
			ui.Text(c, a.tr("无法打开后台", "Cannot open the admin")).FontSize(20).Bold()
			ui.Text(c, a.loadErr).TextColor(t.TextMuted).Selectable()
			ui.Row(c).Gap(8).Children(func() {
				if ui.PrimaryButton(c, a.tr("重试", "Retry")).Clicked() {
					a.loadErr = ""
					a.web.Page().Reload()
				}
				if ui.Button(c, a.tr("更换服务器…", "Change Server…")).Clicked() {
					a.changeServer()
				}
			})
		})
	})
}

// setupView asks for the server.
func (a *app) setupView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Children(func() {
		ui.Column(c).Width(400).Gap(14).Children(func() {
			ui.Text(c, a.tr("连接你的空间", "Connect your space")).FontSize(24).Bold()
			ui.Text(c, a.tr("填入 Mix Space 服务地址，例如 https://server.example.com。", "Enter your Mix Space server, e.g. https://server.example.com.")).TextColor(t.TextMuted)
			input := ui.TextInput(c, &a.serverInput).Placeholder("https://").Label(a.tr("服务地址", "Server")).Disabled(a.probing).AutoFocus()
			connect := ui.PrimaryButton(c, a.tr("连接", "Connect")).Disabled(a.probing || strings.TrimSpace(a.serverInput) == "")
			if (connect.Clicked() || input.Submitted()) && !a.probing && strings.TrimSpace(a.serverInput) != "" {
				a.connect(a.serverInput, true)
			}
			if a.probing {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c)
					ui.Text(c, a.tr("正在连接…", "Connecting…")).TextColor(t.TextMuted)
				})
			}
			if a.setupErr != "" {
				ui.Text(c, a.setupErr).TextColor(t.Danger).Selectable()
			}
		})
	})
}

// loginView offers the two ways to sign in: a passkey in the system
// browser, or the web admin's password form.
func (a *app) loginView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Children(func() {
		ui.Column(c).Width(380).Gap(14).Children(func() {
			ui.Text(c, a.tr("登录", "Sign in")).FontSize(24).Bold()
			ui.Text(c, a.srv.Root).TextColor(t.TextMuted).Selectable()

			switch {
			case a.login.code != nil:
				ui.Text(c, a.tr("已在浏览器中打开授权页面。请用 Passkey 登录并确认下面的验证码：", "The approval page is open in your browser. Sign in with your passkey and confirm this code:")).TextColor(t.TextMuted)
				ui.Text(c, a.login.code.UserCode).FontSize(28).Bold().Font("monospace").Selectable().AlignSelf(ui.Center)
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c)
					ui.Text(c, a.tr("等待确认…", "Waiting for approval…")).TextColor(t.TextMuted)
				})
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, a.tr("重新打开授权页", "Open the approval page again")).Clicked() {
						url := a.login.url
						go mygo.Shell.OpenExternal(url)
					}
					if ui.Button(c, a.tr("取消", "Cancel")).Clicked() {
						a.cancelLogin()
					}
				})
				// The approval page sends a browser that is not signed in to
				// /proxy/qaqdmin/#/login, a 404, and the admin's login does not
				// come back to it: sign in first, then reopen the approval page.
				ui.Text(c, a.tr(
					"浏览器显示 404 或要求登录？先在浏览器中登录后台，再点「重新打开授权页」。",
					"Does the browser show a 404 or ask you to sign in? Sign in to the admin there first, then open the approval page again.",
				)).FontSize(12).TextColor(t.TextMuted)
				if ui.Button(c, a.tr("在浏览器中登录后台", "Sign in to the admin in the browser")).Clicked() {
					url := a.srv.AdminRoute("/login")
					go mygo.Shell.OpenExternal(url)
				}
			case a.login.busy:
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c)
					ui.Text(c, a.tr("正在登录…", "Signing in…")).TextColor(t.TextMuted)
				})
			default:
				if ui.PrimaryButton(c, a.tr("使用 Passkey 登录", "Sign in with a passkey")).FillWidth().Clicked() {
					a.startPasskeyLogin()
				}
				ui.Text(c, a.tr("将在系统浏览器中完成验证。", "You approve it in your web browser.")).FontSize(12).TextColor(t.TextMuted)
				if ui.Button(c, a.tr("使用密码登录", "Sign in with a password")).FillWidth().Clicked() {
					a.login.password = true
				}
			}
			if a.login.err != "" {
				ui.Text(c, a.login.err).TextColor(t.Danger).Selectable()
			}
		})
	})
}
