package main

import (
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/components/command"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// The command palette (Cmd+K) goes to any page of the web admin's sidebar,
// by its title in either language or its path, and runs the menu's actions.
// IDs are "nav:<path>" for pages and "act:<name>" for actions.

func (a *app) openPalette() {
	if a.page != pageMain {
		return
	}
	a.palette.Query, a.palette.ActiveID = "", ""
	a.paletteOpen = true
	if a.win != nil {
		a.win.Invalidate()
	}
}

// paletteView builds the palette's dialog; call it in every frame.
func (a *app) paletteView(c *ui.Context) {
	var items []command.Item
	if a.paletteOpen {
		items = a.paletteItems()
	}
	command.CommandDialog(c, &a.paletteOpen, &a.palette, items, command.Options{
		SearchLabel:  a.tr("前往页面或操作", "Go to a page or action"),
		Placeholder:  a.tr("搜索页面和操作…", "Search pages and actions…"),
		ResultsLabel: a.tr("结果", "Results"),
		EmptyText:    a.tr("没有匹配的结果", "No matches"),
		OnActivate: func(id string) {
			a.paletteOpen = false
			a.runCommand(id)
		},
	}, components.DialogOptions{})
}

func (a *app) paletteItems() []command.Item {
	var items []command.Item
	for _, s := range navSections {
		group := a.tr("页面", "Pages")
		if s.Title != "" {
			group = a.tr(s.Title, s.TitleEN)
		}
		var walk func(its []navItem, parent string)
		walk = func(its []navItem, parent string) {
			for _, it := range its {
				title := a.tr(it.Title, it.TitleEN)
				items = append(items, command.Item{
					ID: "nav:" + it.Path, Title: title, Description: parent, Group: group,
					Keywords: []string{it.Title, it.TitleEN, it.Path}, Icon: navIcon(it.Icon),
				})
				walk(it.Children, title)
			}
		}
		walk(s.Items, "")
	}

	group := a.tr("操作", "Actions")
	act := func(id, title string, icon *ui.SVG, shortcut string, keywords ...string) command.Item {
		return command.Item{ID: "act:" + id, Title: title, Group: group, Icon: icon, Shortcut: shortcut, Keywords: keywords}
	}
	lang := act("lang-en", "Switch to English", lucide.Languages(), "", "language", "语言", "english")
	if a.set.Lang == "en-US" {
		lang = act("lang-zh", "切换到简体中文", lucide.Languages(), "", "language", "语言", "chinese")
	}
	return append(items,
		act("reload", a.tr("刷新", "Reload"), lucide.RefreshCw(), shortcutText("R"), "reload", "refresh"),
		act("back", a.tr("后退", "Back"), lucide.ArrowLeft(), shortcutText("["), "back"),
		act("forward", a.tr("前进", "Forward"), lucide.ArrowRight(), shortcutText("]"), "forward"),
		act("appearance-system", a.tr("外观：跟随系统", "Appearance: System"), lucide.Monitor(), "", "theme", "appearance", "外观"),
		act("appearance-light", a.tr("外观：浅色", "Appearance: Light"), lucide.Sun(), "", "theme", "appearance", "外观", "light"),
		act("appearance-dark", a.tr("外观：深色", "Appearance: Dark"), lucide.Moon(), "", "theme", "appearance", "外观", "dark"),
		lang,
		act("prefs", a.tr("设置…", "Settings…"), lucide.Settings(), shortcutText(","), "settings", "preferences", "设置", "偏好"),
		act("account", a.tr("账户设置", "Account Settings"), lucide.UserRound(), "", "account", "passkey", "password", "账户"),
		act("main-site", a.tr("打开主站", "Open Main Site"), lucide.ExternalLink(), "", "site", "blog", "主站"),
		act("change-server", a.tr("更换服务器…", "Change Server…"), lucide.Server(), "", "server", "服务器"),
		act("sign-out", a.tr("退出登录", "Sign Out"), lucide.LogOut(), "", "sign out", "logout", "退出"),
	)
}

// runCommand runs a palette item, by its ID.
func (a *app) runCommand(id string) {
	if path, ok := strings.CutPrefix(id, "nav:"); ok {
		a.nav = path
		a.navigate(path)
		a.focusWeb()
		return
	}
	page := func(fn func()) {
		if a.web != nil {
			fn()
		}
	}
	switch strings.TrimPrefix(id, "act:") {
	case "reload":
		page(func() { a.web.Page().Reload() })
	case "back":
		page(func() { a.web.Page().GoBack() })
	case "forward":
		page(func() { a.web.Page().GoForward() })
	case "appearance-system":
		a.setAppearance("system")
	case "appearance-light":
		a.setAppearance("light")
	case "appearance-dark":
		a.setAppearance("dark")
	case "lang-en":
		a.setLang("en-US")
	case "lang-zh":
		a.setLang("zh-CN")
	case "prefs":
		a.openPrefs()
	case "account":
		a.navigate("/setting?group=user")
		a.focusWeb()
	case "main-site":
		a.openMainSite()
	case "change-server":
		a.changeServer()
	case "sign-out":
		a.signOut()
	}
}

// focusWeb gives the keyboard back to the page, after the palette.
func (a *app) focusWeb() {
	if a.web != nil {
		a.web.Focus()
	}
}

// shortcutText shows a Cmd shortcut as the platform writes it.
func shortcutText(key string) string {
	if runtime.GOOS == "darwin" {
		return "⌘" + key
	}
	return "Ctrl+" + key
}
