package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/shells/plain"
)

// openPrefs shows the settings window (Cmd+,): the server, appearance,
// language and account, which the View and Account menus also hold.
func (a *app) openPrefs() {
	if a.prefs == nil || a.prefs.IsDestroyed() {
		a.prefs = plain.NewShell(plain.ShellOptions{
			Title: a.tr("设置", "Settings"), Width: 720, Height: 520, MinWidth: 480, MinHeight: 400,
			StateKey: "prefs",
			// The settings surface has its own title.
			HeaderLeading: func(*ui.Context) {},
			Content:       a.prefsView,
		}).Window()
		win := a.prefs
		win.OnClosed(func() {
			if a.prefs == win {
				a.prefs = nil
			}
		})
	}
	a.prefs.Show()
	a.prefs.Focus()
}

// invalidatePrefs redraws the settings window after a change made elsewhere.
func (a *app) invalidatePrefs() {
	if a.prefs != nil && !a.prefs.IsDestroyed() {
		a.prefs.SetTitle(a.tr("设置", "Settings"))
		a.prefs.Invalidate()
	}
}

func (a *app) prefsView(c *ui.Context) {
	w, _ := c.Size()
	components.Settings(c, &a.prefsState, a.prefsCategories(), components.SettingsOptions{
		Narrow:            w < 560,
		Title:             a.tr("设置", "Settings"),
		SearchLabel:       a.tr("搜索设置", "Search settings"),
		SearchPlaceholder: a.tr("搜索", "Search"),
		CategoryLabel:     a.tr("分类", "Category"),
		ResultsTitle:      a.tr("搜索结果", "Results"),
		EmptyText:         a.tr("没有匹配的设置", "No matching settings"),
	})
}

func (a *app) prefsCategories() []components.SettingsCategory {
	t := a.tr
	server := a.srv.Root
	if server == "" {
		server = t("未连接", "Not connected")
	}
	signedIn := a.web != nil && a.page == pageMain
	button := func(label string, variant components.ButtonVariant, disabled bool, fn func()) func(c *ui.Context) {
		return func(c *ui.Context) {
			if components.Button(c, label, variant).Disabled(disabled).Clicked() {
				fn()
			}
		}
	}
	// showInMain goes to a page of the web admin in the main window.
	showInMain := func(path string) func() {
		return func() {
			a.navigate(path)
			if a.win != nil {
				a.win.Show()
				a.win.Focus()
			}
			a.focusWeb()
		}
	}

	return []components.SettingsCategory{{
		ID: "general", Title: t("通用", "General"),
		Groups: []components.SettingsGroup{{
			ID: "server", Title: t("服务器", "Server"),
			Items: []components.SettingsItem{{
				ID: "server", Title: t("服务地址", "Server address"), Description: server,
				Keywords: []string{"server", "url", "服务器"},
				Build: button(t("更换服务器…", "Change Server…"), components.ButtonOutline, a.srv.Root == "", func() {
					a.changeServer()
					if a.win != nil {
						a.win.Focus()
					}
				}),
			}},
		}, {
			ID: "language", Title: t("语言", "Language"),
			Items: []components.SettingsItem{{
				ID: "language", Title: t("界面语言", "Language"),
				Description: t("应用和后台使用的语言。", "The language of the app and the admin."),
				Keywords:    []string{"language", "locale", "语言", "中文", "english"},
				Build: func(c *ui.Context) {
					lang := a.set.Lang
					sel := components.NativeSelect(c, &lang, []components.NativeSelectOption[string]{
						{Value: "zh-CN", Label: "简体中文"}, {Value: "en-US", Label: "English"},
					})
					sel.Trigger.Label(t("界面语言", "Language"))
					if sel.Changed() {
						a.setLang(lang)
					}
				},
			}},
		}},
	}, {
		ID: "appearance", Title: t("外观", "Appearance"),
		Groups: []components.SettingsGroup{{
			ID: "theme",
			Items: []components.SettingsItem{{
				ID: "theme", Title: t("外观", "Appearance"),
				Description: t("浅色、深色，或跟随系统。应用和后台一起切换。", "Light, dark, or follow the system, for the app and the admin together."),
				Keywords:    []string{"theme", "dark", "light", "主题", "深色", "浅色"},
				Build: func(c *ui.Context) {
					v := a.set.Appearance
					sel := components.NativeSelect(c, &v, []components.NativeSelectOption[string]{
						{Value: "system", Label: t("跟随系统", "System")},
						{Value: "light", Label: t("浅色", "Light")},
						{Value: "dark", Label: t("深色", "Dark")},
					})
					sel.Trigger.Label(t("外观", "Appearance"))
					if sel.Changed() {
						a.setAppearance(v)
					}
				},
			}},
		}},
	}, {
		ID: "account", Title: t("账户", "Account"),
		Groups: []components.SettingsGroup{{
			ID: "account",
			Items: []components.SettingsItem{{
				ID: "profile", Title: t("账户设置", "Account Settings"),
				Description: t("在后台中编辑资料、密码和 Passkey。", "Edit your profile, password and passkeys in the admin."),
				Keywords:    []string{"profile", "password", "passkey", "资料", "密码"},
				Build:       button(t("打开", "Open"), components.ButtonOutline, !signedIn, showInMain("/setting?group=user")),
			}, {
				ID: "main-site", Title: t("主站", "Main site"),
				Description: t("在浏览器中打开你的站点。", "Open your site in the browser."),
				Keywords:    []string{"site", "blog", "主站"},
				Build:       button(t("打开", "Open"), components.ButtonOutline, a.web == nil, a.openMainSite),
			}, {
				ID: "sign-out", Title: t("退出登录", "Sign out"),
				Description: t("结束这台设备上的登录，并从钥匙串中删除令牌。", "End the session on this device and remove its token from the Keychain."),
				Keywords:    []string{"sign out", "logout", "退出"},
				Build:       button(t("退出登录", "Sign Out"), components.ButtonSecondary, !signedIn, a.signOut),
			}},
		}},
	}}
}
