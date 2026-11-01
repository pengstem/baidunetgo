package app

import (
	"context"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// Upload placeholders use small negative IDs; this one belongs only to
// the folder being named. It is never sent to the server or dragged.
const draftFolderID int64 = -1 << 63

type folderDraft struct {
	dir, name, err string
	busy, fresh    bool
	selectName     bool
}

func (a *App) newFolder(dir string) {
	if a.dlg.open || a.dlg.busy || a.renaming != nil || a.creating != nil {
		return
	}
	l := a.listingFor(dir)
	if !l.loaded {
		return
	}
	name := "新建文件夹"
	taken := map[string]bool{}
	for _, f := range l.files {
		taken[strings.ToLower(f.Name)] = true
	}
	if taken[strings.ToLower(name)] {
		name = freeName(name, taken)
	}
	a.creating = &folderDraft{dir: dir, name: name, fresh: true, selectName: true}
	a.slow = slowClick{}
	l.sel.Clear()
	l.selected = -1
	l.list.ScrollIntoView(0)
	l.grid.scroll.Y = 0
}

// The draft stays first through refreshes and sort changes. Its eventual
// server entry takes its normal sorted place only after creation succeeds.
func (a *App) folderDraftRow(l *listing) {
	if d := a.creating; d != nil && d.dir == l.dir && len(l.files) > 0 && l.files[0].FsID == draftFolderID {
		l.files[0].Name = d.name
		return
	}
	l.files = slices.DeleteFunc(l.files, func(f baidu.File) bool { return f.FsID == draftFolderID })
	if d := a.creating; d != nil && d.dir == l.dir {
		l.files = slices.Insert(l.files, 0, baidu.File{FsID: draftFolderID, Name: d.name, IsDir: true})
	}
}

func (a *App) cancelFolder(l *listing) {
	a.creating = nil
	// Row callbacks may still be iterating this slice; the next frame
	// removes the draft with folderDraftRow.
	l.sel.Remove(draftFolderID)
	l.selected = -1
	l.refocus = true
}

func (a *App) folderNameField(c *ui.Context, t *ui.Theme, l *listing, center bool) {
	d := a.creating
	in := ui.TextInputBase(c.Key("new-folder-name"), &d.name).Grow(1).MinWidth(48).
		Padding(2, 4).Radius(4).Background(t.Background).Border(1, t.Accent).
		TextColor(t.Text).Label("新文件夹名称").Disabled(d.busy)
	if center {
		in.TextAlign(ui.Center)
	}
	if d.err != "" {
		in.BorderColor(t.Danger).Tooltip(d.err)
	}
	if d.fresh && !d.busy {
		d.fresh = false
		in.Focus()
		if d.selectName {
			in.SetTextSelection(0, utf8.RuneCountInString(d.name))
			d.selectName = false
		}
		c.Invalidate()
	}
	if !d.busy && in.Submitted() {
		a.createFolder(l)
	}
	if !d.busy && c.Shortcut(0, ui.KeyEscape) {
		a.cancelFolder(l)
	}
}

func (a *App) createFolder(l *listing) {
	d, s := a.creating, a.sess
	if d == nil || d.busy || s == nil {
		return
	}
	name := strings.TrimSpace(d.name)
	switch {
	case name == "", name == ".", name == "..":
		d.err = "请输入有效的文件夹名称"
	case strings.ContainsAny(name, badNameChars):
		d.err = `名称不能包含 / \ : * ? " < > |`
	default:
		d.err = ""
		for _, f := range l.files {
			if f.FsID != draftFolderID && strings.EqualFold(f.Name, name) {
				d.err = "这里已有名为「" + name + "」的项目"
				break
			}
		}
	}
	if d.err != "" {
		d.fresh = true
		return
	}
	d.name, d.busy = name, true
	order := a.order
	go func() {
		err := s.client.Mkdir(context.Background(), path.Join(d.dir, name))
		// Read back the server ID before removing the temporary row, so a
		// subsequent refresh cannot briefly make the created folder vanish.
		var files []baidu.File
		var readErr error
		if err == nil {
			files, readErr = s.client.List(context.Background(), d.dir, order)
		}
		a.update(func() {
			d.busy = false
			if a.creating != d || a.sess != s {
				return
			}
			if err != nil {
				d.err, d.fresh = err.Error(), true
				return
			}
			a.cancelFolder(l)
			if readErr != nil {
				a.reload(d.dir)
				a.toast("文件夹已创建，暂时无法刷新："+readErr.Error(), "error")
				return
			}
			l.files, l.sorted = a.keepDying(l.dir, files), a.order
			sortFiles(l.files, a.order)
			l.loads++
			for i, f := range l.files {
				if f.Name == name {
					l.sel.Add(f.FsID)
					l.selected = i
					l.list.ScrollIntoView(i)
					l.grid.scrollIntoView(i, a.gridSize[1])
					break
				}
			}
			a.toast("已新建「"+name+"」", "success")
		})
	}()
}

func (a *App) emptyFolder(c *ui.Context, t *ui.Theme, p *palette, dir string) {
	ui.Column(c).Grow(1).Center().Gap(8).Padding(32).Transition(popIn).Children(func() {
		ui.Box(c).Size(52, 52).Radius(14).Center().Background(p.Group).Children(func() {
			ui.Icon(c, icon("folder-open")).FontSize(24).TextColor(p.Faint)
		})
		ui.Text(c, "这里还空着").FontSize(15).FontWeight(600)
		ui.Text(c, "把文件拖到这里，或选择下面的操作").TextColor(t.TextMuted)
		ui.Row(c).Gap(8).Margin(10, 0, 0).Children(func() {
			if pill(c, t, p, "upload", "上传文件", true).Clicked() {
				a.pickUpload(dir)
			}
			if pill(c, t, p, "folder-plus", "新建文件夹", false).Label("在此新建文件夹").Clicked() {
				a.newFolder(dir)
			}
		})
	})
}

func (a *App) folderDraftActions(c *ui.Context, t *ui.Theme, p *palette, l *listing) {
	if d := a.creating; d != nil && d.dir == l.dir {
		ui.Row(c).Padding(5, 20).Gap(8).AlignItems(ui.Center).Children(func() {
			if d.busy {
				ui.Spinner(c).Label("正在创建文件夹")
				ui.Text(c, "正在创建…").TextColor(t.TextMuted)
			} else {
				if d.err != "" {
					ui.Text(c, d.err).TextColor(t.Danger).SingleLine().Tooltip(d.err).Grow(1)
				} else {
					ui.Text(c, "Enter 创建 · Esc 取消").TextColor(t.TextMuted).Grow(1)
				}
				if pill(c, t, p, "", "取消新建", false).Clicked() {
					a.cancelFolder(l)
				}
				if pill(c, t, p, "check", "创建文件夹", true).Clicked() {
					a.createFolder(l)
				}
			}
		})
	}
}
