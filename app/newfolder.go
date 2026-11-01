package app

import (
	"context"
	"math"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// The maker is the row that ends a folder's files, or its last tile in
// the grid, where what the folder holds is made: a folder, or files
// uploaded, picked or dragged onto the window, which light it up while
// they are over it. It takes the place of a button for each in the title
// bar.
//
// A new folder is named where folders are, at the top: the rows make room
// for it, and it comes from the maker's button into the gap (landing.go).
// Once made, it goes from there to its place among the folders.

// makerID keys the maker's tile among the grid's, and draftID the folder
// being named among the files, whose ids are above them, uploads'
// negative ones included.
const (
	makerID int64 = math.MinInt64
	draftID int64 = math.MinInt64 + 1
)

// creating is a folder being named: Enter makes it, Escape, or moving the
// focus away with its name untouched, gives up. It stays, waiting, until
// the folder shows in the listing, which then chooses it.
type creating struct {
	dir   string
	first string // the name it was given
	draft string
	fresh bool // not shown yet: the input takes the focus when it shows
	held  bool // the input has had the focus
	busy  bool // the netdisk making it
	made  string
	loads int // the listing's loads when it was made
}

// newFolder starts naming a new folder in l, at its top, coming from a
// box in the window.
func (a *App) newFolder(l *listing, from ui.Rect) {
	if m := a.creating; m != nil && m.busy {
		return
	}
	a.renaming, a.slow = nil, slowClick{}
	name := unusedName(l, "新建文件夹")
	a.creating = &creating{dir: l.dir, first: name, draft: name, fresh: true}
	a.fly(l.dir, draftKey(l.dir), baidu.File{Name: name, IsDir: true}, from)
	l.sel.Clear()
	l.selected = -1
	l.list.ScrollTo(0, ui.Start)
	l.grid.scroll.Y = 0
}

// unusedName is name, or name numbered as freeName does when l already
// has an item of that name.
func unusedName(l *listing, name string) string {
	taken := map[string]bool{}
	for _, f := range l.files {
		if f.FsID != draftID {
			taken[strings.ToLower(f.Name)] = true
		}
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

// placeDraft puts the folder being named at the top of l's files, and
// takes it out once it is made or given up.
func (a *App) placeDraft(l *listing) {
	if i := l.index(draftID); i >= 0 {
		l.files = append(l.files[:i:i], l.files[i+1:]...)
	}
	m := a.creatingIn(l)
	if m == nil || !l.loaded {
		return
	}
	draft := baidu.File{FsID: draftID, IsDir: true, Name: m.draft, Path: path.Join(l.dir, m.draft)}
	l.files = append([]baidu.File{draft}, l.files...)
}

// settleMaker ends naming a folder once the listing read after making it
// has come in, choosing the folder, which goes from the top to its place.
func (a *App) settleMaker(l *listing) {
	m := a.creatingIn(l)
	if m == nil || m.made == "" || l.loads <= m.loads {
		return
	}
	a.creating = nil
	i := l.indexNamed(m.made)
	if i < 0 {
		return
	}
	a.fly(l.dir, l.files[i].Path, l.files[i], l.draftAt)
	l.landing = l.files[i].Path
}

// chooseLanded chooses the folder made once its card has landed: the list
// paints a row chosen itself, under the row hidden while the card flies.
func (a *App) chooseLanded(l *listing, now time.Time) {
	if l.landing == "" {
		return
	}
	if fl := a.flights[l.landing]; fl != nil && fl.progress(now) < 1 {
		return
	}
	i := l.indexKey(l.landing)
	l.landing = ""
	if i < 0 {
		return
	}
	l.sel.Clear()
	l.sel.Add(l.files[i].FsID)
	l.selected = i
	l.refocus = true
	if a.gridView {
		l.grid.pivot = i
	}
}

// makeFolder makes the folder named, unless its name is empty, which
// gives up, or one the netdisk would refuse, which a toast says and,
// after Enter, leaves it to be named again.
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
	case l.indexNamed(name) >= 0 && l.files[l.indexNamed(name)].FsID != draftID:
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

// makerField is the input naming a new folder, where its name shows.
func (a *App) makerField(c *ui.Context, t *ui.Theme, l *listing, center bool) {
	m := a.creating
	pad := t.Space(0.5)
	in := ui.TextInputBase(c.Key("maker-name"), &m.draft).Grow(1).Shrink(1).MinWidth(t.Space(10)).
		Padding(pad, pad*2).Margin(-pad-1, -pad*2-1).Radius(t.Space(1)).Background(t.Background).
		Border(1, t.Accent).TextColor(t.Text).Label("新文件夹的名称")
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
		// A click elsewhere gives up a folder not named yet, and keeps
		// one named.
		if strings.TrimSpace(m.draft) == m.first {
			a.creating = nil
			return
		}
		a.makeFolder(l, false)
	}
}

// makerButton is one of the maker's actions: its icon white on an accent
// disc, its label in the accent, as Reminders' new item.
func makerButton(c *ui.Context, t *ui.Theme, ic, label string) ui.Element {
	b := ui.ButtonBase(c).Label(label).Height(28).PaddingX(8).Gap(8).AlignItems(ui.Center).Radius(7).
		TextColor(t.Accent).FontWeight(500)
	hover := t.Accent.Alpha(0)
	if b.Hovered() {
		hover = t.Accent.Alpha(0.1)
	}
	b.Background(b.AnimateColor("hover", hover, motionHover))
	b.Children(func() {
		ui.Box(c).Size(18, 18).Radius(9).Center().Background(t.Accent).TextColor(t.AccentText).Children(func() {
			ui.Icon(c, icon(ic)).FontSize(12)
		})
		ui.Text(c, label).SingleLine()
	})
	return b
}

// newFolderButton is the maker's button making a folder, the folder being
// named coming from it.
func (a *App) newFolderButton(c *ui.Context, t *ui.Theme, l *listing) ui.Element {
	b := makerButton(c, t, "plus", "新建文件夹")
	if b.Clicked() {
		a.newFolder(l, b.Bounds())
	}
	return b
}

// makerRow is the maker as the row ending the list, or the empty
// folder's.
func (a *App) makerRow(c *ui.Context, t *ui.Theme, p *palette, l *listing, center bool) ui.Element {
	row := ui.Row(c.Key("maker")).Height(rowHeight + 4).PaddingX(2).Gap(4).AlignItems(ui.Center).Radius(8)
	if center {
		row.Justify(ui.Center)
	}
	fill := t.Accent.Alpha(0)
	if l.over {
		fill = t.Accent
	}
	row.Background(row.AnimateColor("fill", fill, motionFade))
	if b := row.Bounds(); b.W > 0 {
		l.makerAt = b
	}
	row.Children(func() {
		if l.over {
			ui.Row(c).Grow(1).Gap(8).AlignItems(ui.Center).Justify(ui.Center).TextColor(t.AccentText).Children(func() {
				ui.Icon(c, icon("upload")).FontSize(14)
				ui.Text(c, "松开以上传到「"+dirLabel(l.dir)+"」").FontWeight(600).SingleLine()
			})
			return
		}
		a.newFolderButton(c, t, l)
		up := makerButton(c, t, "upload", "上传文件")
		if up.Clicked() {
			a.pickUpload(l.dir, up.Bounds())
		}
	})
	return row
}

// makerTile is the maker as the grid's last tile.
func (a *App) makerTile(c *ui.Context, t *ui.Theme, p *palette, l *listing, e ui.Element) {
	e.Padding(10).Gap(8).AlignItems(ui.Center).Radius(t.Radius).Label("新建")
	fill := t.Background.Mix(t.Accent, 0)
	if l.over {
		fill = t.Background.Mix(t.Accent, 0.16)
	}
	e.Background(e.AnimateColor("fill", fill, motionFade))
	if b := e.Bounds(); b.W > 0 {
		l.makerAt = b
	}
	e.Children(func() {
		box := ui.Box(c).Width(124).Height(112).Radius(12).Center().Background(p.Group)
		box.Children(func() {
			ic := "plus"
			if l.over {
				ic = "upload"
			}
			ui.Box(c).Size(44, 44).Radius(22).Center().Background(t.Accent).TextColor(t.AccentText).Children(func() {
				ui.Icon(c, icon(ic)).FontSize(22)
			})
		})
		if l.over {
			ui.Text(c, "松开以上传").TextColor(t.Accent).FontWeight(600)
			return
		}
		ui.Column(c).Gap(0).AlignItems(ui.Center).Children(func() {
			a.newFolderButton(c, t, l)
			up := makerButton(c, t, "upload", "上传文件")
			if up.Clicked() {
				a.pickUpload(l.dir, up.Bounds())
			}
		})
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
		a.makerRow(c, t, p, l, true).Width(320).Margin(14, 0, 0)
	})
}
