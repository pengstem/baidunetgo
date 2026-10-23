package app

import (
	"path"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/loading"
	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

const rowHeight = 28

// A folder's files come in one after another from the top as it first
// shows, each popping up from a little below and a little smaller: the
// rows rowStagger apart, the tiles tileStagger, over entranceFor each,
// the first entranceMax staggered and the rest with the last of them.
const (
	rowStagger  = 28 * time.Millisecond
	tileStagger = 45 * time.Millisecond
	entranceFor = 340 * time.Millisecond
	entranceMax = 24
)

// filesPage is the browser showing one directory.
func (a *App) filesPage(c *ui.Context, t *ui.Theme, p *palette, dir string) {
	l := a.listingFor(dir)
	a.arrivals(l, c.Now())
	if l.loaded && l.sorted != a.order {
		sortFiles(l.files, a.order)
		l.sorted = a.order
	}
	// Files coming and going are not chosen.
	for id := range l.sel.All() {
		if id < 0 || a.dying[id] != nil {
			l.sel.Remove(id)
		}
	}
	a.fileActions(c, l)
	a.slowRename(c, l)
	page := ui.Column(c).Key("files:" + dir).Grow(1)
	if dropped := page.DroppedFiles(); len(dropped) > 0 {
		a.upload(dropped, dir)
		a.dropped = c.Now()
	}
	page.Children(func() {
		switch {
		case !l.loaded && l.err != "":
			emptyState(c, t, p, "triangle-alert", "无法读取这个文件夹", l.err, func() { a.load(l) })
		case !l.loaded:
			loadingHint(c, t)
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
		a.selectionBar(c, t, p, l, page.Bounds().W)
		over := page.FileDragOver()
		if over {
			a.dropZone(c, t, p, dir)
		}
		a.dropGlow(c, t, over)
	})
	l.sole = 0
	if files := l.chosen(); len(files) == 1 {
		l.sole = files[0].FsID
	}
}

// dropZone shows where files dragged over the window will go.
func (a *App) dropZone(c *ui.Context, t *ui.Theme, p *palette, dir string) {
	zone := ui.Column(c).Key("dropzone").Absolute().Top(6).Left(8).Right(8).Bottom(8).Center().Gap(12).
		Radius(10).Background(t.Background.Alpha(0.8)).PassThrough().
		Transition(ui.ElementTransition{Animation: &motionFade, Enter: &ui.Motion{Opacity: 0}, Exit: &ui.Motion{Opacity: 0}})
	zone.Children(func() {
		disc := ui.Box(c).Size(52, 52).Radius(26).Center().Background(t.Accent).TextColor(t.AccentText)
		// The arrow rises and settles, again and again.
		lift := disc.Loop("lift", 1600*time.Millisecond, ui.Bounce(ui.EaseInOut))
		disc.Children(func() {
			ui.Icon(c, icon("upload")).FontSize(24).Translate(0, 2-4*lift)
		})
		ui.Text(c, "松开以上传到「"+dirLabel(dir)+"」").FontSize(14).FontWeight(500)
	})
}

// entering returns how far the file at i is into its entrance, from 0 to
// 1, the files after it waiting a stagger more; 1 for a listing shown a
// while.
func entering(e ui.Element, l *listing, i int, now time.Time, stagger time.Duration) float32 {
	if now.Sub(l.shown) > entranceMax*stagger+entranceFor {
		return 1
	}
	return e.Keyframes("in", []ui.Keyframe{{At: 0, Value: 0}, {At: entranceFor, Value: 1, Ease: easeStandard}},
		ui.KeyframeOptions{Delay: time.Duration(min(i, entranceMax)) * stagger})
}

// loadingHint shows a folder loading: nothing at first, as most load at
// once, then a spinner fading in.
func loadingHint(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).Center().Children(func() {
		s := loading.Ring(c, loading.Options{Size: 20, Color: t.TextMuted, Easing: loading.EaseInOut}).Label("加载中")
		s.Opacity(s.Keyframes("show", []ui.Keyframe{{At: 0, Value: 0}, {At: 400 * time.Millisecond, Value: 0}, {At: 600 * time.Millisecond, Value: 1}}))
	})
}

func (a *App) bindListState(l *listing) {
	files := l.files
	l.list.Key = func(i int) any { return files[i].FsID }
	l.list.Label = func(i int) string { return files[i].Name }
	l.list.Selected = &l.selected
	l.list.Selection = &l.sel
	l.list.DragSelect = true // a drag over the rows chooses them, as in Finder
}

func (a *App) fileList(c *ui.Context, t *ui.Theme, p *palette, l *listing) {
	a.bindListState(l)
	now := c.Now()
	a.listHeader(c, t, p)
	list := ui.List(c, &l.list, len(l.files)).Grow(1).Padding(0, 10, 16)
	outer, enter := c, false
	list.Rows(func(row ui.ListRow) {
		if enterRenames && !enter {
			// Enter renames the file chosen, as in Finder, before the list
			// takes it to open it; a double click still opens it.
			if enter = true; l.list.Shortcut(outer, 0, ui.KeyEnter) {
				if files := l.chosen(); len(files) == 1 {
					a.rename(l, files[0])
				}
			}
		}
		c := row.Context
		f := &l.files[row.Index]
		r := ui.Row(c).Height(rowHeight).PaddingX(10).Gap(8).AlignItems(ui.Center).Radius(6).TextColor(t.Text)
		muted := t.TextMuted
		passing, gone := a.passing(r, f, t, now)
		switch {
		case passing:
			// Gone, the row closes up.
			r.Height(rowHeight * (1 - smoothstep(0.7, 1, gone)))
		case row.Selected() && row.ListFocused():
			// Chosen in the list with the focus: the accent, its text
			// turned white, as AppKit's tables.
			r.Background(t.Accent).TextColor(t.AccentText)
			muted = t.AccentText.Alpha(0.8)
		case row.Selected():
			r.Background(t.Background.Mix(t.Text, 0.09))
		case r.Hovered():
			r.Background(t.Background.Mix(t.Text, 0.05))
		case row.Index%2 == 1:
			r.Background(t.Background.Mix(t.Text, 0.028))
		default:
			r.Background(t.Background) // opaque, over the list's own fill
		}
		if in := entering(r, l, row.Index, now, rowStagger); in < 1 {
			r.Opacity(in).Translate(0, (1-in)*10).Scale(0.97+0.03*in, 0.97+0.03*in)
		}
		r.Children(func() {
			a.fileIcon(c, t, p, f, 18)
			if a.renamingIn(l, f) {
				a.nameField(c, t, l, f, false)
			} else {
				a.clickName(l, f, ui.Text(c, f.Name).Grow(1).Shrink(1).SingleLine())
			}
			size := "—"
			if !f.IsDir {
				size = humanSize(f.Size)
			}
			ui.Text(c, kindLabel(f)).Width(colType).TextColor(muted).SingleLine()
			figure(c, size).Width(colSize).TextAlign(ui.End).TextColor(muted)
			when := stampTime(f.Mtime, now)
			if g := a.gathers[f.Path]; g != nil && f.FsID < 0 {
				when = uploadLabel(g)
			}
			figure(c, when).Width(colTime).TextAlign(ui.End).TextColor(muted)
		})
		if !passing {
			a.fileMenu(r, l, row.Index)
		}
	})
	if l.refocus {
		l.refocus = false
		l.list.Focus(c)
	}
	if list.Submitted() && l.selected >= 0 && l.selected < len(l.files) {
		a.open(l.files[l.selected])
	}
}

// List columns, in the header and the rows.
const (
	colType = 84
	colSize = 76
	colTime = 128
)

// listHeader names the columns, sorting by the one clicked: the column
// sorted by in the text color, with a chevron of its direction.
func (a *App) listHeader(c *ui.Context, t *ui.Theme, p *palette) {
	ui.Row(c).Height(28).Shrink(0).PaddingX(20).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Hairline).FontSize(12).Children(func() {
		col := func(label, by string, width float32) {
			h := ui.ButtonBase(c).Label("按" + label + "排序").Gap(3).AlignItems(ui.Center).TextColor(t.TextMuted)
			if width > 0 {
				h.Width(width).Justify(ui.End)
			} else {
				h.Grow(1).Justify(ui.Start).MarginStart(26)
			}
			if a.order.By == by {
				h.TextColor(t.Text).FontWeight(600)
			} else if h.Hovered() {
				h.TextColor(t.Text)
			}
			h.Children(func() {
				ui.Text(c, label)
				if a.order.By == by {
					ui.Icon(c, icon(map[bool]string{true: "chevron-down", false: "chevron-up"}[a.order.Desc])).FontSize(11)
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
		ui.Text(c, "种类").Width(colType).TextColor(t.TextMuted)
		col("大小", "size", colSize)
		col("修改日期", "time", colTime)
	})
}

// statusBar is the line under the browser: how many items, and what the
// transfers are doing.
func (a *App) statusBar(c *ui.Context, t *ui.Theme, p *palette, l *listing) {
	ui.Row(c).Height(28).Shrink(0).PaddingX(20).Gap(14).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(p.Hairline).
		FontSize(11).TextColor(t.TextMuted).Children(func() {
		label := itoa(len(l.files)) + " 项"
		if n := l.sel.Len(); n > 1 {
			label = "已选 " + itoa(n) + " 项，共 " + label
		}
		figure(c, label)
		ui.Spacer(c)
		if n, speed := a.transferPulse(); n > 0 {
			statusDot(c, t.Accent, itoa(n)+" 个传输")
			statusDot(c, t.Success, humanSpeed(speed))
		}
	})
}

// statusDot is a reading of the status bar after a dot of its color.
func statusDot(c *ui.Context, color ui.Color, label string) {
	ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(color)
		figure(c, label)
	})
}

func (a *App) fileGrid(c *ui.Context, t *ui.Theme, p *palette, l *listing) {
	now := c.Now()
	a.tileGrid(c, l, func(i int, tile ui.Element) {
		f := &l.files[i]
		tile.Padding(10).Gap(8).AlignItems(ui.Center).Radius(t.Radius).TextColor(t.Text)
		passing, _ := a.passing(tile, f, t, now)
		fill := t.Background
		switch {
		case passing:
			fill = ui.Transparent
		case l.sel.Has(f.FsID):
			fill = t.Background.Mix(t.Accent, 0.16)
		case tile.Hovered():
			fill = t.Background.Mix(t.Text, 0.05)
		}
		tile.Background(tile.AnimateColor("fill", fill, motionHover))
		in := entering(tile, l, i, now, tileStagger)
		tile.AnimatePose(ui.Pose{Y: (1 - in) * 14, Scale: 0.9 + 0.1*in, Fade: 1 - in}, motionPanel)
		tile.Children(func() {
			box := ui.Box(c).Width(124).Height(112).Radius(8).Center().Clip()
			k := kindOf(f)
			if bmp := a.thumb(f, 124); bmp != nil {
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
						ui.Text(c, typeLabel(f)).FontSize(10).FontWeight(600).TextColor(t.TextMuted)
					})
				})
			}
			if a.renamingIn(l, f) {
				ui.Row(c).Width(tileNameWidth).Children(func() { a.nameField(c, t, l, f, true) })
			} else {
				a.clickName(l, f, ui.Text(c, a.tileName(c, f.Name)).NoWrap().TextAlign(ui.Center).LineHeight(1.25))
			}
		})
		if !passing {
			a.fileMenu(tile, l, i)
		}
	})
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
		if bmp := a.thumb(f, size); bmp != nil {
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

// kindLabel is what a file is, in words, as Finder's Kind column.
func kindLabel(f *baidu.File) string {
	k := kindOf(f)
	switch k {
	case kindFolder:
		return "文件夹"
	case kindOther:
		if ext := strings.ToUpper(strings.TrimPrefix(path.Ext(f.Name), ".")); ext != "" && len(ext) <= 5 {
			return ext + " 文件"
		}
	}
	return k.look(nil).label
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
			a.rename(l, f)
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
		if one && kindOf(&f) == kindImage && m.Item("复制图片").Chosen() {
			a.copyImage(f, nil)
		}
		if m.Item("显示详情").Shortcut(ui.Cmd, ui.KeyI).Chosen() {
			a.showInfo(l.dir, files)
		}
		m.Separator()
		if m.Item("删除").Shortcut(0, ui.KeyDelete).Chosen() {
			a.remove(l.dir, files)
		}
	})
}

// breadcrumbs shows where the browser is: the folder as the title, the
// folders above it as links before it, and under them what it holds.
func (a *App) breadcrumbs(c *ui.Context, t *ui.Theme, p *palette, dir string) {
	parts := []string{"/"}
	if dir != "/" {
		parts = append(parts, strings.Split(strings.TrimPrefix(dir, "/"), "/")...)
	}
	start := 0
	if len(parts) > 4 {
		start = len(parts) - 3
	}
	ui.Column(c).Shrink(1).Gap(1).Children(func() {
		ui.Row(c).Gap(1).AlignItems(ui.Center).Shrink(1).ClipX().Children(func() {
			for i := range parts {
				if i > 0 && i < start {
					continue
				}
				if i > 0 {
					ui.Icon(c, icon("chevron-right")).FontSize(12).TextColor(p.Faint)
				}
				if i == 1 && start > 1 {
					ui.Text(c, "…").TextColor(t.TextMuted).PaddingX(3).Tooltip(strings.Join(parts[1:start], " / "))
					ui.Icon(c, icon("chevron-right")).FontSize(12).TextColor(p.Faint)
				}
				label := parts[i]
				if i == 0 {
					label = "我的文件"
				}
				last := i == len(parts)-1
				// The tip tells the whole path, as the crumbs shorten long
				// names and fold the middle of a deep one into "…".
				tip := "我的文件"
				if i > 0 {
					tip += " / " + strings.Join(parts[1:i+1], " / ")
				}
				crumb := ui.ButtonBase(c.Key(i)).Label(label).Tooltip(tip).PaddingX(3).Radius(4).Shrink(1).FontSize(14).
					Transition(ui.ElementTransition{Animation: &motionPanel, Enter: &ui.Motion{X: 6, Opacity: 0}, Exit: &ui.Motion{Opacity: 0}})
				if last {
					crumb.FontWeight(700)
				} else {
					crumb.FontWeight(500).TextColor(t.TextMuted)
					if crumb.Hovered() {
						crumb.TextColor(t.Text)
					}
				}
				crumb.Children(func() { ui.Text(c, label).SingleLine() })
				if crumb.Clicked() && !last {
					target := "/" + path.Join(parts[1:i+1]...)
					a.router.Push(filesRoute(target))
				}
			}
		})
		sub := filesSubtitle(a.listingFor(dir))
		ui.Text(c, sub).Tooltip(sub).FontSize(11).PaddingX(3).TextColor(t.TextMuted).SingleLine().FontFeatures("tnum")
	})
}

// filesToolbar is the title bar's controls while browsing.
func (a *App) filesToolbar(c *ui.Context, t *ui.Theme, p *palette, dir string) {
	ui.Row(c).Gap(0).Children(func() {
		if iconButton(c, t, p, "chevron-left", "后退").Disabled(!a.router.CanGoBack()).Clicked() {
			a.router.Back()
		}
		if iconButton(c, t, p, "chevron-right", "前进").Disabled(!a.router.CanGoForward()).Clicked() {
			a.router.Forward()
		}
	})
	a.breadcrumbs(c, t, p, dir)
	ui.Spacer(c)
	a.searchField(c, t, p)
	if a.gridView {
		a.sortMenu(c, t, p)
	}
	a.viewSwitch(c, t, p)
	if toolButton(c, t, p, "folder-plus", "新建文件夹").Clicked() {
		a.ask(dlgNewFolder, dir, nil)
	}
	if toolButton(c, t, p, "refresh-cw", "刷新").Clicked() {
		a.reload(dir)
	}
	if pill(c, t, p, "upload", "上传", true).Clicked() {
		a.pickUpload(dir)
	}
}

// sortOrders are what the browser sorts by, as the list's columns.
var sortOrders = []struct{ by, label string }{{"name", "名称"}, {"size", "大小"}, {"time", "修改日期"}}

// sortMenu is the grid's button choosing the order, which the list
// chooses by its columns: what to sort by, then which way.
func (a *App) sortMenu(c *ui.Context, t *ui.Theme, p *palette) {
	toolButton(c, t, p, "arrow-down-wide-narrow", "排序方式").Menu(func(m *ui.Menu) {
		for _, o := range sortOrders {
			if m.Item(o.label).Checked(a.order.By == o.by).Chosen() && a.order.By != o.by {
				a.order = baidu.Order{By: o.by, Desc: o.by != "name"}
			}
		}
		m.Separator()
		if m.Item("升序").Checked(!a.order.Desc).Chosen() {
			a.order.Desc = false
		}
		if m.Item("降序").Checked(a.order.Desc).Chosen() {
			a.order.Desc = true
		}
	})
}

// viewSwitch toggles between the list and the grid.
func (a *App) viewSwitch(c *ui.Context, t *ui.Theme, p *palette) {
	chosen := 0
	if a.gridView {
		chosen = 1
	}
	labels, icons := []string{"列表", "网格"}, []string{"list", "layout-grid"}
	if a.compact {
		popUp(c, t, p, &chosen, labels, icons).Label("视图")
	} else {
		segmentedOf(c, t, p, &chosen, 2, func(i int, s ui.Element) {
			s.Width(34).Label(labels[i])
			s.Children(func() { ui.Icon(c, icon(icons[i])).FontSize(14) })
		}).Label("视图")
	}
	a.gridView = chosen == 1
}

// previewing reports whether the preview shows f.
func (a *App) previewing(f *baidu.File) bool { return a.pv != nil && a.pv.file.FsID == f.FsID }
