package app

import (
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/transfer"
)

// transfersState is the transfers page.
type transfersState struct {
	tab   int // 0 going, 1 done
	tasks []transfer.Task
	list  ui.ListState
}

// startTransfers starts the queue of a session.
func (a *App) startTransfers() {
	state := ""
	if !a.fake {
		if dir, err := mygo.App.Path(mygo.PathUserData); err == nil {
			state = filepath.Join(dir, "transfers.json")
		}
	}
	m := transfer.New(a.sess.client, baidu.Expired, state)
	m.Threads, m.Parallel = a.settings.threads(), a.settings.parallel()
	m.OnChange = func() {
		tasks := m.Tasks()
		a.update(func() {
			if a.xfer == m {
				a.transfers.tasks = tasks
			}
		})
	}
	m.OnFinish = func(t transfer.Task) {
		a.update(func() { a.finished(t) })
	}
	a.xfer = m
	a.transfers.tasks = m.Tasks()
}

func (a *App) stopTransfers() {
	if a.xfer != nil {
		a.xfer.Close()
		a.xfer = nil
	}
	a.transfers = transfersState{}
}

// finished tells of a task that ended.
func (a *App) finished(t transfer.Task) {
	if t.Kind == transfer.Upload {
		a.reload(path.Dir(t.Remote))
	}
	if t.State == transfer.Failed {
		a.toast("「"+t.Name+"」传输失败："+t.Err, "error")
		return
	}
	verb := "下载"
	if t.Kind == transfer.Upload {
		verb = "上传"
	}
	if a.win != nil && !a.win.IsFocused() {
		mygo.NewNotification(mygo.NotificationOptions{Title: verb + "完成", Body: t.Name, Group: "transfers"}).Show()
	}
}

// download queues files to download.
func (a *App) download(files []baidu.File) {
	if a.xfer == nil || len(files) == 0 {
		return
	}
	a.xfer.AddDownloads(files, a.settings.downloadDir())
	a.pendingToasts = append(a.pendingToasts, ui.Toast{
		Title: "已加入下载：" + countLabel(files), Type: "success", Action: "查看",
		OnAction: func() { a.router.Push("/transfers") },
	})
}

// upload queues local files and folders to go into dir.
func (a *App) upload(locals []string, dir string) {
	if a.xfer == nil || len(locals) == 0 {
		return
	}
	a.xfer.AddUploads(locals, dir)
	n := itoa(len(locals)) + " 项"
	if len(locals) == 1 {
		n = "「" + filepath.Base(locals[0]) + "」"
	}
	a.pendingToasts = append(a.pendingToasts, ui.Toast{
		Title: "正在上传" + n + "到「" + dirLabel(dir) + "」", Type: "success", Action: "查看",
		OnAction: func() { a.router.Push("/transfers") },
	})
}

// pickUpload asks for files to upload into dir.
func (a *App) pickUpload(dir string) {
	win := a.win
	go func() {
		files, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: "选择要上传的文件", ButtonLabel: "上传", Multiple: true})
		if err != nil || len(files) == 0 {
			return
		}
		a.update(func() { a.upload(files, dir) })
	}()
}

// activeTransfers counts the tasks not finished.
func (a *App) activeTransfers() int {
	n := 0
	for _, t := range a.transfers.tasks {
		if t.State == transfer.Running || t.State == transfer.Queued {
			n++
		}
	}
	return n
}

func (a *App) transfersPage(c *ui.Context, t *ui.Theme, p *palette) {
	x := &a.transfers
	var going, done []transfer.Task
	for _, task := range x.tasks {
		if task.State == transfer.Done {
			done = append(done, task)
		} else {
			going = append(going, task)
		}
	}
	shown := going
	if x.tab == 1 {
		shown = done
	}
	ui.Column(c).Grow(1).Children(func() {
		ui.Row(c).Padding(4, 24, 12).Children(func() {
			tabs(c, t, p, &x.tab, "进行中 "+itoa(len(going)), "已完成 "+itoa(len(done)))
		})
		if len(shown) == 0 {
			if x.tab == 0 {
				emptyState(c, t, p, "arrow-up-down", "没有正在进行的传输", "下载或上传的文件会出现在这里", nil)
			} else {
				emptyState(c, t, p, "circle-check", "还没有完成的传输", "", nil)
			}
			return
		}
		x.list.Key = func(i int) any { return shown[i].ID }
		ui.List(c, &x.list, len(shown), func(i int) {
			a.transferRow(c, t, p, shown[i])
		}).Grow(1).Padding(0, 12, 24).Gap(2)
	})
}

func (a *App) transferRow(c *ui.Context, t *ui.Theme, p *palette, task transfer.Task) {
	row := ui.Row(c.Key(task.ID)).Height(66).PaddingX(12).Gap(14).AlignItems(ui.Center).Radius(12)
	if row.Hovered() {
		row.Background(p.Hover)
	}
	f := baidu.File{Name: task.Name, Path: task.Remote}
	row.Children(func() {
		ui.Box(c).Children(func() {
			a.fileIcon(c, t, p, &f, 38)
			ic, color := "download", t.Accent
			if task.Kind == transfer.Upload {
				ic, color = "upload", t.Success
			}
			ui.Box(c).Absolute().Right(-5).Bottom(-5).Size(18, 18).Radius(9).Center().Background(color).
				Border(2, t.Background).TextColor(ui.Hex("#ffffff")).Children(func() {
				ui.Icon(c, icon(ic)).FontSize(9)
			})
		})
		ui.Column(c).Grow(1).Shrink(1).Gap(6).Children(func() {
			ui.Text(c, task.Name).FontWeight(500).SingleLine()
			if task.State != transfer.Done {
				frac := float32(0)
				if task.Size > 0 {
					frac = float32(float64(task.Done) / float64(task.Size))
				}
				track := ui.Box(c).Height(4).Radius(2).Background(p.Hairline).Clip()
				track.Children(func() {
					color := t.Accent
					switch task.State {
					case transfer.Paused, transfer.Queued:
						color = t.TextMuted
					case transfer.Failed:
						color = t.Danger
					}
					bar := ui.Box(c).Height(4).Radius(2).Background(color)
					bar.WidthPercent(bar.AnimateTo("p", frac*100, springSoft))
				})
			}
			ui.Text(c, transferDetail(task)).FontSize(t.Rem(0.84)).TextColor(t.TextMuted).FontFeatures("tnum").SingleLine()
		})
		a.transferActions(c, t, p, task)
	})
	if task.State == transfer.Done && row.DoubleClicked() && task.Kind == transfer.Download {
		mygo.Shell.OpenPath(task.Local)
	}
}

func transferDetail(task transfer.Task) string {
	sizes := humanSize(task.Done) + " / " + humanSize(task.Size)
	switch task.State {
	case transfer.Queued:
		return "等待中 · " + sizes
	case transfer.Paused:
		return "已暂停 · " + sizes
	case transfer.Failed:
		return "失败：" + task.Err
	case transfer.Done:
		where := task.Local
		if task.Kind == transfer.Upload {
			where = task.Remote
		}
		return humanSize(task.Size) + " · " + where
	}
	s := sizes + " · " + humanSpeed(task.Speed)
	if task.Speed > 0 && task.Size > task.Done {
		s += " · 剩余 " + humanDuration(time.Duration(float64(task.Size-task.Done)/task.Speed)*time.Second)
	}
	return s
}

func humanDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return itoa(int(d.Seconds())) + " 秒"
	case d < time.Hour:
		return itoa(int(d.Minutes())) + " 分钟"
	}
	return itoa(int(d.Hours())) + " 小时 " + itoa(int(d.Minutes())%60) + " 分"
}

func (a *App) transferActions(c *ui.Context, t *ui.Theme, p *palette, task transfer.Task) {
	m := a.xfer
	if m == nil {
		return
	}
	switch task.State {
	case transfer.Running, transfer.Queued:
		if iconButton(c, t, p, "pause", "暂停").Clicked() {
			m.Pause(task.ID)
		}
	case transfer.Paused, transfer.Failed:
		if iconButton(c, t, p, "play", "继续").Clicked() {
			m.Resume(task.ID)
		}
	case transfer.Done:
		if task.Kind == transfer.Download {
			if iconButton(c, t, p, "external-link", "打开").Clicked() {
				mygo.Shell.OpenPath(task.Local)
			}
			if iconButton(c, t, p, "folder-open", "在文件夹中显示").Clicked() {
				mygo.Shell.ShowItemInFolder(task.Local)
			}
		} else if iconButton(c, t, p, "folder-open", "在网盘中显示").Clicked() {
			a.router.Push(filesRoute(path.Dir(task.Remote)))
		}
	}
	label := "取消"
	if task.State == transfer.Done {
		label = "移除记录"
	}
	if iconButton(c, t, p, "x", label).Clicked() {
		m.Remove(task.ID)
	}
}

// transfersToolbar is the title bar's controls on the transfers page.
func (a *App) transfersToolbar(c *ui.Context, t *ui.Theme, p *palette) {
	m := a.xfer
	if m == nil {
		return
	}
	var down, up float64
	for _, task := range a.transfers.tasks {
		if task.State == transfer.Running {
			if task.Kind == transfer.Download {
				down += task.Speed
			} else {
				up += task.Speed
			}
		}
	}
	if down > 0 || up > 0 {
		ui.Row(c).Gap(12).Margin(0, 8, 0, 0).FontFeatures("tnum").TextColor(t.TextMuted).Children(func() {
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icon("download")).FontSize(14).TextColor(t.Accent)
				ui.Text(c, humanSpeed(down)).ContentTransition(ui.ContentTransition{Numeric: true})
			})
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icon("upload")).FontSize(14).TextColor(t.Success)
				ui.Text(c, humanSpeed(up)).ContentTransition(ui.ContentTransition{Numeric: true})
			})
		})
	}
	stopped := 0
	for _, task := range a.transfers.tasks {
		if task.State == transfer.Paused || task.State == transfer.Failed {
			stopped++
		}
	}
	if a.activeTransfers() > 0 {
		if pill(c, t, p, "pause", "全部暂停", false).Clicked() {
			m.PauseAll()
		}
	} else if stopped > 0 && pill(c, t, p, "play", "全部开始", false).Clicked() {
		m.ResumeAll()
	}
	if a.transfers.tab == 1 && iconButton(c, t, p, "trash-2", "清除已完成").Clicked() {
		m.ClearFinished()
	}
	if iconButton(c, t, p, "folder-open", "打开下载文件夹").Clicked() {
		dir := a.settings.downloadDir()
		os.MkdirAll(dir, 0o755)
		mygo.Shell.OpenPath(dir)
	}
}

// tabs is a segmented control of labels whose thumb glides to the one
// chosen.
func tabs(c *ui.Context, t *ui.Theme, p *palette, chosen *int, labels ...string) ui.Element {
	seg := ui.SegmentedBase(c, chosen, len(labels))
	seg.Track.Height(34).Padding(3).Radius(10).Background(t.Surface)
	x := seg.Track.AnimateTo("thumb", float32(*chosen), springSoft)
	thumb, shadow := p.Card, p.Shadow
	if t.Dark {
		thumb = t.SurfacePressed
	}
	seg.Track.Draw(func(pt *ui.Painter, r ui.Rect) {
		w := (r.W - 6) / float32(len(labels))
		th := ui.Rect{X: r.X + 3 + x*w, Y: r.Y + 3, W: w, H: r.H - 6}
		pt.Shadow(th, 7, 0, 1, 3, 0, shadow)
		pt.Fill(th, thumb, 7)
	})
	seg.Track.Children(func() {
		for i, label := range labels {
			s := seg.Segment(i).Width(116).Center().Label(label)
			color := t.TextMuted
			if i == *chosen {
				color = t.Text
			}
			s.TextColor(s.AnimateColor("fg", color, fadeQuick))
			s.Children(func() {
				ui.Text(c, label).FontWeight(560).SingleLine().FontFeatures("tnum").ContentTransition(ui.ContentTransition{Numeric: true})
			})
		}
	})
	return seg.Track
}

// share asks how to share files.
func (a *App) share(files []baidu.File) { a.toast("分享功能即将到来", "") }
