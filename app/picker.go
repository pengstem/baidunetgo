package app

import (
	"context"
	"path"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// picker chooses a folder of the netdisk, as the destination of a move or
// a copy.
type picker struct {
	dir     string
	folders map[string][]baidu.File
	loading map[string]bool
	list    ui.ListState
}

// pickerFolders returns the folders in dir, loading them the first time.
func (a *App) pickerFolders(pk *picker, dir string) ([]baidu.File, bool) {
	if pk.folders == nil {
		pk.folders, pk.loading = map[string][]baidu.File{}, map[string]bool{}
	}
	if fs, ok := pk.folders[dir]; ok {
		return fs, true
	}
	if l := a.listings[dir]; l != nil && l.loaded {
		var fs []baidu.File
		for _, f := range l.files {
			if f.IsDir {
				fs = append(fs, f)
			}
		}
		pk.folders[dir] = fs
		return fs, true
	}
	if !pk.loading[dir] && a.sess != nil {
		pk.loading[dir] = true
		c := a.sess.client
		go func() {
			files, err := c.List(context.Background(), dir, baidu.Order{By: "name"})
			a.update(func() {
				pk.loading[dir] = false
				var fs []baidu.File
				if err == nil {
					for _, f := range files {
						if f.IsDir {
							fs = append(fs, f)
						}
					}
					sortFiles(fs, baidu.Order{By: "name"})
				}
				pk.folders[dir] = fs
			})
		}()
	}
	return nil, false
}

func (a *App) pickerDialog(c *ui.Context, t *ui.Theme, p *palette) {
	d := &a.dlg
	pk := &d.picker
	verb := "移动"
	if d.kind == dlgCopy {
		verb = "复制"
	}
	dialogTitle(c, t, verb+countLabel(d.files)+"到…", "")
	moving := map[string]bool{}
	for _, f := range d.files {
		moving[f.Path] = true
	}
	a.folderChooser(c, t, p, pk, 260, moving)
	// A folder cannot go into itself, nor stay where it is when moved.
	invalid := false
	for _, f := range d.files {
		if pk.dir == f.Path || strings.HasPrefix(pk.dir, f.Path+"/") || (d.kind == dlgMove && path.Dir(f.Path) == pk.dir) {
			invalid = true
		}
	}
	if a.dialogButtons(c, t, p, verb+"到这里", false, invalid) {
		files, from, to, kind := d.files, d.dir, pk.dir, d.kind
		op := func(ctx context.Context, c *baidu.Client) error {
			if kind == dlgCopy {
				return c.Copy(ctx, paths(files), to)
			}
			return c.Move(ctx, paths(files), to)
		}
		if l := a.listings[from]; l != nil {
			l.sel.Clear()
		}
		a.run(op, []string{from, to}, "已"+verb+"到「"+dirLabel(to)+"」", nil)
	}
}

// folderChooser lets the user go through the netdisk's folders to choose
// one, pk.dir; skip names folders that cannot be chosen.
func (a *App) folderChooser(c *ui.Context, t *ui.Theme, p *palette, pk *picker, height float32, skip map[string]bool) {
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		up := iconButton(c, t, p, "arrow-left", "上一级").Disabled(pk.dir == "/")
		if up.Clicked() {
			pk.dir = path.Dir(pk.dir)
		}
		ui.Icon(c, icon("folder")).FontSize(16).TextColor(p.Folder)
		ui.Text(c, dirLabel(pk.dir)).FontWeight(600).SingleLine().ContentTransition(ui.ContentTransition{})
	})
	box := ui.Column(c).Height(height).Radius(12).Background(t.Surface).Clip()
	box.Children(func() {
		folders, ok := a.pickerFolders(pk, pk.dir)
		switch {
		case !ok:
			ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c).Label("加载中") })
		case len(folders) == 0:
			ui.Column(c).Grow(1).Center().Children(func() {
				ui.Text(c, "没有子文件夹").TextColor(t.TextMuted)
			})
		default:
			ui.List(c, &pk.list, len(folders), func(i int) {
				f := folders[i]
				off := skip[f.Path]
				row := ui.ButtonBase(c.Key(f.Path)).Label("打开 " + f.Name).Height(40).PaddingX(12).Gap(10).Justify(ui.Start).
					AlignItems(ui.Center).Disabled(off)
				if off {
					row.Opacity(0.4)
				} else if row.Hovered() {
					row.Background(p.Hover)
				}
				row.Children(func() {
					ui.Icon(c, icon("folder")).FontSize(17).TextColor(p.Folder)
					ui.Text(c, f.Name).Grow(1).SingleLine()
					ui.Icon(c, icon("chevron-right")).FontSize(14).TextColor(t.TextMuted)
				})
				if row.Clicked() {
					pk.dir = f.Path
				}
			}).Grow(1).Padding(4)
		}
	})
}
