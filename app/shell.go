package app

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// navItem is a page of the sidebar, its icon on a tile of its color.
type navItem struct {
	path, label string
	tile        ui.Color
}

// navItems are the sidebar's pages; those from navSplit on sit under the
// title navSection.
var navItems = []navItem{
	{"/files", "我的文件", ui.Hex("#2f7cf6")},
	{"/transfers", "传输", ui.Hex("#34c759")},
	{"/shares", "分享", ui.Hex("#ff9500")},
	{"/offline", "离线下载", ui.Hex("#30b0c7")},
	{"/recycle", "回收站", ui.Hex("#8e8e93")},
}

const (
	navSplit   = 2
	navSection = "网盘"

	sidebarWidth = 216
	navRowHeight = 28
	navGap       = 2
	navHeader    = 30 // a section's title over its rows
	titleBarMin  = 52
	// compactBar is the title bar's width under which its choices fold
	// into pop-up buttons.
	compactBar = 720
)

// barHeight is how tall the title bar is: as tall as the window says, so
// that what it holds centers on the traffic lights, else titleBarMin.
func barHeight(c *ui.Context) float32 {
	if h := c.TitleBar().Height; h >= 40 {
		return h
	}
	return titleBarMin
}

// shell is the window around the pages: the sidebar over the window's
// material, and the pane of the page with its title bar.
func (a *App) shell(c *ui.Context, t *ui.Theme, p *palette) {
	a.sampleSpeed(c)
	if !a.dlg.open && a.pv == nil && c.Shortcut(ui.Cmd, ui.KeyComma) && a.router.Path() != "/settings" {
		a.router.Push("/settings")
	}
	vibrant := c.Vibrancy()
	if vibrant {
		c.Root().Background(ui.Transparent)
	}
	bar := c.TitleBar()
	row := ui.Row(c).Fill().AlignItems(ui.Stretch)
	row.Draw(func(p *ui.Painter, _ ui.Rect) { a.gpu = p.Accelerated() })
	shown := row.AnimateTo("sidebar", map[bool]float32{true: 0, false: 1}[a.sidebarCollapsed], motionPanel)
	row.Children(func() {
		// The sidebar slides out to the left as it hides.
		w := sidebarWidth * shown
		ui.Row(c).Width(w).Shrink(0).AlignItems(ui.Stretch).Clip().Children(func() {
			side := ui.Column(c).Width(sidebarWidth).Shrink(0).Margin(0, 0, 0, w-sidebarWidth)
			if !vibrant {
				side.Background(p.Sidebar)
			}
			side.Children(func() { a.sidebar(c, t, p) })
		})
		ui.Column(c).Grow(1).MinWidth(0).Background(t.Background).Children(func() {
			a.titleBar(c, t, p, 20+(1-shown)*(max(bar.Left, 0)+40))
			a.router.View(c, func(r *ui.Route) {
				switch {
				case r.Match("/files"), r.Match("/files/{path...}"):
					dir := dirOfRoute(r.Param("path"))
					r.Title(dirLabel(dir))
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

	// Beside the window controls, at the sidebar's edge while it shows.
	label := map[bool]string{true: "显示侧栏", false: "隐藏侧栏"}[a.sidebarCollapsed]
	left := max(bar.Left, 0) + 4 + shown*(sidebarWidth-max(bar.Left, 0)-44)
	toggle := ui.ButtonBase(c.Key("sidebar-toggle")).Absolute().Left(left).Top((barHeight(c)-28)/2).
		Size(32, 28).Radius(6).Center().TextColor(t.TextMuted).Label(label).Tooltip(label)
	if toggle.Hovered() {
		toggle.Background(p.Hover).TextColor(t.Text)
	}
	toggle.Children(func() { ui.Icon(c, icon("panel-left")).FontSize(16) })
	if toggle.Clicked() {
		a.sidebarCollapsed = !a.sidebarCollapsed
	}
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
	ui.Box(c).Height(barHeight(c)).Shrink(0).DragWindow()
	ui.Scroll(c).Grow(1).Padding(0, 10).Children(func() { a.nav(c, t, p) })
	a.profile(c, t, p)
}

// navKey keys a sidebar icon so that a new gesture starts afresh.
type navKey struct {
	path string
	run  uint64
	on   bool
}

// navY is where the row of item i starts in the sidebar's list.
func navY(i int) float32 {
	y := float32(i) * (navRowHeight + navGap)
	if i >= navSplit {
		y += navHeader
	}
	return y
}

// nav builds the sidebar's pages over a backdrop of the accent color that
// glides to the one chosen.
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
	y := list.AnimateTo("y", navY(max(chosen, 0)), motionPanel)
	shown := list.AnimateTo("shown", map[bool]float32{true: 1, false: 0}[chosen >= 0], motionHover)
	fill := t.Accent
	list.Draw(func(pt *ui.Painter, r ui.Rect) {
		pt.Fill(ui.Rect{X: r.X, Y: r.Y + y, W: r.W, H: navRowHeight}, fill.Alpha(shown), 6)
	})
	list.Children(func() {
		for i, it := range navItems {
			if i == navSplit {
				ui.Text(c, navSection).Height(navHeader-navGap).Padding(12, 8, 0).
					FontSize(11).FontWeight(600).TextColor(p.Faint).SingleLine()
			}
			a.navRow(c.Key(it.path), t, p, it, i == chosen)
		}
	})
}

func (a *App) navRow(c *ui.Context, t *ui.Theme, p *palette, it navItem, on bool) ui.Element {
	row := ui.ButtonBase(c).Label(it.label).Height(navRowHeight).Radius(6).PaddingX(6).Gap(8).Justify(ui.Start).
		AlignItems(ui.Center)
	text, value := t.Text, t.TextMuted
	if on {
		text, value = t.AccentText, t.AccentText.Alpha(0.85)
	} else if row.Hovered() {
		row.Background(p.Hover)
	}
	row.TextColor(row.AnimateColor("fg", text, motionHover))
	row.Children(func() {
		tile := ui.Box(c.Key(navKey{it.path, a.navRun, on})).Size(20, 20).Radius(5).Background(it.tile)
		ms := float32(navIconRun.Milliseconds())
		if on && a.navRun > 0 {
			ms = tile.Keyframes("gesture", []ui.Keyframe{{At: 0, Value: 0}, {At: navIconRun, Value: ms}})
		}
		white := ui.Hex("#ffffff")
		tile.Draw(func(pt *ui.Painter, r ui.Rect) {
			navGlyph(pt, ui.Rect{X: r.X + 3, Y: r.Y + 3, W: r.W - 6, H: r.H - 6}, it.path, ms, white)
		})
		ui.Text(c, it.label).SingleLine().Grow(1)
		if v := a.navValue(it.path); v != "" {
			figure(c, v).FontSize(11).TextColor(row.AnimateColor("value", value, motionHover)).
				ContentTransition(ui.ContentTransition{Numeric: true})
		}
	})
	if row.Clicked() && a.router.Path() != it.path {
		a.router.Push(it.path)
	}
	return row
}

// navValue is the figure a sidebar page shows at its end, as Pulse's
// show each meter's reading: what the page holds or does now.
func (a *App) navValue(page string) string {
	switch page {
	case "/files":
		if s := a.sess; s != nil && s.quota.Total > 0 {
			return itoa(int(float64(s.quota.Used)/float64(s.quota.Total)*100+0.5)) + "%"
		}
	case "/transfers":
		if n, speed := a.transferPulse(); n > 0 {
			return humanSpeed(speed)
		}
		if n := a.activeTransfers(); n > 0 {
			return itoa(n)
		}
	case "/shares":
		if a.shares.loaded && len(a.shares.items) > 0 {
			return itoa(len(a.shares.items))
		}
	case "/offline":
		if a.offline.loaded && len(a.offline.items) > 0 {
			return itoa(len(a.offline.items))
		}
	case "/recycle":
		if a.recycle.loaded && len(a.recycle.items) > 0 {
			return itoa(len(a.recycle.items))
		}
	}
	return ""
}

// titleBar is the band at the top of the pane, which drags the window:
// the page's title over a line of what it holds, then its tools. It keeps
// clear of all of bar.Right: on Linux that is a GTK header bar wider than
// its buttons, and its empty part, though transparent, takes the clicks.
func (a *App) titleBar(c *ui.Context, t *ui.Theme, p *palette, left float32) {
	bar := c.TitleBar()
	row := ui.Row(c).Height(barHeight(c)).Shrink(0).PaddingStart(left).PaddingEnd(bar.Right + 14).
		AlignItems(ui.Center).Gap(8).DragWindow()
	// As wide as the pane, whatever it holds, so last frame's width tells
	// whether the tools fit.
	if w := row.Bounds().W; w > 0 {
		a.compact = w < compactBar
	}
	row.Children(func() {
		if dir := a.currentDir(); dir != "" {
			a.filesToolbar(c, t, p, dir)
			return
		}
		if a.router.Path() == "/search" {
			if iconButton(c, t, p, "chevron-left", "后退").Disabled(!a.router.CanGoBack()).Clicked() {
				a.router.Back()
			}
		}
		pageTitle(c, t, a.router.Title(), a.subtitle())
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

// subtitle is the line under a page's title: what it holds.
func (a *App) subtitle() string {
	switch a.section() {
	case "/transfers":
		down, up, queued, speed := a.transferCounts()
		if down+up+queued == 0 {
			return "下载与上传"
		}
		s := []string{}
		if down > 0 {
			s = append(s, itoa(down)+" 个下载")
		}
		if up > 0 {
			s = append(s, itoa(up)+" 个上传")
		}
		if queued > 0 {
			s = append(s, itoa(queued)+" 个排队")
		}
		if speed > 0 {
			s = append(s, humanSpeed(speed))
		}
		return strings.Join(s, " · ")
	case "/shares":
		if a.shares.loaded {
			return itoa(len(a.shares.items)) + " 个链接"
		}
	case "/offline":
		if a.offline.loaded {
			return itoa(len(a.offline.items)) + " 个任务 · 网盘替你在云端下载"
		}
	case "/recycle":
		if a.recycle.loaded {
			return itoa(len(a.recycle.items)) + " 项 · 到期自动清除"
		}
	case "/search":
		if !a.found.loading && a.found.err == "" {
			where := ""
			if a.found.in != "/" {
				where = "在「" + dirLabel(a.found.in) + "」中"
			}
			if a.found.more {
				return where + "找到超过 " + itoa(len(a.found.results)) + " 个结果，只列出这些"
			}
			return where + "找到 " + itoa(len(a.found.results)) + " 个结果"
		}
	case "/settings":
		return "BaiduNetGo " + version()
	}
	return ""
}

// filesSubtitle counts what a folder holds.
func filesSubtitle(l *listing) string {
	if !l.loaded {
		return "正在读取…"
	}
	dirs, files := 0, 0
	var size int64
	for _, f := range l.files {
		if f.IsDir {
			dirs++
		} else {
			files++
			size += f.Size
		}
	}
	var s []string
	if dirs > 0 {
		s = append(s, itoa(dirs)+" 个文件夹")
	}
	if files > 0 {
		s = append(s, itoa(files)+" 个文件", humanSize(size))
	}
	if len(s) == 0 {
		return "空文件夹"
	}
	return strings.Join(s, " · ")
}
