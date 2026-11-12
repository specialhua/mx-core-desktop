package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"
)

// The pages the window shows in place of the sidebar and the web admin.
const (
	pageMain       = ""           // the shell's sidebar beside the web admin
	pageSetup      = "setup"      // ask for the server
	pageConnecting = "connecting" // the server answered, the web view is coming
	pageError      = "error"      // the web admin's page did not load
	pageLogin      = "login"      // the native sign-in
	pageWeb        = "web"        // a page of the web admin without its shell
)

// pageKind is the page the state asks for.
func (a *app) pageKind() string {
	switch {
	case a.srv.Root == "":
		return pageSetup
	case a.web == nil:
		return pageConnecting
	case a.loadErr != "":
		return pageError
	case strings.HasPrefix(a.route, "/login") && !a.login.password:
		return pageLogin
	case a.route == "" || isPublicRoute(a.route):
		// Loading, or a page of the web admin without its shell (the
		// password form, setup): the page alone.
		return pageWeb
	}
	return pageMain
}

// beginFrame runs first in every frame of the main window, as the shell's
// Theme hook, before the shell reads its page: the page follows the state
// in the same frame that changed it.
func (a *app) beginFrame(c *ui.Context) *ui.Theme {
	if kind := a.pageKind(); kind != a.page {
		a.page = kind
		if kind == pageMain {
			a.shell.SetPage(nil)
		} else {
			a.shell.SetPage(a.pageView)
		}
	}
	return c.Theme()
}

// pageView builds the pages that replace the sidebar and the web admin.
func (a *app) pageView(c *ui.Context) {
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c), 0, 0, 0).AlignItems(ui.Stretch).Children(func() {
		switch a.pageKind() {
		case pageSetup:
			a.setupView(c)
		case pageConnecting:
			ui.Column(c).Fill().Center().Children(func() {
				components.Spinner(c, components.SpinnerRing).Label(a.tr("正在打开后台…", "Opening the admin…"))
			})
		case pageError:
			a.loadErrorView(c)
		case pageLogin:
			a.loginView(c)
		default:
			if a.login.password {
				ui.Row(c).Padding(0, 12, 8, 12).Children(func() {
					back := components.Button(c, "", components.ButtonGhost).Label(a.tr("返回", "Back"))
					back.Children(func() {
						ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
							components.Icon(c, lucide.ChevronLeft()).Size(16, 16)
							ui.Text(c, a.tr("返回", "Back"))
						})
					})
					if back.Clicked() {
						a.login.password = false
					}
				})
			}
			if a.web != nil {
				ui.WebView(c, a.web).Grow(1)
			}
		}
	})
	a.flushToasts(c)
}

// contentView is the web admin beside the shell's sidebar, below its
// header, with the command palette over it.
func (a *app) contentView(c *ui.Context) {
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c), 0, 0, 0).AlignItems(ui.Stretch).Children(func() {
		if a.web != nil {
			ui.WebView(c, a.web).Grow(1).MinWidth(0)
		} else {
			ui.Box(c).Grow(1) // tests draw the view without a web view
		}
	})
	// Not a menu accelerator: the admin's editor binds Cmd+K itself, and the
	// page keeps the keys it handles.
	if c.Shortcut(ui.Cmd, ui.KeyK) && !c.ShortcutRepeated() {
		a.openPalette()
	}
	a.paletteView(c)
	a.flushToasts(c)
}

// contentTitle shows where the web admin is, in the shell's header: the
// sidebar items down to the chosen one, whose ancestors go back up.
func (a *app) contentTitle(c *ui.Context) {
	trail := navTrail(a.nav)
	if len(trail) == 0 {
		return
	}
	titles := make([]string, len(trail))
	for i, it := range trail {
		titles[i] = a.tr(it.Title, it.TitleEN)
	}
	chosen := len(trail) - 1
	if components.Breadcrumbs(c, titles, &chosen).Changed() && chosen < len(trail)-1 {
		a.nav = trail[chosen].Path
		a.navigate(trail[chosen].Path)
	}
}

// sidebarView is the shell's sidebar: the web admin's pages.
func (a *app) sidebarView(c *ui.Context) {
	t := c.Theme()
	// The scroll starts below the shell's header, which holds the window's
	// buttons and the sidebar toggle: items scroll out of sight at its edge
	// rather than under them.
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c), 0, 0, 0).Children(func() {
		ui.Scroll(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
			components.Sidebar(c).Padding(0, t.Space(3), t.Space(3), t.Space(3)).Children(func() {
				for _, s := range navSections {
					items := func() {
						for _, it := range s.Items {
							a.sidebarItem(c, it)
						}
					}
					if s.Title == "" {
						ui.Column(c).FillWidth().MinWidth(0).Gap(t.Space(1)).Children(items)
						continue
					}
					components.SidebarSection(c, a.tr(s.Title, s.TitleEN), items)
				}
			})
		})
	})
}

// sidebarItem shows an item and, while it or one of them is chosen, its
// children, as the web sidebar expands the active branch.
func (a *app) sidebarItem(c *ui.Context, it navItem) {
	e := components.SidebarItem(c, a.tr(it.Title, it.TitleEN), navIcon(it.Icon), a.nav == it.Path).Key(it.Path)
	if e.Clicked() && a.nav != it.Path {
		a.nav = it.Path
		a.navigate(it.Path)
	}
	if len(it.Children) == 0 || !(a.nav == it.Path || strings.HasPrefix(a.nav, it.Path+"/")) {
		return
	}
	// Indent the children with the padding of their column, not a margin of
	// their own, which would push them past the sidebar's edge.
	t := c.Theme()
	ui.Column(c).Key(it.Path+"/children").FillWidth().MinWidth(0).Gap(t.Space(1)).Padding(0, 0, 0, 14).Children(func() {
		for _, child := range it.Children {
			a.sidebarItem(c, child)
		}
	})
}

// sidebarToolbar is the bar under the sidebar.
func (a *app) sidebarToolbar(c *ui.Context) {
	if sidebar.IconButton(c, lucide.Search(), a.tr("前往页面或操作", "Go to a page or action")+" ("+shortcutText("K")+")").Clicked() {
		a.openPalette()
	}
	ui.Spacer(c)
	if sidebar.IconButton(c, lucide.ExternalLink(), a.tr("打开主站", "Open Main Site")).Clicked() {
		a.openMainSite()
	}
	if sidebar.IconButton(c, lucide.Settings(), a.tr("设置", "Settings")+" ("+shortcutText(",")+")").Clicked() {
		a.openPrefs()
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
				if components.Button(c, a.tr("重试", "Retry"), components.ButtonPrimary).Clicked() {
					a.loadErr = ""
					a.web.Page().Reload()
				}
				if components.Button(c, a.tr("更换服务器…", "Change Server…"), components.ButtonOutline).Clicked() {
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
			input := components.TextBox(c, &a.serverInput, components.TextBoxOptions{LeadingIcon: lucide.Server()}).
				Placeholder("https://").Label(a.tr("服务地址", "Server")).Disabled(a.probing).AutoFocus()
			empty := strings.TrimSpace(a.serverInput) == ""
			connect := components.Button(c, a.tr("连接", "Connect"), components.ButtonPrimary).Disabled(a.probing || empty)
			if (connect.Clicked() || input.Submitted()) && !a.probing && !empty {
				a.connect(a.serverInput, true)
			}
			if a.probing {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					components.Spinner(c, components.SpinnerRing).Size(16, 16)
					ui.Text(c, a.tr("正在连接…", "Connecting…")).TextColor(t.TextMuted)
				})
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
				ui.Text(c, a.login.code.UserCode).FontSize(28).Bold().Font("monospace").Selectable().AlignSelf(ui.Center).
					Padding(8, 20, 8, 20).Radius(t.Radius*2).Background(t.Surface).Border(1, t.Border)
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					components.Spinner(c, components.SpinnerRing).Size(16, 16)
					ui.Text(c, a.tr("等待确认…", "Waiting for approval…")).TextColor(t.TextMuted)
				})
				ui.Row(c).Gap(8).Children(func() {
					if components.Button(c, a.tr("重新打开授权页", "Open the approval page again"), components.ButtonOutline).Clicked() {
						url := a.login.url
						go mygo.Shell.OpenExternal(url)
					}
					if components.Button(c, a.tr("取消", "Cancel"), components.ButtonGhost).Clicked() {
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
				if components.Button(c, a.tr("在浏览器中登录后台", "Sign in to the admin in the browser"), components.ButtonLink).Clicked() {
					url := a.srv.AdminRoute("/login")
					go mygo.Shell.OpenExternal(url)
				}
			case a.login.busy:
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					components.Spinner(c, components.SpinnerRing).Size(16, 16)
					ui.Text(c, a.tr("正在登录…", "Signing in…")).TextColor(t.TextMuted)
				})
			default:
				if components.Button(c, a.tr("使用 Passkey 登录", "Sign in with a passkey"), components.ButtonPrimary).FillWidth().Clicked() {
					a.startPasskeyLogin()
				}
				ui.Text(c, a.tr("将在系统浏览器中完成验证。", "You approve it in your web browser.")).FontSize(12).TextColor(t.TextMuted)
				if components.Button(c, a.tr("使用密码登录", "Sign in with a password"), components.ButtonOutline).FillWidth().Clicked() {
					a.login.password = true
				}
			}
		})
	})
}

// notify queues a toast for the main window, which shows it in its next
// frame. Errors stay longer, and a toast with the same ID replaces it.
func (a *app) notify(kind, id, title, desc string) {
	t := ui.Toast{ID: id, Type: kind, Title: title, Description: desc}
	if kind == components.ToastError {
		t.Timeout = 8 * time.Second
	}
	a.toasts = append(a.toasts, t)
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) flushToasts(c *ui.Context) {
	for _, t := range a.toasts {
		c.AddToast(t)
	}
	a.toasts = nil
}
