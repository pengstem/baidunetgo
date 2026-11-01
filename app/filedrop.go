package app

import (
	"path"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

type fileDrag struct {
	from  string
	files []baidu.File
}

// In a list the icon is the drag handle, leaving drags over names to
// range selection. Grid tiles can be dragged from anywhere but the editor.
func (a *App) dragFile(e ui.Element, l *listing, f *baidu.File, chosen []baidu.File) {
	if f.FsID < 0 || a.dying[f.FsID] != nil || a.dlg.open || a.creating != nil || a.renaming != nil {
		return
	}
	files := []baidu.File{*f}
	if l.sel.Has(f.FsID) {
		files = chosen
	}
	e.Drag(fileDrag{from: l.dir, files: files}).Tooltip("拖到文件夹以移动")
	if e.Dragging() {
		a.slow = slowClick{}
		e.Opacity(0.55)
	}
}

func validFileDrop(d fileDrag, to string) bool {
	if len(d.files) == 0 || d.from == to {
		return false
	}
	for _, f := range d.files {
		if f.FsID < 0 || f.Path == to || (f.IsDir && strings.HasPrefix(to, f.Path+"/")) {
			return false
		}
	}
	return true
}

// Only real folders accept drops. Local files upload; a typed in-window
// drag moves through the existing conflict-resolution flow.
func (a *App) folderDrop(e ui.Element, l *listing, f *baidu.File) bool {
	if !f.IsDir || f.FsID < 0 || a.dying[f.FsID] != nil || a.dlg.open || a.dlg.busy || a.creating != nil || a.renaming != nil {
		return false
	}
	over := e.FileDragOver()
	if over {
		a.dropHint = "松开以上传到「" + f.Name + "」"
	}
	if locals := e.DroppedFiles(); len(locals) > 0 {
		a.upload(locals, f.Path)
	}
	if d, ok := ui.DragOver[fileDrag](e); ok {
		over = true
		if validFileDrop(d, f.Path) {
			a.dropHint = "松开以移动" + countLabel(d.files) + "到「" + f.Name + "」"
		} else {
			a.dropHint = "不能移动到自身或自己的子文件夹"
			over = false
		}
	}
	if d, ok := ui.Drop[fileDrag](e); ok && validFileDrop(d, f.Path) {
		// A closed operation state lets conflict handling show its own
		// dialog, without opening the destination picker first.
		a.dlg = dialog{kind: dlgMove, dir: d.from, files: d.files}
		a.moveOrCopy(dlgMove, d.files, d.from, path.Clean(f.Path))
	}
	return over
}
