package app

import (
	"context"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// dialogKind is what the open dialog asks.
type dialogKind int

const (
	dlgNone dialogKind = iota
	dlgNewFolder
	dlgRename
	dlgMove
	dlgCopy
	dlgShare
	dlgSaveLink
	dlgOffline
	dlgInfo
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
		if f.FsID >= 0 && l.sel.Has(f.FsID) {
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

// remove moves files to the recycle bin at once, as Finder does, with no
// question first: they dissolve where they are, and a toast offers to
// undo it. It works apart from the dialogs (a.run), which another dialog
// opened meanwhile could be closed by.
func (a *App) remove(dir string, files []baidu.File) {
	s := a.sess
	if s == nil || len(files) == 0 {
		return
	}
	ids := make([]int64, len(files))
	for i, f := range files {
		ids[i] = f.FsID
	}
	if l := a.listings[dir]; l != nil {
		// They dissolve at once, then leave; the reload confirms it.
		a.dissolve(l, ids)
		l.sel.Clear()
	}
	go func() {
		err := s.client.Delete(context.Background(), paths(files))
		a.update(func() {
			a.reload(dir)
			if err != nil {
				a.spare(ids)
				a.toast(err.Error(), "error")
				return
			}
			a.pendingToasts = append(a.pendingToasts, ui.Toast{
				Title: "已将" + countLabel(files) + "移到回收站", Type: "success", Action: "撤销", Timeout: undoToast,
				OnAction: func() {
					a.spare(ids)
					a.run(func(ctx context.Context, c *baidu.Client) error { return c.Restore(ctx, ids) }, []string{dir}, "已还原", nil)
				},
			})
			a.recycleDirty = true
		})
	}()
}

// dialogs builds the dialog open, if any.
func (a *App) dialogs(c *ui.Context, t *ui.Theme, p *palette) {
	d := &a.dlg
	if d.kind == dlgNone {
		return
	}
	open := d.open
	ui.DialogBase(c, &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(p.Scrim)
		panel.Width(420).Padding(20).Gap(14).Radius(14).Background(p.Card).Border(1, p.Hairline).
			Shadow(0, 16, 40, -12, p.Shadow).Transition(popIn)
		switch d.kind {
		case dlgNewFolder:
			a.nameDialog(c, t, p, "新建文件夹", "在「"+dirLabel(d.dir)+"」中", "创建")
		case dlgRename:
			a.nameDialog(c, t, p, "重命名", d.files[0].Name, "重命名")
		case dlgMove, dlgCopy:
			a.pickerDialog(c, t, p)
		case dlgShare:
			a.shareDialog(c, t, p)
		case dlgSaveLink:
			a.saveLinkDialog(c, t, p)
		case dlgOffline:
			a.offlineDialog(c, t, p)
		case dlgInfo:
			a.infoDialog(c, t, p)
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
		ui.Text(c, title).FontSize(15).FontWeight(600)
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
		if d.kind == dlgMove || d.kind == dlgCopy || d.kind == dlgShare {
			b.AutoFocus() // Enter presses it
		}
		if danger {
			b.Background(t.Danger).TextColor(ui.Hex("#ffffff"))
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

// fileActions handles the browser's keys on the files chosen.
func (a *App) fileActions(c *ui.Context, l *listing) {
	if a.dlg.open || a.pv != nil {
		return
	}
	files := l.chosen()
	if l.dir != "/" && c.Shortcut(ui.Alt, ui.KeyUp) {
		a.router.Push(filesRoute(path.Dir(l.dir)))
	}
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
		a.remove(l.dir, files)
	}
	if c.Shortcut(ui.Cmd, ui.KeyI) {
		a.showInfo(l.dir, files)
	}
	if c.Shortcut(0, ui.KeyEscape) {
		l.sel.Clear()
	}
}

// selBarLabels is the page's width under which the selection bar's
// actions drop their labels and keep only their icons.
const selBarLabels = 660

// selectionBar floats over the browser while files are chosen, with what
// can be done to them. On a page narrower than selBarLabels (last frame's
// width) the actions show only their icons, so the bar never squeezes the
// count into a column.
func (a *App) selectionBar(c *ui.Context, t *ui.Theme, p *palette, l *listing, width float32) {
	files := l.chosen()
	if len(files) == 0 {
		return
	}
	labels := width == 0 || width >= selBarLabels
	bar := ui.Row(c).Key("selbar").Absolute().Bottom(40).Attach(ui.AnchorBottom, ui.AnchorBottom).
		Padding(4, 4, 4, 14).Gap(2).Radius(12).AlignItems(ui.Center).
		Background(p.Card).Border(1, p.Hairline).Shadow(0, 8, 24, -10, p.Shadow).
		Transition(ui.ElementTransition{Animation: &motionFade, Enter: &ui.Motion{Y: 8, Opacity: 0}, Exit: &ui.Motion{Opacity: 0}})
	bar.Children(func() {
		ui.Text(c, "已选 "+itoa(len(files))+" 项").FontWeight(500).Margin(0, 10, 0, 0).FontFeatures("tnum").
			SingleLine().Shrink(0).ContentTransition(ui.ContentTransition{Numeric: true})
		button := func(ic, label string) ui.Element {
			b := ui.ButtonBase(c).Label(label).Height(28).PaddingX(10).Radius(7).Gap(6).AlignItems(ui.Center).Shrink(0)
			if !labels {
				b.Tooltip(label).PaddingX(7)
			}
			b.Children(func() {
				ui.Icon(c, icon(ic)).FontSize(15)
				if labels {
					ui.Text(c, label).SingleLine()
				}
			})
			return b
		}
		action := func(ic, label string) bool {
			b := button(ic, label)
			if b.Hovered() {
				b.Background(p.Hover)
			}
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
		if action("info", "详情") {
			a.showInfo(l.dir, files)
		}
		del := button("trash-2", "删除").TextColor(t.Danger)
		if del.Hovered() {
			del.Background(t.Danger.Alpha(0.1))
		}
		if del.Clicked() {
			a.remove(l.dir, files)
		}
		ui.Divider(c).Height(20).Margin(0, 4)
		if iconButton(c, t, p, "x", "取消选择").Clicked() {
			l.sel.Clear()
		}
	})
}
