package app

import (
	"context"
	"path"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// dialogKind is what the open dialog asks.
type dialogKind int

const (
	dlgNone dialogKind = iota
	dlgNewFolder
	dlgRename
	dlgDelete
	dlgMove
	dlgCopy
	dlgShare
	dlgSaveLink
	dlgOffline
)

// dialog is the dialog over the window, if any.
type dialog struct {
	kind  dialogKind
	open  bool
	name  string
	files []baidu.File
	dir   string // the folder it acts in
	busy  bool
	err   string

	picker picker // the destination of a move or copy
}

// chosen returns the files chosen in a listing, the row under the pointer
// when it was right-clicked outside the choice included.
func (l *listing) chosen() []baidu.File {
	var out []baidu.File
	for _, f := range l.files {
		if l.sel.Has(f.FsID) {
			out = append(out, f)
		}
	}
	return out
}

func (a *App) ask(kind dialogKind, dir string, files []baidu.File) {
	a.dlg = dialog{kind: kind, open: true, dir: dir, files: files}
	switch kind {
	case dlgRename:
		a.dlg.name = files[0].Name
	case dlgNewFolder:
		a.dlg.name = "新建文件夹"
	case dlgMove, dlgCopy:
		a.dlg.picker = picker{dir: dir}
	}
}

// run does an operation on the netdisk, then reloads the folders it
// changed and says how it went.
func (a *App) run(op func(ctx context.Context, c *baidu.Client) error, dirs []string, done string, after func()) {
	s := a.sess
	if s == nil {
		return
	}
	a.dlg.busy, a.dlg.err = true, ""
	go func() {
		err := op(context.Background(), s.client)
		a.update(func() {
			a.dlg.busy = false
			if err != nil {
				a.dlg.err = err.Error()
				if !a.dlg.open {
					a.toast(err.Error(), "error")
				}
				return
			}
			a.dlg.open = false
			for _, d := range dirs {
				a.reload(d)
			}
			if after != nil {
				after()
			}
			if done != "" {
				a.toast(done, "success")
			}
		})
	}()
}

func paths(files []baidu.File) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path
	}
	return out
}

func countLabel(files []baidu.File) string {
	if len(files) == 1 {
		return "「" + files[0].Name + "」"
	}
	return itoa(len(files)) + " 项"
}

// remove deletes files, offering to undo it from the recycle bin.
func (a *App) remove(dir string, files []baidu.File) {
	ids := make([]int64, len(files))
	for i, f := range files {
		ids[i] = f.FsID
	}
	if l := a.listings[dir]; l != nil {
		// Take them out at once; the reload confirms it.
		l.files = slices.DeleteFunc(l.files, func(f baidu.File) bool { return slices.Contains(ids, f.FsID) })
		l.sel.Clear()
	}
	a.run(func(ctx context.Context, c *baidu.Client) error {
		return c.Delete(ctx, paths(files))
	}, []string{dir}, "", func() {
		a.pendingToasts = append(a.pendingToasts, ui.Toast{
			Title: "已删除 " + countLabel(files), Type: "success", Action: "撤销", Timeout: 7 * time.Second,
			OnAction: func() {
				a.run(func(ctx context.Context, c *baidu.Client) error { return c.Restore(ctx, ids) }, []string{dir}, "已还原", nil)
			},
		})
		a.recycleDirty = true
	})
}

// dialogs builds the dialog open, if any.
func (a *App) dialogs(c *ui.Context, t *ui.Theme, p *palette) {
	d := &a.dlg
	if d.kind == dlgNone {
		return
	}
	open := d.open
	ui.DialogBase(c, &d.open, func(backdrop, panel ui.Element) {
		backdrop.Material(glass.Blur{Radius: 10}).Background(p.Scrim)
		panel.Width(420).Padding(24).Gap(16).Radius(18).Background(p.Card).Border(1, p.Hairline).
			Shadow(0, 30, 70, -20, p.Shadow).Transition(popIn)
		switch d.kind {
		case dlgNewFolder:
			a.nameDialog(c, t, p, "新建文件夹", "在「"+dirLabel(d.dir)+"」中", "创建")
		case dlgRename:
			a.nameDialog(c, t, p, "重命名", d.files[0].Name, "重命名")
		case dlgDelete:
			a.deleteDialog(c, t, p)
		case dlgMove, dlgCopy:
			a.pickerDialog(c, t, p)
		case dlgShare:
			a.shareDialog(c, t, p)
		case dlgSaveLink:
			a.saveLinkDialog(c, t, p)
		case dlgOffline:
			a.offlineDialog(c, t, p)
		}
	})
	if open && !d.open && d.busy {
		d.open = true // keep it while the netdisk works
	}
}

func dirLabel(dir string) string {
	if dir == "/" {
		return "我的文件"
	}
	return path.Base(dir)
}

func dialogTitle(c *ui.Context, t *ui.Theme, title, detail string) {
	ui.Column(c).Gap(4).Children(func() {
		ui.Text(c, title).FontSize(17).FontWeight(650)
		if detail != "" {
			ui.Text(c, detail).TextColor(t.TextMuted).SingleLine()
		}
	})
}

// dialogButtons are the dialog's buttons, the main one last; it reports
// whether the main one was pressed.
func (a *App) dialogButtons(c *ui.Context, t *ui.Theme, p *palette, label string, danger, disabled bool) bool {
	d := &a.dlg
	if d.err != "" {
		ui.Text(c, d.err).TextColor(t.Danger).Transition(rowEnter)
	}
	pressed := false
	ui.Row(c).Gap(8).Justify(ui.End).Margin(4, 0, 0).Children(func() {
		if pill(c, t, p, "", "取消", false).Disabled(d.busy).Clicked() {
			d.open = false
		}
		b := pill(c, t, p, "", label, true).Disabled(disabled || d.busy)
		if d.kind == dlgDelete || d.kind == dlgMove || d.kind == dlgCopy || d.kind == dlgShare {
			b.AutoFocus() // Enter presses it
		}
		if danger {
			b.Background(t.Danger).Shadow(0, 4, 12, -4, t.Danger.Alpha(0.6))
		}
		if d.busy {
			b.Children(func() { ui.Spinner(c).TextColor(t.AccentText) })
		}
		pressed = b.Clicked()
	})
	return pressed
}

func (a *App) nameDialog(c *ui.Context, t *ui.Theme, p *palette, title, detail, action string) {
	d := &a.dlg
	dialogTitle(c, t, title, detail)
	in := ui.TextInput(c, &d.name).AutoFocus().Label("名称").Height(38)
	if d.kind == dlgRename && !d.busy && in.Focused() && d.err == "" {
		// Choose the name without its extension, as Finder does.
		if s, e := in.TextSelection(); s == e && s == utf8.RuneCountInString(d.name) {
			stem := strings.TrimSuffix(d.name, path.Ext(d.name))
			if d.files[0].IsDir || stem == "" {
				stem = d.name
			}
			in.SetTextSelection(0, utf8.RuneCountInString(stem))
		}
	}
	name := strings.TrimSpace(d.name)
	valid := name != "" && !strings.ContainsAny(name, `/\:*?"<>|`)
	if d.err == "" && name != "" && !valid {
		ui.Text(c, `名称不能包含 / \ : * ? " < > |`).TextColor(t.Warning).FontSize(t.Rem(0.88))
	}
	submit := in.Submitted() && valid
	if a.dialogButtons(c, t, p, action, false, !valid) || submit {
		switch d.kind {
		case dlgNewFolder:
			dir, target := d.dir, path.Join(d.dir, name)
			a.run(func(ctx context.Context, c *baidu.Client) error { return c.Mkdir(ctx, target) }, []string{dir}, "已新建「"+name+"」", nil)
		case dlgRename:
			f, dir := d.files[0], d.dir
			if name == f.Name {
				d.open = false
				return
			}
			a.run(func(ctx context.Context, c *baidu.Client) error { return c.Rename(ctx, f.Path, name) }, []string{dir}, "", nil)
		}
	}
}

func (a *App) deleteDialog(c *ui.Context, t *ui.Theme, p *palette) {
	d := &a.dlg
	ui.Row(c).Gap(14).AlignItems(ui.Start).Children(func() {
		ui.Box(c).Size(40, 40).Radius(12).Center().Background(t.Danger.Alpha(0.12)).Children(func() {
			ui.Icon(c, icon("trash-2")).FontSize(19).TextColor(t.Danger)
		})
		ui.Column(c).Grow(1).Shrink(1).Children(func() {
			dialogTitle(c, t, "删除"+countLabel(d.files)+"？", "")
			ui.Text(c, "文件会移到回收站，保留一段时间后自动清除。").TextColor(t.TextMuted).Margin(4, 0, 0)
		})
	})
	if a.dialogButtons(c, t, p, "删除", true, false) {
		d.open = false
		a.remove(d.dir, d.files)
	}
}

// fileActions handles the browser's keys on the files chosen.
func (a *App) fileActions(c *ui.Context, l *listing) {
	if a.dlg.open || a.pv != nil {
		return
	}
	files := l.chosen()
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyN) {
		a.ask(dlgNewFolder, l.dir, nil)
	}
	if len(files) == 0 {
		return
	}
	if c.Shortcut(0, ui.KeyF2) && len(files) == 1 {
		a.ask(dlgRename, l.dir, files)
	}
	if c.Shortcut(0, ui.KeyDelete) || c.Shortcut(ui.Cmd, ui.KeyBackspace) {
		a.ask(dlgDelete, l.dir, files)
	}
	if c.Shortcut(0, ui.KeyEscape) {
		l.sel.Clear()
	}
}

// selectionBar floats over the browser while files are chosen, with what
// can be done to them.
func (a *App) selectionBar(c *ui.Context, t *ui.Theme, p *palette, l *listing) {
	files := l.chosen()
	if len(files) == 0 {
		return
	}
	bar := ui.Row(c).Key("selbar").Absolute().Bottom(22).Attach(ui.AnchorBottom, ui.AnchorBottom).
		Padding(6, 6, 6, 16).Gap(4).Radius(16).AlignItems(ui.Center).
		Material(glass.Glass{}).Shadow(0, 18, 40, -14, p.Shadow).
		Transition(ui.ElementTransition{Animation: &springPop, Enter: &ui.Motion{Y: 30, Scale: 0.9, Opacity: 0}, Exit: &ui.Motion{Y: 20, Opacity: 0}})
	bar.Children(func() {
		ui.Text(c, "已选 "+itoa(len(files))+" 项").FontWeight(600).Margin(0, 10, 0, 0).FontFeatures("tnum").
			ContentTransition(ui.ContentTransition{Numeric: true})
		action := func(ic, label string) bool {
			b := ui.ButtonBase(c).Label(label).Height(34).PaddingX(10).Radius(10).Gap(6).AlignItems(ui.Center)
			if b.Hovered() {
				b.Background(p.Hover)
			}
			b.Children(func() {
				ui.Icon(c, icon(ic)).FontSize(15)
				ui.Text(c, label).SingleLine()
			})
			return b.Clicked()
		}
		if action("download", "下载") {
			a.download(files)
		}
		if action("share-2", "分享") {
			a.share(files)
		}
		if action("move", "移动") {
			a.ask(dlgMove, l.dir, files)
		}
		if action("copy", "复制") {
			a.ask(dlgCopy, l.dir, files)
		}
		if len(files) == 1 && action("pencil", "重命名") {
			a.ask(dlgRename, l.dir, files)
		}
		del := ui.ButtonBase(c).Label("删除").Height(34).PaddingX(10).Radius(10).Gap(6).AlignItems(ui.Center).TextColor(t.Danger)
		if del.Hovered() {
			del.Background(t.Danger.Alpha(0.1))
		}
		del.Children(func() {
			ui.Icon(c, icon("trash-2")).FontSize(15)
			ui.Text(c, "删除")
		})
		if del.Clicked() {
			a.ask(dlgDelete, l.dir, files)
		}
		ui.Divider(c).Height(20).Margin(0, 4)
		if iconButton(c, t, p, "x", "取消选择").Clicked() {
			l.sel.Clear()
		}
	})
}
