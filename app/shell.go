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
	sidebarWidth     = 168
	sidebarCollapsed = 52
	navRowHeight     = 32
	navGap           = 2
	titleBarMin      = 46
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
					a.settingsPage(c, t, p)
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
	if s := a.section(); s != a.navAt {
		if a.navAt != "" {
			a.navRun++ // the new page's icon makes its gesture
		}
		a.navAt = s
	}
	bar := c.TitleBar()
	w := float32(sidebarWidth)
	if a.sidebarCollapsed {
		w = sidebarCollapsed
	}
	side := ui.Column(c).Key("sidebar").Width(w).ClipX().Background(p.Sidebar).
		BorderWidth(0, 1, 0, 0).BorderColor(p.Hairline).
		Transition(ui.ElementTransition{Size: true, Animation: &motionPanel})
	side.Children(func() {
		// The top of the sidebar drags the window, under macOS's traffic
		// lights where they are.
		ui.Row(c).Height(max(bar.Height, titleBarMin)).PaddingStart(max(bar.Left, 0) + 16).
			AlignItems(ui.Center).Gap(10).DragWindow().Children(func() {
			logoMark(c, t, p, 20)
			if !a.sidebarCollapsed {
				ui.Text(c, "BaiduNetGo").FontSize(14).FontWeight(600).SingleLine()
			}
		})

		ui.Column(c).Padding(4, 8).Children(func() {
			a.nav(c, t, p)
		})
		ui.Spacer(c)
		ui.Column(c).Padding(0, 8, 8).Gap(4).Children(func() {
			a.profile(c, t, p)
			ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
				if !a.sidebarCollapsed {
					a.navRow(c, t, p, navItem{"/settings", "settings", "设置"}, a.section() == "/settings").Grow(1)
				}
				collapse := ui.ButtonBase(c).Label("收起侧栏").Tooltip(map[bool]string{true: "展开侧栏", false: "收起侧栏"}[a.sidebarCollapsed]).
					Size(32, navRowHeight).Radius(5).Center().TextColor(t.TextMuted)
				if collapse.Hovered() {
					collapse.Background(p.Hover).TextColor(t.Text)
				}
				collapse.Children(func() { ui.Icon(c, icon("panel-left")).FontSize(16) })
				if collapse.Clicked() {
					a.sidebarCollapsed = !a.sidebarCollapsed
				}
			})
			if a.sidebarCollapsed {
				a.navRow(c, t, p, navItem{"/settings", "settings", "设置"}, a.section() == "/settings")
			}
		})
	})
}

// logoMark is the app's mark: four blocks, the last one fetched, as the
// downloads fetch theirs.
func logoMark(c *ui.Context, t *ui.Theme, p *palette, size float32) {
	cell := size * 0.26
	gap := size * 0.1
	ui.Box(c).Size(size, size).Radius(size * 0.2).Background(t.Text).Draw(func(pt *ui.Painter, r ui.Rect) {
		x0 := r.X + (r.W-2*cell-gap)/2
		y0 := r.Y + (r.H-2*cell-gap)/2
		for i := range 4 {
			color := t.Background
			if i == 3 {
				color = t.Accent
			}
			x := x0 + float32(i%2)*(cell+gap)
			y := y0 + float32(i/2)*(cell+gap)
			pt.Fill(ui.Rect{X: x, Y: y, W: cell, H: cell}, color, size*0.05)
		}
	})
}

// navKey keys a sidebar icon so that a new gesture starts afresh.
type navKey struct {
	path string
	run  uint64
	on   bool
}

// nav builds the sidebar's pages over a backdrop that glides to the one
// chosen.
func (a *App) nav(c *ui.Context, t *ui.Theme, p *palette) {
	chosen := -1
	for i, it := range navItems {
		if a.section() == it.path {
			chosen = i
		}
	}
	list := ui.Column(c).Gap(navGap)
	// The backdrop is painted by the list, under the rows: an absolute
	// element would cover them.
	y := list.AnimateTo("y", float32(max(chosen, 0))*(navRowHeight+navGap), motionPanel)
	shown := list.AnimateTo("shown", map[bool]float32{true: 1, false: 0}[chosen >= 0], motionHover)
	raised := p.Raised
	list.Draw(func(pt *ui.Painter, r ui.Rect) {
		pt.Fill(ui.Rect{X: r.X, Y: r.Y + y, W: r.W, H: navRowHeight}, raised.Alpha(shown), 5)
	})
	list.Children(func() {
		for i, it := range navItems {
			a.navRow(c.Key(it.path), t, p, it, i == chosen)
		}
	})
}

func (a *App) navRow(c *ui.Context, t *ui.Theme, p *palette, it navItem, on bool) ui.Element {
	row := ui.ButtonBase(c).Label(it.label).Height(navRowHeight).Radius(5).PaddingX(10).Gap(10).Justify(ui.Start).
		AlignItems(ui.Center)
	if a.sidebarCollapsed {
		row.PaddingX(0).Justify(ui.Center)
	}
	text, mark := t.TextMuted, t.TextMuted
	if on {
		text, mark = t.Text, t.Accent
		if it.path == "/settings" { // outside the backdrop's list
			row.Background(p.Raised)
		}
	} else if row.Hovered() {
		row.Background(p.Hover)
		text, mark = t.Text, t.Text
	}
	row.TextColor(row.AnimateColor("fg", text, motionHover))
	if a.sidebarCollapsed {
		row.Tooltip(it.label)
	}
	badge := 0
	if it.path == "/transfers" {
		badge = a.activeTransfers()
	}
	row.Children(func() {
		mark := row.AnimateColor("mark", mark, motionHover)
		glyph := ui.Box(c.Key(navKey{it.path, a.navRun, on})).Size(18, 18)
		ms := float32(navIconRun.Milliseconds())
		if on && a.navRun > 0 {
			ms = glyph.Keyframes("gesture", []ui.Keyframe{{At: 0, Value: 0}, {At: navIconRun, Value: ms}})
		}
		glyph.Draw(func(pt *ui.Painter, r ui.Rect) { navGlyph(pt, r, it.path, ms, mark) })
		if badge > 0 && a.sidebarCollapsed {
			ui.Box(c).Absolute().Top(6).Right(8).Size(6, 6).Radius(3).Background(t.Accent).Transition(popIn)
		}
		if !a.sidebarCollapsed {
			ui.Text(c, it.label).SingleLine().Grow(1).FontWeight(500)
			if badge > 0 {
				figure(c, itoa(badge)).FontSize(11).TextColor(t.Accent).
					ContentTransition(ui.ContentTransition{Numeric: true})
			}
		}
	})
	if row.Clicked() && a.router.Path() != it.path {
		a.router.Push(it.path)
	}
	return row
}

// titleBar is the band at the top of the content, under the window
// controls, which drags the window. It keeps clear of all of bar.Right:
// on Linux that is a GTK header bar wider than its buttons, and its empty
// part, though transparent, takes the clicks.
func (a *App) titleBar(c *ui.Context, t *ui.Theme, p *palette) {
	bar := c.TitleBar()
	ui.Row(c).Height(max(bar.Height, titleBarMin)).PaddingStart(20).PaddingEnd(bar.Right + 8).
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
		ui.Text(c, a.router.Title()).FontSize(15).FontWeight(600).SingleLine()
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
