package app

import (
	"context"
	"math"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// The maker is the dashed row that ends a folder's files, or its last
// tile in the grid, where what the folder holds is made: a folder, named
// in place as Finder names one, or files uploaded, picked or dragged onto
// the window, which light it up while they are over it. It takes the
// place of a button for each in the title bar.

// makerID keys the maker's tile among the grid's, whose files have ids
// above it, uploads' negative ones included.
const makerID int64 = math.MinInt64

// creating is a folder being named in the maker: Enter, or moving the
// focus away, makes it, Escape or an empty name gives up. It stays,
// waiting, until the folder shows in the listing, which then chooses it.
type creating struct {
	dir   string
	draft string
	fresh bool // not shown yet: the input takes the focus when it shows
	held  bool // the input has had the focus
	busy  bool // the netdisk making it
	made  string
	loads int // the listing's loads when it was made
}

// newFolder starts naming a new folder in l, in the maker.
func (a *App) newFolder(l *listing) {
	if m := a.creating; m != nil && m.busy {
		return
	}
	a.renaming, a.slow = nil, slowClick{}
	a.creating = &creating{dir: l.dir, draft: unusedName(l, "新建文件夹"), fresh: true}
	l.sel.Clear()
	l.selected = -1
	a.showMaker(l)
}

// unusedName is name, or name numbered as freeName does when l already
// has an item of that name.
func unusedName(l *listing, name string) string {
	taken := map[string]bool{}
	for _, f := range l.files {
		taken[strings.ToLower(f.Name)] = true
	}
	if !taken[strings.ToLower(name)] {
		return name
	}
	return freeName(name, taken)
}

// showMaker scrolls l to its end, where the maker is.
func (a *App) showMaker(l *listing) {
	n := len(l.files)
	if a.gridView {
		l.grid.scrollIntoView(n, a.gridSize[1])
	} else {
		l.list.ScrollIntoView(n)
	}
}

// creatingIn returns the folder being named in l, if any.
func (a *App) creatingIn(l *listing) *creating {
	if m := a.creating; m != nil && m.dir == l.dir {
		return m
	}
	return nil
}

// settleMaker ends naming a folder once the listing read after making it
// has come in, choosing the folder.
func (a *App) settleMaker(l *listing) {
	m := a.creatingIn(l)
	if m == nil || m.made == "" || l.loads <= m.loads {
		return
	}
	a.creating = nil
	if i := l.indexNamed(m.made); i >= 0 {
		l.sel.Clear()
		l.sel.Add(l.files[i].FsID)
		l.selected = i
		l.refocus = true
		if a.gridView {
			l.grid.pivot = i
			l.grid.scrollIntoView(i, a.gridSize[1])
		} else {
			l.list.ScrollIntoView(i)
		}
	}
}

// makeFolder makes the folder named in the maker, unless its name is
// empty, which gives up, or one the netdisk would refuse, which a toast
// says and, after Enter, leaves it to be named again.
func (a *App) makeFolder(l *listing, enter bool) {
	m := a.creating
	name := strings.TrimSpace(m.draft)
	why := ""
	switch {
	case name == "":
		a.creating = nil
		return
	case strings.ContainsAny(name, badNameChars):
		why = `名称不能包含 / \ : * ? " < > |`
	case l.indexNamed(name) >= 0:
		why = "这里已有名为「" + name + "」的项目"
	}
	s := a.sess
	if why != "" || s == nil {
		if why != "" {
			a.toast(why, "error")
		}
		if !enter || s == nil {
			a.creating = nil
		}
		return
	}
	m.busy = true
	dir := l.dir
	go func() {
		err := s.client.Mkdir(context.Background(), path.Join(dir, name))
		a.update(func() {
			if a.creating != m {
				return
			}
			if err != nil {
				a.toast("无法新建「"+name+"」："+err.Error(), "error")
				m.busy, m.held, m.fresh = false, false, true
				return
			}
			m.made, m.loads = name, l.loads
			a.reload(dir)
		})
	}()
}

// makerField is the input naming a new folder in the maker.
func (a *App) makerField(c *ui.Context, t *ui.Theme, l *listing, center bool) {
	m := a.creating
	pad := t.Space(0.5)
	in := ui.TextInputBase(c.Key("maker-name"), &m.draft).Grow(1).Shrink(1).MinWidth(t.Space(10)).
		Padding(pad, pad*2).Radius(t.Space(1)).Background(t.Background).Border(1, t.Accent).
		TextColor(t.Text).Label("新文件夹的名称")
	if center {
		in.TextAlign(ui.Center)
	}
	if m.busy {
		return
	}
	switch {
	case m.fresh:
		m.fresh = false
		in.Focus()
		c.Invalidate()
	case in.Focused() && !m.held:
		m.held = true
		in.SetTextSelection(0, utf8.RuneCountInString(m.draft))
	case in.Submitted():
		a.makeFolder(l, true)
	case in.Shortcut(0, ui.KeyEscape):
		a.creating = nil
		l.refocus = true
	case m.held && !in.Focused():
		a.makeFolder(l, false)
	}
}

// makerFrame dresses e as the maker: dashed, quiet, and lit in the
// accent while files are dragged over the window or a folder is named.
func makerFrame(e ui.Element, t *ui.Theme, lit bool) {
	edge, fill := t.TextMuted.Alpha(0.35), t.Background.Alpha(0)
	if lit {
		edge, fill = t.Accent, t.Background.Mix(t.Accent, 0.08)
	}
	e.Border(1, e.AnimateColor("edge", edge, motionHover)).BorderStyle(ui.BorderDashed).
		Background(e.AnimateColor("fill", fill, motionHover)).TextColor(t.TextMuted)
}

// makerButton is one of the maker's actions, an icon and a label in the
// muted text, the text color while hovered.
func makerButton(c *ui.Context, t *ui.Theme, ic, label string) ui.Element {
	b := ui.ButtonBase(c).Label(label).Gap(5).AlignItems(ui.Center).Justify(ui.Center).Padding(4, 10).Radius(6)
	if b.Hovered() {
		b.TextColor(t.Text)
	}
	b.Children(func() {
		ui.Icon(c, icon(ic)).FontSize(13)
		ui.Text(c, label).SingleLine()
	})
	return b
}

// makerRow is the maker as the row ending the list, or the empty
// folder's.
func (a *App) makerRow(c *ui.Context, t *ui.Theme, p *palette, l *listing) ui.Element {
	m := a.creatingIn(l)
	row := ui.Row(c.Key("maker")).Height(rowHeight + 4).PaddingX(10).Gap(8).AlignItems(ui.Center).Justify(ui.Center).Radius(7)
	makerFrame(row, t, l.over || m != nil)
	row.Children(func() {
		switch {
		case l.over:
			ui.Icon(c, icon("upload")).FontSize(13).TextColor(t.Accent)
			ui.Text(c, "松开以上传到「"+dirLabel(l.dir)+"」").TextColor(t.Accent).FontWeight(500).SingleLine()
		case m != nil:
			folderGlyph(c, p, 18)
			a.makerField(c, t, l, false)
			if m.busy {
				ui.Spinner(c).Size(14, 14).Label("正在新建")
			}
		default:
			if makerButton(c, t, "folder-plus", "新建文件夹").Grow(1).Clicked() {
				a.newFolder(l)
			}
			ui.Box(c).Size(1, 14).Background(p.Hairline)
			if makerButton(c, t, "upload", "上传文件").Grow(1).Clicked() {
				a.pickUpload(l.dir)
			}
		}
	})
	return row
}

// makerTile is the maker as the grid's last tile.
func (a *App) makerTile(c *ui.Context, t *ui.Theme, p *palette, l *listing, e ui.Element) {
	m := a.creatingIn(l)
	e.Padding(10).Gap(8).AlignItems(ui.Center).Justify(ui.Center).Radius(t.Radius).Label("新建")
	makerFrame(e, t, l.over || m != nil)
	e.Children(func() {
		switch {
		case l.over:
			ui.Icon(c, icon("upload")).FontSize(28).TextColor(t.Accent)
			ui.Text(c, "松开以上传").TextColor(t.Accent).FontWeight(500)
		case m != nil:
			folderGlyph(c, p, 72)
			ui.Row(c).Width(tileNameWidth).Gap(6).AlignItems(ui.Center).Children(func() {
				a.makerField(c, t, l, true)
				if m.busy {
					ui.Spinner(c).Size(14, 14).Label("正在新建")
				}
			})
		default:
			if makerButton(c, t, "folder-plus", "新建文件夹").Clicked() {
				a.newFolder(l)
			}
			if makerButton(c, t, "upload", "上传文件").Clicked() {
				a.pickUpload(l.dir)
			}
		}
	})
}

// emptyFolder is a folder with nothing in it, the maker under the words.
func (a *App) emptyFolder(c *ui.Context, t *ui.Theme, p *palette, l *listing) {
	ui.Column(c).Grow(1).Center().Gap(6).Padding(40).Transition(popIn).Children(func() {
		ui.Box(c).Size(52, 52).Radius(14).Center().Background(p.Group).Margin(0, 0, 8).Children(func() {
			ui.Icon(c, icon("folder-open")).FontSize(24).TextColor(p.Faint)
		})
		ui.Text(c, "这里还空着").FontSize(15).FontWeight(600)
		ui.Text(c, "新建一个文件夹，或把文件拖进窗口上传").TextColor(t.TextMuted).TextAlign(ui.Center).MaxWidth(360)
		a.makerRow(c, t, p, l).Width(320).Margin(14, 0, 0)
	})
}
