package app

import (
	"context"
	"path"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// renaming is a file's name edited in place, where the list or the grid
// shows it, as in Finder: F2, Enter on macOS, the menu, the selection bar,
// or a slow second click on the name of the file chosen alone start it;
// Enter keeps the name typed, as moving the focus away does, and Escape
// goes back.
//
// ui.EditableText does this in a list, but a click choosing a row starts
// editing it too, as it takes the row for chosen before the click.
type renaming struct {
	dir   string
	id    int64
	draft string
	fresh bool // not shown yet: the input takes the focus when it shows
	held  bool // the input has had the focus
}

// slowClick is a click on the name of the file chosen alone, which edits
// it once a second click would no longer make a double click: at is when,
// set by the frame after the click.
type slowClick struct {
	dir string
	id  int64
	at  time.Time
}

// renameDelay is how long after a click on the name of the file chosen
// editing it starts, unless a second click makes a double click.
const renameDelay = 500 * time.Millisecond

// enterRenames is whether Enter renames the file chosen, as in Finder,
// rather than opening it, as elsewhere.
var enterRenames = runtime.GOOS == "darwin"

const badNameChars = `/\:*?"<>|`

// rename starts editing the name of f, in l, in place.
func (a *App) rename(l *listing, f baidu.File) {
	i := l.index(f.FsID)
	if i < 0 || f.FsID < 0 || a.dying[f.FsID] != nil {
		return
	}
	a.renaming = &renaming{dir: l.dir, id: f.FsID, draft: f.Name, fresh: true}
	a.slow = slowClick{}
	l.sel.Clear()
	l.sel.Add(f.FsID)
	l.selected = i
	if a.gridView {
		l.grid.scrollIntoView(i, a.gridSize[1])
	} else {
		l.list.ScrollIntoView(i)
	}
}

// renamingIn reports whether the name of f, in l, is being edited.
func (a *App) renamingIn(l *listing, f *baidu.File) bool {
	r := a.renaming
	return r != nil && r.dir == l.dir && r.id == f.FsID
}

func (l *listing) index(id int64) int {
	for i := range l.files {
		if l.files[i].FsID == id {
			return i
		}
	}
	return -1
}

// clickName arms a slow click on the name of f, in l, when f was the file
// chosen alone before the click, and a second click disarms it. It only
// watches the presses, which go on to the row or the tile: asking the
// name whether it was clicked would take them.
func (a *App) clickName(l *listing, f *baidu.File, name ui.Element) {
	id := f.FsID
	name.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputPointerDown || ev.Button != 0 {
			return false
		}
		a.slow = slowClick{}
		if ev.Clicks == 1 && ev.Mods == 0 && l.sole == id && a.renaming == nil {
			a.slow = slowClick{dir: l.dir, id: id}
		}
		return false
	})
}

// slowRename starts editing the name a slow click armed, once its time
// has come and its file is still the one chosen alone; a double click
// on it, or another choice, disarms it.
func (a *App) slowRename(c *ui.Context, l *listing) {
	s := &a.slow
	if s.dir != l.dir {
		return
	}
	if l.sel.Len() != 1 || !l.sel.Has(s.id) {
		a.slow = slowClick{}
		return
	}
	if s.at.IsZero() {
		s.at = c.Now().Add(renameDelay)
		c.After(renameDelay)
	}
	if !c.Now().Before(s.at) {
		id := s.id
		a.slow = slowClick{}
		if i := l.index(id); i >= 0 {
			a.rename(l, l.files[i])
		}
	}
}

// nameField is the input editing a file's name in place, where its name
// shows, within the name's padding and border.
func (a *App) nameField(c *ui.Context, t *ui.Theme, l *listing, f *baidu.File, center bool) {
	r := a.renaming
	pad := t.Space(0.5)
	in := ui.TextInputBase(c.Key("rename"), &r.draft).Grow(1).Shrink(1).MinWidth(t.Space(10)).
		Padding(pad, pad*2).Margin(-pad-1, -pad*2-1).Radius(t.Space(1)).
		Background(t.Background).Border(1, t.Accent).TextColor(t.Text).Label("名称")
	if center {
		in.TextAlign(ui.Center)
	}
	switch {
	case r.fresh:
		r.fresh = false
		in.Focus()
		c.Invalidate()
	case in.Focused() && !r.held:
		// The name before its extension, as Finder chooses.
		r.held = true
		stem := strings.TrimSuffix(r.draft, path.Ext(r.draft))
		if f.IsDir || stem == "" {
			stem = r.draft
		}
		in.SetTextSelection(0, utf8.RuneCountInString(stem))
	case in.Submitted():
		a.endRename(l, true, true)
	case in.Shortcut(0, ui.KeyEscape):
		a.endRename(l, false, true)
	case r.held && !in.Focused():
		// The focus went elsewhere: the name typed stays, and the focus
		// with it.
		a.endRename(l, true, false)
	}
}

// endRename ends editing a name, renaming the file on the netdisk when
// keep and the name changed, and gives the focus back to the files when
// refocus. A name the netdisk would refuse keeps it editing after Enter.
func (a *App) endRename(l *listing, keep, refocus bool) {
	r := a.renaming
	if keep && !a.renameTo(l, r.id, r.draft) && refocus {
		return
	}
	a.renaming = nil
	if refocus {
		l.refocus = true
	}
}

// renameTo renames file id, in l, to name, unless the name is the same,
// or one the netdisk would refuse, which a toast says, and reports
// whether it was not refused.
func (a *App) renameTo(l *listing, id int64, name string) bool {
	i := l.index(id)
	name = strings.TrimSpace(name)
	if i < 0 || name == "" || name == l.files[i].Name {
		return true
	}
	why := ""
	if strings.ContainsAny(name, badNameChars) {
		why = `名称不能包含 / \ : * ? " < > |`
	} else if j := l.indexNamed(name); j >= 0 && j != i {
		why = "这里已有名为「" + name + "」的项目"
	}
	if why != "" {
		a.toast(why, "error")
		return false
	}
	// The new name shows at once, sorted to its place; the reload
	// confirms it, or brings the old one back.
	f := l.files[i]
	l.files[i].Name, l.files[i].Path = name, path.Join(path.Dir(f.Path), name)
	l.sorted = baidu.Order{}
	s := a.sess
	if s == nil {
		return true
	}
	dir := l.dir
	go func() {
		err := s.client.Rename(context.Background(), f.Path, name)
		a.update(func() {
			if err != nil {
				a.toast("无法重命名「"+f.Name+"」："+err.Error(), "error")
			}
			a.reload(dir)
		})
	}()
	return true
}

func (l *listing) indexNamed(name string) int {
	for i := range l.files {
		if l.files[i].Name == name {
			return i
		}
	}
	return -1
}
