package app

import (
	"path"
	"strings"

	"github.com/egoist/mygo/ui"
)

// navItem is a page of the sidebar.
type navItem struct {
	path, icon, label string
}

var navItems = []navItem{
	{"/files", "hard-drive", "我的文件"},
	{"/transfers", "arrow-up-down", "传输"},
	{"/shares", "share-2", "分享"},
	{"/offline", "cloud-download", "离线下载"},
	{"/recycle", "trash-2", "回收站"},
}

const (
	sidebarWidth     = 228
	sidebarCollapsed = 68
	navRowHeight     = 38
	titleBarMin      = 52
)

// shell is the window around the pages: the sidebar and the title bar.
func (a *App) shell(c *ui.Context, t *ui.Theme, p *palette) {
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.sidebar(c, t, p)
		ui.Column(c).Grow(1).Background(t.Background).Children(func() {
			a.titleBar(c, t, p)
			a.router.View(c, func(r *ui.Route) {
				switch {
				case r.Match("/files"), r.Match("/files/{path...}"):
					dir := dirOfRoute(r.Param("path"))
					r.Title(path.Base(dir))
					if dir == "/" {
						r.Title("我的文件")
					}
					a.filesPage(c, t, p, dir)
				case r.Match("/transfers"):
					r.Title("传输")
					a.transfersPage(c, t, p)
				case r.Match("/shares"):
					r.Title("我的分享")
					a.sharesPage(c, t, p)
				case r.Match("/offline"):
					r.Title("离线下载")
					a.offlinePage(c, t, p)
				case r.Match("/recycle"):
					r.Title("回收站")
					a.recyclePage(c, t, p)
				case r.Match("/search"):
					r.Title("搜索")
					a.searchPage(c, t, p, a.router.Query("q"))
				case r.Match("/settings"):
					r.Title("设置")
					a.placeholder(c, t, "settings", "设置")
				}
			})
		})
	})
}

// section returns the sidebar entry the page shown belongs to.
func (a *App) section() string {
	path := a.router.Path()
	for _, it := range navItems {
		if path == it.path || strings.HasPrefix(path, it.path+"/") {
			return it.path
		}
	}
	return path
}

func (a *App) sidebar(c *ui.Context, t *ui.Theme, p *palette) {
	bar := c.TitleBar()
	w := float32(sidebarWidth)
	if a.sidebarCollapsed {
		w = sidebarCollapsed
	}
	side := ui.Column(c).Key("sidebar").Width(w).ClipX().Background(p.Sidebar).
		BorderWidth(0, 1, 0, 0).BorderColor(p.Hairline).
		Transition(ui.ElementTransition{Size: true, Animation: &springPanel})
	side.Children(func() {
		// The top of the sidebar drags the window, under macOS's traffic
		// lights where they are.
		ui.Row(c).Height(max(bar.Height, titleBarMin)).PaddingStart(max(bar.Left, 0) + 18).
			AlignItems(ui.Center).Gap(10).DragWindow().Children(func() {
			logo := ui.Box(c).Size(28, 28).Radius(8).Center().
				LinearGradient(ui.LinearGradient{
					Stops: []ui.GradientStop{{Color: ui.Oklch(0.68, 0.17, 250), At: 0}, {Color: ui.Oklch(0.58, 0.21, 285), At: 1}},
					Angle: 135, Oklab: true,
				})
			logo.Children(func() {
				ui.Icon(c, icon("hard-drive")).FontSize(15).TextColor(ui.Hex("#ffffff"))
			})
			if !a.sidebarCollapsed {
				ui.Text(c, "BaiduNetGo").Bold().FontSize(15).SingleLine().
					Transition(ui.ElementTransition{Enter: &ui.Motion{X: -8}, Exit: &ui.Motion{}, Animation: &springSoft})
			}
		})

		ui.Column(c).Padding(6, 10).Children(func() {
			a.nav(c, t, p)
		})
		ui.Spacer(c)
		ui.Column(c).Padding(10).Gap(4).Children(func() {
			a.profile(c, t, p)
			a.navRow(c, t, p, navItem{"/settings", "settings", "设置"}, a.section() == "/settings")
			collapse := ui.ButtonBase(c).Label("收起侧栏").Height(navRowHeight).Radius(9).PaddingX(12).Gap(12).Justify(ui.Start).
				AlignItems(ui.Center).TextColor(t.TextMuted)
			if collapse.Hovered() {
				collapse.Background(p.Hover)
			}
			collapse.Children(func() {
				ic := ui.Icon(c, icon("panel-left")).FontSize(17)
				ic.Rotate(ic.AnimateTo("turn", map[bool]float32{true: 180, false: 0}[a.sidebarCollapsed], springSoft))
				if !a.sidebarCollapsed {
					ui.Text(c, "收起").SingleLine()
				}
			})
			if collapse.Clicked() {
				a.sidebarCollapsed = !a.sidebarCollapsed
			}
		})
	})
}

// nav builds the sidebar's pages over a pill that glides to the one
// chosen.
func (a *App) nav(c *ui.Context, t *ui.Theme, p *palette) {
	chosen := -1
	for i, it := range navItems {
		if a.section() == it.path {
			chosen = i
		}
	}
	ui.Box(c).Children(func() {
		pill := ui.Box(c).Absolute().Left(0).Right(0).Top(0).Height(navRowHeight).Radius(9).
			Background(p.Selected).PassThrough()
		y := pill.AnimateTo("y", float32(max(chosen, 0))*navRowHeight, springSoft)
		pill.Translate(0, y)
		pill.Opacity(pill.AnimateTo("shown", map[bool]float32{true: 1, false: 0}[chosen >= 0], fadeQuick))

		for i, it := range navItems {
			a.navRow(c.Key(it.path), t, p, it, i == chosen)
		}
	})
}

func (a *App) navRow(c *ui.Context, t *ui.Theme, p *palette, it navItem, on bool) {
	row := ui.ButtonBase(c).Label(it.label).Height(navRowHeight).Radius(9).PaddingX(12).Gap(12).Justify(ui.Start).
		AlignItems(ui.Center)
	color := t.TextMuted
	if on {
		color = t.Accent
	} else if row.Hovered() {
		row.Background(p.Hover)
		color = t.Text
	}
	row.TextColor(row.AnimateColor("fg", color, fadeQuick))
	if a.sidebarCollapsed {
		row.Tooltip(it.label)
	}
	badge := 0
	if it.path == "/transfers" {
		badge = a.activeTransfers()
	}
	row.Children(func() {
		ui.Box(c).Children(func() {
			ui.Icon(c, icon(it.icon)).FontSize(17)
			if badge > 0 && a.sidebarCollapsed {
				ui.Box(c).Absolute().Top(-3).Right(-4).Size(8, 8).Radius(4).Background(t.Accent).Transition(popIn)
			}
		})
		if !a.sidebarCollapsed {
			txt := ui.Text(c, it.label).SingleLine().Grow(1)
			if on {
				txt.FontWeight(600)
			}
			if badge > 0 {
				ui.Text(c, itoa(badge)).FontSize(11).FontWeight(700).Padding(1, 7).Radius(999).
					Background(t.Accent).TextColor(t.AccentText).FontFeatures("tnum").
					ContentTransition(ui.ContentTransition{Numeric: true}).Transition(popIn)
			}
		}
	})
	if row.Clicked() && a.router.Path() != it.path {
		a.router.Push(it.path)
	}
}

// titleBar is the band at the top of the content, under the window
// controls, which drags the window.
func (a *App) titleBar(c *ui.Context, t *ui.Theme, p *palette) {
	bar := c.TitleBar()
	ui.Row(c).Height(max(bar.Height, titleBarMin)).PaddingStart(20).PaddingEnd(bar.Right + 16).
		AlignItems(ui.Center).Gap(10).DragWindow().Children(func() {
		if dir := a.currentDir(); dir != "" {
			a.filesToolbar(c, t, p, dir)
			return
		}
		if a.router.Path() == "/search" {
			if iconButton(c, t, p, "arrow-left", "后退").Disabled(!a.router.CanGoBack()).Clicked() {
				a.router.Back()
			}
		}
		ui.Text(c, a.router.Title()).FontSize(17).FontWeight(650).SingleLine()
		ui.Spacer(c)
		switch a.section() {
		case "/recycle":
			a.recycleToolbar(c, t, p)
		case "/transfers":
			a.transfersToolbar(c, t, p)
		case "/shares":
			a.sharesToolbar(c, t, p)
		case "/offline":
			a.offlineToolbar(c, t, p)
		case "/search":
			a.searchField(c, t, p)
		}
	})
}

func (a *App) placeholder(c *ui.Context, t *ui.Theme, ic, label string) {
	ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
		ui.Icon(c, icon(ic)).FontSize(40).TextColor(t.TextMuted).Transition(popIn)
		ui.Text(c, label).TextColor(t.TextMuted)
	})
}
