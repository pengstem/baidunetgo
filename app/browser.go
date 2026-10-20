package app

import (
	"path"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

const (
	rowHeight  = 36
	entranceIn = 500 * time.Millisecond
)

// filesPage is the browser showing one directory.
func (a *App) filesPage(c *ui.Context, t *ui.Theme, p *palette, dir string) {
	l := a.listingFor(dir)
	if l.loaded && l.sorted != a.order {
		sortFiles(l.files, a.order)
		l.sorted = a.order
	}
	a.fileActions(c, l)
	page := ui.Column(c).Key("files:" + dir).Grow(1)
	if dropped := page.DroppedFiles(); len(dropped) > 0 {
		a.upload(dropped, dir)
	}
	page.Children(func() {
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
		if l.loaded {
			a.statusBar(c, t, p, l)
		}
		a.selectionBar(c, t, p, l)
		if page.FileDragOver() {
			a.dropZone(c, t, p, dir)
		}
	})
}

// dropZone shows where files dragged over the window will go.
func (a *App) dropZone(c *ui.Context, t *ui.Theme, p *palette, dir string) {
	zone := ui.Column(c).Key("dropzone").Absolute().Top(8).Left(12).Right(12).Bottom(12).Center().Gap(10).
		Radius(8).Background(t.Background.Alpha(0.9)).Border(1, t.Accent).BorderStyle(ui.BorderDashed).PassThrough().
		Transition(ui.ElementTransition{Animation: &motionFade, Enter: &ui.Motion{Opacity: 0}, Exit: &ui.Motion{Opacity: 0}})
	zone.Children(func() {
		ui.Icon(c, icon("upload")).FontSize(22).TextColor(t.Accent)
		ui.Text(c, "松开以上传到「"+dirLabel(dir)+"」").FontSize(14).FontWeight(500)
	})
}

// entering returns how far a row is into its entrance, 1 once in, for rows
// that show as the listing first appears.
func entering(e ui.Element, l *listing, i int, now time.Time) float32 {
	if now.Sub(l.shown) > entranceIn {
		return 1
	}
	return e.Keyframes("in", []ui.Keyframe{{At: 0, Value: 0}, {At: 180 * time.Millisecond, Value: 1, Ease: easeStandard}},
		ui.KeyframeOptions{Delay: time.Duration(min(i, 16)) * 12 * time.Millisecond})
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
	list := ui.List(c, &l.list, len(l.files)).Grow(1).Padding(0, 12, 16)
	list.Rows(func(row ui.ListRow) {
		c := row.Context
		f := &l.files[row.Index]
		r := ui.Row(c).Height(rowHeight).PaddingX(10).Gap(10).AlignItems(ui.Center).Radius(t.Radius).TextColor(t.Text)
		if row.Selected() {
			r.Background(selectionFill(t, row.ListFocused()))
		} else if r.Hovered() {
			r.Background(p.Hover)
		}
		in := entering(r, l, row.Index, now)
		if in < 1 {
			r.Opacity(in).Translate(0, (1-in)*4)
		}
		r.Children(func() {
			a.fileIcon(c, t, p, f, 18)
			ui.Text(c, f.Name).Grow(1).Shrink(1).SingleLine()
			typ, size, sizeColor := "—", "—", p.Faint
			if !f.IsDir {
				typ, size, sizeColor = typeLabel(f), humanSize(f.Size), t.Text
			}
			figure(c, typ).Width(colType).FontSize(11).TextColor(t.TextMuted)
			figure(c, size).Width(colSize).TextAlign(ui.End).TextColor(sizeColor)
			figure(c, stampTime(f.Mtime, now)).Width(colTime).TextAlign(ui.End).TextColor(t.TextMuted)
		})
		a.fileMenu(r, l, row.Index)
	})
	if list.Submitted() && l.selected >= 0 && l.selected < len(l.files) {
		a.open(l.files[l.selected])
	}
}

// List columns, in the header and the rows.
const (
	colType = 56
	colSize = 88
	colTime = 110
)

// listHeader names the columns, sorting by the one clicked.
func (a *App) listHeader(c *ui.Context, t *ui.Theme, p *palette) {
	ui.Row(c).Height(30).Margin(0, 12).PaddingX(10).Gap(10).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Hairline).Font(fontMono).FontSize(11).Children(func() {
		col := func(label, by string, width float32) {
			h := ui.ButtonBase(c).Label("按" + label + "排序").Gap(4).AlignItems(ui.Center).TextColor(p.Faint)
			if width > 0 {
				h.Width(width).Justify(ui.End)
			} else {
				h.Grow(1).Justify(ui.Start).MarginStart(28)
			}
			if a.order.By == by {
				h.TextColor(t.TextMuted)
			} else if h.Hovered() {
				h.TextColor(t.Text)
			}
			h.Children(func() {
				ui.Text(c, label)
				if a.order.By == by {
					ui.Text(c, map[bool]string{true: "↓", false: "↑"}[a.order.Desc])
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
		col("名称", "name", 0)
		ui.Text(c, "类型").Width(colType).TextColor(p.Faint)
		col("大小", "size", colSize)
		col("修改时间", "time", colTime)
	})
}

// statusBar is the line under the browser: how many items, and what the
// transfers are doing.
func (a *App) statusBar(c *ui.Context, t *ui.Theme, p *palette, l *listing) {
	ui.Row(c).Height(28).PaddingX(22).Gap(16).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(p.Hairline).
		Background(p.Sidebar).Font(fontMono).FontSize(11).TextColor(p.Faint).Children(func() {
		ui.Text(c, itoa(len(l.files))+" 项").FontFeatures("tnum")
		ui.Spacer(c)
		if n, speed := a.transferPulse(); n > 0 {
			ui.Text(c, "↓ "+itoa(n)+" · "+humanSpeed(speed)).TextColor(t.Text).FontFeatures("tnum")
		}
	})
}

func (a *App) fileGrid(c *ui.Context, t *ui.Theme, p *palette, l *listing) {
	a.bindListState(l)
	now := c.Now()
	grid := ui.GridView(c, &l.grid, len(l.files), 148, 178, func(i int) {
		f := &l.files[i]
		// One plane: the tile's own fill, opaque and 2 DIP over the cell's
		// edges so that it covers the border and fill the grid paints for
		// a hovered or chosen cell, and no frames inside it.
		tile := ui.Column(c).Absolute().Top(-2).Left(-2).Right(-2).Bottom(-2).Padding(10).Gap(8).AlignItems(ui.Center).Radius(t.Radius).TextColor(t.Text)
		fill := t.Background
		switch {
		case l.sel.Has(f.FsID):
			fill = selectionFill(t, true)
		case tile.Hovered():
			fill = t.Background.Mix(t.Text, 0.05)
		}
		tile.Background(tile.AnimateColor("fill", fill, motionHover))
		in := entering(tile, l, i, now)
		tile.AnimatePose(ui.Pose{Y: (1 - in) * 4, Fade: 1 - in}, motionPanel)
		tile.Children(func() {
			box := ui.Box(c).Width(124).Height(112).Radius(5).Center().Clip()
			k := kindOf(f)
			if bmp := a.thumb(f); bmp != nil {
				box.Children(func() {
					if a.previewing(f) {
						return // the preview shows it, grown from here
					}
					ui.Image(c, bmp).Fill().Fit(ui.Cover).
						Transition(ui.ElementTransition{Enter: &ui.Motion{Opacity: 0}, Animation: &motionPanel, Match: thumbMatch{f.FsID}})
					if k == kindVideo {
						ui.Box(c).Absolute().Bottom(6).Right(6).Size(22, 22).Radius(11).Center().
							Background(ui.RGBA(0, 0, 0, 0.55)).TextColor(ui.Hex("#ffffff")).Children(func() {
							ui.Icon(c, icon("play")).FontSize(11)
						})
					}
				})
			} else {
				box.Children(func() {
					if k == kindFolder {
						folderGlyph(c, p, 72)
						return
					}
					ui.Column(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, icon(k.look(p).icon)).FontSize(44).TextColor(p.Faint)
						figure(c, typeLabel(f)).FontSize(10).TextColor(t.TextMuted)
					})
				})
			}
			ui.Text(c, a.tileName(c, f.Name)).NoWrap().TextAlign(ui.Center).LineHeight(1.25)
		})
		a.fileMenu(tile, l, i)
	}).Grow(1).Padding(10, 16, 24).Gap(6)
	if grid.Submitted() && l.selected >= 0 && l.selected < len(l.files) {
		a.open(l.files[l.selected])
	}
}

// tileNameWidth is how wide a grid tile's name may be: the narrowest
// tile's width inside its padding.
const tileNameWidth = 132

// tileName lays a name out for a grid tile on at most two lines, as
// Finder does: the first breaks where the name stops fitting, after a
// separator when one is near the end, and the second keeps the end of the
// name, extension and all, cutting from its middle what does not fit.
// Release names have no spaces, so wrapping alone runs them over the
// tiles beside them or ends them before the episode and extension.
func (a *App) tileName(c *ui.Context, name string) string {
	if s, ok := a.tileNames[name]; ok {
		return s
	}
	fits := func(r []rune) bool {
		w, _ := c.MeasureText(0, ui.Span{Text: string(r)})
		return w <= tileNameWidth
	}
	// longest is the most runes, at least one, that fit of those f takes.
	longest := func(n int, f func(int) []rune) int {
		lo, hi := 1, n
		for lo < hi {
			m := (lo + hi + 1) / 2
			if fits(f(m)) {
				lo = m
			} else {
				hi = m - 1
			}
		}
		return lo
	}
	r := []rune(name)
	out := name
	if !fits(r) {
		cut := longest(len(r), func(n int) []rune { return r[:n] })
		for i := cut - 1; i >= cut*2/3; i-- {
			if strings.ContainsRune(" ._-()[]【】", r[i]) {
				cut = i + 1
				break
			}
		}
		first, rest := r[:cut], []rune(strings.TrimLeft(string(r[cut:]), " "))
		if !fits(rest) {
			middle := func(n int) []rune {
				head, tail := rest[:n-n/2], rest[len(rest)-n/2:]
				return append(append(append([]rune{}, head...), '…'), tail...)
			}
			rest = middle(longest(len(rest), middle))
		}
		out = strings.TrimRight(string(first), " ") + "\n" + string(rest)
	}
	if a.tileNames == nil {
		a.tileNames = map[string]string{}
	}
	a.tileNames[name] = out
	return out
}

// fileIcon shows a file's thumbnail, else a solid folder, else its kind's
// icon in a single quiet color.
func (a *App) fileIcon(c *ui.Context, t *ui.Theme, p *palette, f *baidu.File, size float32) {
	k := kindOf(f)
	if k.hasThumb() {
		if bmp := a.thumb(f); bmp != nil {
			ui.Box(c).Size(size, size).Radius(3).Clip().Children(func() {
				if a.previewing(f) {
					return
				}
				ui.Image(c, bmp).Fill().Fit(ui.Cover).
					Transition(ui.ElementTransition{Enter: &ui.Motion{Opacity: 0}, Animation: &motionPanel, Match: thumbMatch{f.FsID}})
			})
			return
		}
	}
	if k == kindFolder {
		folderGlyph(c, p, size)
		return
	}
	ui.Box(c).Size(size, size).Center().Children(func() {
		ui.Icon(c, icon(k.look(p).icon)).FontSize(size * 0.92).TextColor(t.TextMuted)
	})
}

// folderGlyph is a folder, solid in the folder color: the one color a
// file list shows, as it is what tells folders apart.
func folderGlyph(c *ui.Context, p *palette, size float32) ui.Element {
	color := p.Folder
	return ui.Box(c).Size(size, size).Draw(func(pt *ui.Painter, r ui.Rect) {
		s := r.W / 24
		at := func(x, y float32) (float32, float32) { return r.X + x*s, r.Y + y*s }
		var path ui.Path
		path.MoveTo(at(2.5, 6.5))
		qx, qy := at(2.5, 5)
		ex, ey := at(4, 5)
		path.QuadTo(qx, qy, ex, ey)
		path.LineTo(at(9.2, 5))
		path.LineTo(at(11.2, 7.2))
		path.LineTo(at(20, 7.2))
		qx, qy = at(21.5, 7.2)
		ex, ey = at(21.5, 8.7)
		path.QuadTo(qx, qy, ex, ey)
		path.LineTo(at(21.5, 18))
		qx, qy = at(21.5, 19.5)
		ex, ey = at(20, 19.5)
		path.QuadTo(qx, qy, ex, ey)
		path.LineTo(at(4, 19.5))
		qx, qy = at(2.5, 19.5)
		ex, ey = at(2.5, 18)
		path.QuadTo(qx, qy, ex, ey)
		path.Close()
		pt.FillPath(&path, color)
	})
}

// typeLabel is a file's type in a few capitals: its extension, else its
// kind.
func typeLabel(f *baidu.File) string {
	ext := strings.ToUpper(strings.TrimPrefix(path.Ext(f.Name), "."))
	if ext != "" && len(ext) <= 5 {
		return ext
	}
	return kindOf(f).look(nil).label
}

// fileMenu gives a file's row its context menu, acting on the files
// chosen when the row is one of them, else on the row's.
func (a *App) fileMenu(e ui.Element, l *listing, i int) {
	f := l.files[i]
	e.ContextMenu(func(m *ui.Menu) {
		files := []baidu.File{f}
		if l.sel.Has(f.FsID) {
			files = l.chosen()
		}
		one := len(files) == 1
		if one && m.Item("打开").Chosen() {
			a.open(f)
		}
		if k := kindOf(&f); one && (k == kindVideo || k == kindAudio) {
			if m.Item("用 mpv 播放").Chosen() {
				a.playInMPV(f)
			}
		}
		m.Separator()
		if m.Item("下载").Chosen() {
			a.download(files)
		}
		if m.Item("分享").Chosen() {
			a.share(files)
		}
		m.Separator()
		if one && m.Item("重命名").Shortcut(0, ui.KeyF2).Chosen() {
			a.ask(dlgRename, l.dir, files)
		}
		if m.Item("移动到…").Chosen() {
			a.ask(dlgMove, l.dir, files)
		}
		if m.Item("复制到…").Chosen() {
			a.ask(dlgCopy, l.dir, files)
		}
		if one && m.Item("复制路径").Chosen() {
			a.copy(f.Path, "已复制路径")
		}
		m.Separator()
		if m.Item("删除").Shortcut(0, ui.KeyDelete).Chosen() {
			a.ask(dlgDelete, l.dir, files)
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
				ui.Text(c, "/").Font(fontMono).TextColor(p.Faint)
			}
			if i == 1 && start > 1 {
				ui.Text(c, "…").TextColor(t.TextMuted).PaddingX(4)
				ui.Text(c, "/").Font(fontMono).TextColor(p.Faint)
			}
			label := parts[i]
			if i == 0 {
				label = "我的文件"
			}
			last := i == len(parts)-1
			crumb := ui.ButtonBase(c.Key(i)).Label(label).Height(28).PaddingX(6).Radius(5).Shrink(1).
				Transition(ui.ElementTransition{Animation: &motionPanel, Enter: &ui.Motion{X: 6, Opacity: 0}, Exit: &ui.Motion{Opacity: 0}})
			if last {
				crumb.FontWeight(600).FontSize(15)
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
	a.searchField(c, t, p)
	if iconButton(c, t, p, "folder-plus", "新建文件夹").Clicked() {
		a.ask(dlgNewFolder, dir, nil)
	}
	if iconButton(c, t, p, "upload", "上传").Clicked() {
		a.pickUpload(dir)
	}
	if iconButton(c, t, p, "refresh-cw", "刷新").Clicked() {
		a.reload(dir)
	}
	a.viewSwitch(c, t, p)
}

// viewSwitch toggles between the list and the grid: an outlined pair,
// the chosen one filled with the line color.
func (a *App) viewSwitch(c *ui.Context, t *ui.Theme, p *palette) {
	chosen := 0
	if a.gridView {
		chosen = 1
	}
	seg := ui.SegmentedBase(c, &chosen, 2)
	seg.Track.Height(30).Radius(5).Border(1, p.Hairline).Clip().Label("视图")
	x := seg.Track.AnimateTo("thumb", float32(chosen), motionPanel)
	thumb := p.Hairline
	seg.Track.Draw(func(pt *ui.Painter, r ui.Rect) {
		w := float32(30)
		pt.Fill(ui.Rect{X: r.X + 1 + x*w, Y: r.Y + 1, W: w, H: r.H - 2}, thumb, 0)
	})
	seg.Track.Children(func() {
		for i, ic := range []string{"list", "layout-grid"} {
			s := seg.Segment(i).Width(30).Center().Label([]string{"列表", "网格"}[i])
			color := p.Faint
			if i == chosen {
				color = t.Text
			}
			s.TextColor(s.AnimateColor("fg", color, motionHover))
			s.Children(func() { ui.Icon(c, icon(ic)).FontSize(14) })
		}
	})
	a.gridView = chosen == 1
}

// previewing reports whether the preview shows f.
func (a *App) previewing(f *baidu.File) bool { return a.pv != nil && a.pv.file.FsID == f.FsID }

// selectionFill is the soft tint of chosen rows, opaque so that it covers
// the list's own accent fill.
func selectionFill(t *ui.Theme, focused bool) ui.Color {
	amount := float32(0.1)
	if focused {
		amount = 0.17
	}
	return t.Background.Mix(t.Accent, amount)
}
