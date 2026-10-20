package app

import (
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// showInfo opens the details of files, as Finder's Get Info: a folder's
// listing is read for what it holds.
func (a *App) showInfo(dir string, files []baidu.File) {
	a.ask(dlgInfo, dir, files)
	if len(files) == 1 && files[0].IsDir {
		a.listingFor(files[0].Path)
	}
}

// infoDialog shows what the netdisk knows of the files in the dialog: of
// one, its kind, size, place, times and checksum; of several, how many and
// how large.
func (a *App) infoDialog(c *ui.Context, t *ui.Theme, p *palette) {
	d := &a.dlg
	files := d.files
	one := len(files) == 1
	f := files[0]
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(48, 48).Center().Shrink(0).Children(func() {
			if one {
				a.fileIcon(c, t, p, &f, 48)
			} else {
				ui.Icon(c, icon("layers")).FontSize(36).TextColor(t.TextMuted)
			}
		})
		ui.Column(c).Grow(1).Shrink(1).Gap(3).Children(func() {
			title := itoa(len(files)) + " 项"
			kind := countKinds(files)
			if one {
				title, kind = f.Name, kindLabel(&f)
			}
			ui.Text(c, title).FontSize(15).FontWeight(600).Selectable()
			ui.Text(c, kind).TextColor(t.TextMuted).SingleLine()
		})
	})
	ui.Column(c).Padding(4, 12).Radius(10).Background(p.Group).Dividers(1, p.Hairline).Children(func() {
		row := func(label, value string) {
			ui.Row(c).Padding(7, 0).Gap(12).AlignItems(ui.Start).Children(func() {
				ui.Text(c, label).Width(64).Shrink(0).TextColor(t.TextMuted)
				ui.Text(c, value).Grow(1).Shrink(1).FontFeatures("tnum").Selectable()
			})
		}
		switch {
		case one && f.IsDir:
			row("包含", a.folderHolds(f.Path))
		case one:
			row("大小", exactSize(f.Size))
		default:
			var size int64
			for _, f := range files {
				size += f.Size
			}
			row("大小", exactSize(size)+map[bool]string{true: "（不含文件夹）", false: ""}[strings.Contains(countKinds(files), "文件夹")])
		}
		row("位置", dirPathLabel(path.Dir(f.Path)))
		if one {
			if f.Ctime > 0 {
				row("创建时间", fullTime(f.Ctime))
			}
			if f.Mtime > 0 {
				row("修改时间", fullTime(f.Mtime))
			}
			if !f.IsDir && f.MD5 != "" {
				row("MD5", f.MD5)
			}
		}
	})
	ui.Row(c).Gap(8).Justify(ui.End).Margin(4, 0, 0).Children(func() {
		if one && pill(c, t, p, "copy", "复制路径", false).Clicked() {
			a.copy(f.Path, "已复制路径")
		}
		if pill(c, t, p, "", "完成", true).AutoFocus().Clicked() {
			d.open = false
		}
	})
}

// folderHolds says what a folder holds, from its listing.
func (a *App) folderHolds(dir string) string {
	l := a.listingFor(dir)
	switch {
	case !l.loaded && l.err != "":
		return "无法读取：" + l.err
	case !l.loaded:
		return "正在读取…"
	case len(l.files) == 0:
		return "空文件夹"
	}
	s := itoa(len(l.files)) + " 项"
	var size int64
	files := 0
	for _, f := range l.files {
		if !f.IsDir {
			files++
			size += f.Size
		}
	}
	if files > 0 {
		s += "，其中 " + itoa(files) + " 个文件共 " + humanSize(size)
	}
	return s
}

// countKinds counts the folders and files of a choice.
func countKinds(files []baidu.File) string {
	dirs := 0
	for _, f := range files {
		if f.IsDir {
			dirs++
		}
	}
	var s []string
	if dirs > 0 {
		s = append(s, itoa(dirs)+" 个文件夹")
	}
	if n := len(files) - dirs; n > 0 {
		s = append(s, itoa(n)+" 个文件")
	}
	return strings.Join(s, "，")
}

// exactSize is a size both short and in bytes, as Finder's info shows it.
func exactSize(n int64) string {
	if n < 1000 {
		return itoa(int(n)) + " 字节"
	}
	return humanSize(n) + "（" + groupDigits(n) + " 字节）"
}

// groupDigits writes n with a comma every three digits.
func groupDigits(n int64) string {
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// dirPathLabel is a folder's path as the browser names it: under 我的文件.
func dirPathLabel(dir string) string {
	if dir == "/" {
		return "我的文件"
	}
	return "我的文件" + dir
}

func fullTime(unix int64) string {
	return time.Unix(unix, 0).Format("2006年1月2日 15:04:05")
}
