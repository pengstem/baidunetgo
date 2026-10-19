package app

import (
	"path"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

const (
	rowHeight  = 52
	entranceIn = 700 * time.Millisecond
)

// filesPage is the browser showing one directory.
func (a *App) filesPage(c *ui.Context, t *ui.Theme, p *palette, dir string) {
	l := a.listingFor(dir)
	if l.loaded && l.sorted != a.order {
		sortFiles(l.files, a.order)
		l.sorted = a.order
	}
	ui.Column(c).Key("files:" + dir).Grow(1).Children(func() {
		switch {
		case !l.loaded && l.err != "":
			emptyState(c, t, p, "triangle-alert", "无法读取这个文件夹", l.err, func() { a.load(l) })
		case !l.loaded:
			skeletonRows(c, t, p)
		case len(l.files) == 0:
			emptyState(c, t, p, "folder-open", "这里还空着", "把文件拖进窗口即可上传", nil)
		case a.gridView:
			a.fileGrid(c, t, p, l)
		default:
			a.fileList(c, t, p, l)
		}
	})
}

// entering returns how far a row is into its entrance, 1 once in, for rows
// that show as the listing first appears.
func entering(e ui.Element, l *listing, i int, now time.Time) float32 {
	if now.Sub(l.shown) > entranceIn {
		return 1
	}
	return e.Keyframes("in", []ui.Keyframe{{At: 0, Value: 0}, {At: 360 * time.Millisecond, Value: 1, Ease: ui.CubicBezier(0.2, 0.8, 0.2, 1)}},
		ui.KeyframeOptions{Delay: time.Duration(min(i, 16)) * 24 * time.Millisecond})
}

func (a *App) bindListState(l *listing) {
	files := l.files
	l.list.Key = func(i int) any { return files[i].FsID }
	l.list.Label = func(i int) string { return files[i].Name }
	l.list.Selected = &l.selected
	l.list.Selection = &l.sel
	l.grid.Key = l.list.Key
	l.grid.Label = l.list.Label
	l.grid.Selected = &l.selected
	l.grid.Selection = &l.sel
}

func (a *App) fileList(c *ui.Context, t *ui.Theme, p *palette, l *listing) {
	a.bindListState(l)
	now := c.Now()
	a.listHeader(c, t, p)
	list := ui.List(c, &l.list, len(l.files)).Grow(1).Padding(2, 12, 24)
	list.Rows(func(row ui.ListRow) {
		c := row.Context
		f := &l.files[row.Index]
		r := ui.Row(c).Height(rowHeight).PaddingX(12).Gap(12).AlignItems(ui.Center)
		in := entering(r, l, row.Index, now)
		if in < 1 {
			r.Opacity(in).Translate(0, (1-in)*10)
		}
		r.Children(func() {
			a.fileIcon(c, t, p, f, 34)
			ui.Text(c, f.Name).Grow(1).Shrink(1).SingleLine().FontWeight(500)
			size := "—"
			if !f.IsDir {
				size = humanSize(f.Size)
			}
			ui.Text(c, size).Width(96).TextAlign(ui.End).TextColor(t.TextMuted).FontFeatures("tnum").SingleLine()
			ui.Text(c, humanTime(f.Mtime, now)).Width(150).TextAlign(ui.End).TextColor(t.TextMuted).SingleLine()
		})
		a.fileMenu(r, l, row.Index)
	})
	if list.Submitted() && l.selected >= 0 && l.selected < len(l.files) {
		a.open(l.files[l.selected])
	}
}

// listHeader names the columns, sorting by the one clicked.
func (a *App) listHeader(c *ui.Context, t *ui.Theme, p *palette) {
	ui.Row(c).Height(30).Margin(0, 12).PaddingX(12).Gap(12).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Hairline).FontSize(t.Rem(0.86)).Children(func() {
		col := func(label, by string, width float32, align ui.Align) {
			h := ui.ButtonBase(c).Label("按" + label + "排序").Gap(4).AlignItems(ui.Center).TextColor(t.TextMuted)
			if width > 0 {
				h.Width(width).Justify(ui.End)
			} else {
				h.Grow(1).Justify(ui.Start).MarginStart(46)
			}
			if a.order.By == by {
				h.TextColor(t.Text)
			}
			h.Children(func() {
				ui.Text(c, label).FontWeight(560)
				if a.order.By == by {
					ic := ui.Icon(c, icon("chevron-right")).FontSize(12)
					ic.Rotate(ic.AnimateTo("dir", map[bool]float32{true: -90, false: 90}[a.order.Desc], springSoft))
				}
			})
			if h.Clicked() {
				if a.order.By == by {
					a.order.Desc = !a.order.Desc
				} else {
					a.order = baidu.Order{By: by, Desc: by != "name"}
				}
			}
		}
		col("名称", "name", 0, ui.Start)
		col("大小", "size", 96, ui.End)
		col("修改时间", "time", 150, ui.End)
	})
}

func (a *App) fileGrid(c *ui.Context, t *ui.Theme, p *palette, l *listing) {
	a.bindListState(l)
	now := c.Now()
	grid := ui.GridView(c, &l.grid, len(l.files), 148, 178, func(i int) {
		f := &l.files[i]
		tile := ui.Column(c).Fill().Padding(8).Gap(8).AlignItems(ui.Center).Radius(14)
		in := entering(tile, l, i, now)
		pose := ui.Pose{Y: (1 - in) * 14, Fade: 1 - in}
		if tile.Hovered() && in == 1 {
			pose.Y = -2
		}
		tile.AnimatePose(pose, springSoft)
		tile.Children(func() {
			box := ui.Box(c).Width(124).Height(112).Radius(12).Center().Clip()
			k := kindOf(f)
			if bmp := a.thumb(f); bmp != nil {
				box.Children(func() {
					if a.previewing(f) {
						return // the preview shows it, grown from here
					}
					ui.Image(c, bmp).Fill().Fit(ui.Cover).
						Transition(ui.ElementTransition{Enter: &ui.Motion{Opacity: 0}, Animation: &springPanel, Match: thumbMatch{f.FsID}})
					if k == kindVideo {
						ui.Box(c).Absolute().Bottom(6).Right(6).Size(22, 22).Radius(11).Center().
							Background(ui.RGBA(0, 0, 0, 0.55)).TextColor(ui.Hex("#ffffff")).Children(func() {
							ui.Icon(c, icon("play")).FontSize(11)
						})
					}
				})
			} else {
				lk := k.look(p)
				box.Background(lk.color.Alpha(0.12)).Children(func() {
					ui.Icon(c, icon(lk.icon)).FontSize(44).TextColor(lk.color)
				})
			}
			ui.Text(c, f.Name).MaxLines(2).TextAlign(ui.Center).FontSize(t.Rem(0.92)).LineHeight(1.25)
		})
		a.fileMenu(tile, l, i)
	}).Grow(1).Padding(10, 16, 24).Gap(6)
	if grid.Submitted() && l.selected >= 0 && l.selected < len(l.files) {
		a.open(l.files[l.selected])
	}
}

// fileIcon shows a file's thumbnail, or its kind's icon on a tint.
func (a *App) fileIcon(c *ui.Context, t *ui.Theme, p *palette, f *baidu.File, size float32) {
	k := kindOf(f)
	box := ui.Box(c).Size(size, size).Radius(size * 0.26).Center().Clip()
	if k.hasThumb() {
		if bmp := a.thumb(f); bmp != nil {
			box.Children(func() {
				if a.previewing(f) {
					return
				}
				ui.Image(c, bmp).Fill().Fit(ui.Cover).
					Transition(ui.ElementTransition{Enter: &ui.Motion{Opacity: 0, Scale: 0.9}, Animation: &springPanel, Match: thumbMatch{f.FsID}})
			})
			return
		}
	}
	lk := k.look(p)
	box.Background(lk.color.Alpha(0.13)).Children(func() {
		ui.Icon(c, icon(lk.icon)).FontSize(size * 0.52).TextColor(lk.color)
	})
}

// fileMenu gives a file's row its context menu.
func (a *App) fileMenu(e ui.Element, l *listing, i int) {
	f := l.files[i]
	e.ContextMenu(func(m *ui.Menu) {
		if m.Item("打开").Chosen() {
			a.open(f)
		}
	})
}

// breadcrumbs shows where the browser is, each folder above a link to it.
func (a *App) breadcrumbs(c *ui.Context, t *ui.Theme, p *palette, dir string) {
	parts := []string{"/"}
	if dir != "/" {
		parts = append(parts, strings.Split(strings.TrimPrefix(dir, "/"), "/")...)
	}
	start := 0
	if len(parts) > 4 {
		start = len(parts) - 3
	}
	ui.Row(c).Gap(2).AlignItems(ui.Center).Shrink(1).ClipX().Children(func() {
		for i := range parts {
			if i > 0 && i < start {
				continue
			}
			if i > 0 {
				ui.Icon(c, icon("chevron-right")).FontSize(14).TextColor(t.TextMuted)
			}
			if i == 1 && start > 1 {
				ui.Text(c, "…").TextColor(t.TextMuted).PaddingX(4)
				ui.Icon(c, icon("chevron-right")).FontSize(14).TextColor(t.TextMuted)
			}
			label := parts[i]
			if i == 0 {
				label = "我的文件"
			}
			last := i == len(parts)-1
			crumb := ui.ButtonBase(c.Key(i)).Label(label).Height(30).PaddingX(8).Radius(8).Shrink(1).
				Transition(ui.ElementTransition{Animation: &springSoft, Enter: &ui.Motion{X: 12, Opacity: 0}, Exit: &ui.Motion{Opacity: 0}})
			if last {
				crumb.FontWeight(650).FontSize(16)
			} else {
				crumb.TextColor(t.TextMuted)
				if crumb.Hovered() {
					crumb.Background(p.Hover).TextColor(t.Text)
				}
			}
			crumb.Children(func() { ui.Text(c, label).SingleLine() })
			if crumb.Clicked() && !last {
				target := "/" + path.Join(parts[1:i+1]...)
				a.router.Push(filesRoute(target))
			}
		}
	})
}

// filesToolbar is the title bar's controls while browsing.
func (a *App) filesToolbar(c *ui.Context, t *ui.Theme, p *palette, dir string) {
	ui.Row(c).Gap(2).Children(func() {
		if iconButton(c, t, p, "arrow-left", "后退").Disabled(!a.router.CanGoBack()).Clicked() {
			a.router.Back()
		}
		if iconButton(c, t, p, "arrow-right", "前进").Disabled(!a.router.CanGoForward()).Clicked() {
			a.router.Forward()
		}
	})
	a.breadcrumbs(c, t, p, dir)
	ui.Spacer(c)
	if iconButton(c, t, p, "refresh-cw", "刷新").Clicked() {
		a.reload(dir)
	}
	a.viewSwitch(c, t, p)
}

// viewSwitch toggles between the list and the grid, its thumb gliding.
func (a *App) viewSwitch(c *ui.Context, t *ui.Theme, p *palette) {
	chosen := 0
	if a.gridView {
		chosen = 1
	}
	seg := ui.SegmentedBase(c, &chosen, 2)
	seg.Track.Height(32).Padding(3).Radius(10).Background(t.Surface).Label("视图")
	x := seg.Track.AnimateTo("thumb", float32(chosen), springSoft)
	thumb, shadow := p.Card, p.Shadow
	if t.Dark {
		thumb = t.SurfacePressed
	}
	seg.Track.Draw(func(pt *ui.Painter, r ui.Rect) {
		w := float32(32)
		th := ui.Rect{X: r.X + 3 + x*w, Y: r.Y + 3, W: w, H: r.H - 6}
		pt.Shadow(th, 7, 0, 1, 2, 0, shadow)
		pt.Fill(th, thumb, 7)
	})
	seg.Track.Children(func() {
		for i, ic := range []string{"list", "layout-grid"} {
			s := seg.Segment(i).Width(32).Center().Label([]string{"列表", "网格"}[i])
			color := t.TextMuted
			if i == chosen {
				color = t.Text
			}
			s.TextColor(s.AnimateColor("fg", color, fadeQuick))
			s.Children(func() { ui.Icon(c, icon(ic)).FontSize(15) })
		}
	})
	a.gridView = chosen == 1
}

// previewing reports whether the preview shows f.
func (a *App) previewing(f *baidu.File) bool { return a.pv != nil && a.pv.file.FsID == f.FsID }
