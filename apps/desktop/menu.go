package main

import (
	"runtime"

	"github.com/egoist/mygo"
)

// buildMenu sets the menu bar, in the app's language. The web admin's
// sidebar footer (account settings, main site, appearance, language, sign
// out) moves here, as its sidebar is hidden.
func (a *app) buildMenu() {
	click := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) { fn() }
	}
	page := func(fn func(p *mygo.Page)) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			if a.web != nil {
				fn(a.web.Page())
			}
		}
	}
	radio := func(label string, checked bool, fn func()) *mygo.MenuItem {
		return &mygo.MenuItem{Label: label, Type: mygo.MenuItemRadio, Checked: checked, Click: click(fn)}
	}

	prefs := &mygo.MenuItem{Label: a.tr("设置…", "Settings…"), Accelerator: "CmdOrCtrl+,", Click: click(a.openPrefs)}
	appMenu := &mygo.MenuItem{Role: mygo.RoleAppMenu}
	if runtime.GOOS == "darwin" {
		appMenu.Submenu = []*mygo.MenuItem{
			{Role: mygo.RoleAbout},
			mygo.Separator(),
			prefs,
			mygo.Separator(),
			{Role: mygo.RoleServices},
			mygo.Separator(),
			{Role: mygo.RoleHide},
			{Role: mygo.RoleHideOthers},
			{Role: mygo.RoleUnhide},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}
	}
	account := []*mygo.MenuItem{
		{Label: a.tr("账户设置", "Account Settings"), Click: click(func() { a.navigate("/setting?group=user") })},
		{Label: a.tr("打开主站", "Open Main Site"), Click: click(a.openMainSite)},
		mygo.Separator(),
		{Label: a.tr("更换服务器…", "Change Server…"), Click: click(a.changeServer)},
		{Label: a.tr("退出登录", "Sign Out"), Click: click(a.signOut)},
	}
	if runtime.GOOS != "darwin" {
		account = append([]*mygo.MenuItem{prefs, mygo.Separator()}, account...)
	}

	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		appMenu,
		{Role: mygo.RoleEditMenu},
		{Label: a.tr("显示", "View"), Submenu: []*mygo.MenuItem{
			// Cmd+K is the view's shortcut, not an accelerator here: the
			// admin's editor binds it, and an accelerator would take it.
			{Label: a.tr("前往页面或操作…", "Go to a Page or Action…"), Click: click(a.openPalette)},
			{Label: a.tr("显示或隐藏侧边栏", "Toggle Sidebar"), Click: click(func() {
				if a.shell != nil {
					a.shell.SetLeftExpanded(!a.shell.LeftExpanded())
				}
			})},
			mygo.Separator(),
			{Label: a.tr("刷新", "Reload"), Accelerator: "CmdOrCtrl+R", Click: page(func(p *mygo.Page) { p.Reload() })},
			{Label: a.tr("后退", "Back"), Accelerator: "CmdOrCtrl+[", Click: page(func(p *mygo.Page) { p.GoBack() })},
			{Label: a.tr("前进", "Forward"), Accelerator: "CmdOrCtrl+]", Click: page(func(p *mygo.Page) { p.GoForward() })},
			mygo.Separator(),
			{Label: a.tr("外观", "Appearance"), Submenu: []*mygo.MenuItem{
				radio(a.tr("跟随系统", "System"), a.set.Appearance == "system", func() { a.setAppearance("system") }),
				radio(a.tr("浅色", "Light"), a.set.Appearance == "light", func() { a.setAppearance("light") }),
				radio(a.tr("深色", "Dark"), a.set.Appearance == "dark", func() { a.setAppearance("dark") }),
			}},
			{Label: a.tr("语言", "Language"), Submenu: []*mygo.MenuItem{
				radio("简体中文", a.set.Lang == "zh-CN", func() { a.setLang("zh-CN") }),
				radio("English", a.set.Lang == "en-US", func() { a.setLang("en-US") }),
			}},
			mygo.Separator(),
			{Role: mygo.RoleToggleFullScreen},
			{Label: a.tr("开发者工具", "Developer Tools"), Accelerator: "Alt+CmdOrCtrl+I", Click: page(func(p *mygo.Page) { p.ToggleDevTools() })},
		}},
		{Label: a.tr("账户", "Account"), Submenu: account},
		{Role: mygo.RoleWindowMenu},
	}))
}
