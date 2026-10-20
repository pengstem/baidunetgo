package app

import (
	"os"
	"path"
	"path/filepath"
	"strings"
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
	cfg := a.transferConfig()
	m.Threads, m.Parallel, m.Upload, m.Block = cfg.Threads, cfg.Parallel, cfg.Upload, cfg.Block
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

// transferPulse returns how many tasks run and their speed together.
func (a *App) transferPulse() (int, float64) {
	n, speed := 0, 0.0
	for _, t := range a.transfers.tasks {
		if t.State == transfer.Running {
			n++
			speed += t.Speed
		}
	}
	return n, speed
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
		ui.Row(c).Padding(4, 24, 12).Gap(24).AlignItems(ui.Center).Children(func() {
			tabs(c, t, p, &x.tab, "进行中 "+itoa(len(going)), "已完成 "+itoa(len(done)))
			a.transferSummary(c, t, p)
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
		}).Grow(1).Padding(0, 24, 24).Dividers(1, p.Hairline)
	})
}

// transferSummary is one line of what the transfers do together: the
// speed, then the tasks by direction and the connections.
func (a *App) transferSummary(c *ui.Context, t *ui.Theme, p *palette) {
	var down, up, queued int
	var speed float64
	for _, task := range a.transfers.tasks {
		switch {
		case task.State == transfer.Queued:
			queued++
		case task.State != transfer.Running:
		case task.Kind == transfer.Download:
			down++
			speed += task.Speed
		default:
			up++
			speed += task.Speed
		}
	}
	if down+up == 0 {
		return
	}
	cfg := a.transferConfig()
	ui.Row(c).Gap(20).AlignItems(ui.End).Font(fontMono).FontFeatures("tnum").TextColor(t.TextMuted).Children(func() {
		ui.Row(c).Gap(4).AlignItems(ui.End).Children(func() {
			n, unit, _ := strings.Cut(humanSpeed(speed), " ")
			ui.Text(c, n).FontSize(22).FontWeight(500).TextColor(t.Text).ContentTransition(ui.ContentTransition{Numeric: true})
			ui.Text(c, unit)
		})
		line := "↓ " + itoa(down) + "  ↑ " + itoa(up)
		if queued > 0 {
			line += "  排队 " + itoa(queued)
		}
		ui.Text(c, line).SingleLine()
		ui.Text(c, itoa(cfg.Threads)+" 连接 × "+itoa(cfg.Parallel)+" 文件").TextColor(p.Faint).SingleLine()
	})
}

func (a *App) transferRow(c *ui.Context, t *ui.Theme, p *palette, task transfer.Task) {
	col := ui.Column(c.Key(task.ID)).Padding(10, 0).Gap(8)
	f := baidu.File{Name: task.Name, Path: task.Remote}
	col.Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			dir, color := "↓", t.Accent
			if task.Kind == transfer.Upload {
				dir, color = "↑", t.Success
			}
			if task.State != transfer.Running {
				color = p.Faint
			}
			figure(c, dir).Width(12).TextColor(color)
			a.fileIcon(c, t, p, &f, 18)
			ui.Text(c, task.Name).FontWeight(500).SingleLine().Shrink(1)
			ui.Spacer(c)
			detail := figure(c, transferDetail(task)).FontSize(12).TextColor(t.TextMuted).Shrink(1)
			if task.State == transfer.Failed {
				detail.TextColor(t.Danger)
			}
			ui.Row(c).Gap(2).Children(func() { a.transferActions(c, t, p, task) })
		})
		if task.State != transfer.Done {
			frac := float32(0)
			if task.Size > 0 {
				frac = float32(float64(task.Done) / float64(task.Size))
			}
			ui.Row(c).Gap(12).AlignItems(ui.Center).PaddingStart(50).Children(func() {
				blockBar(c, t, p, task, frac).Grow(1)
				figure(c, itoa(int(frac*100))+"%").Width(36).FontSize(12).TextAlign(ui.End)
			})
		}
	})
	if task.State == transfer.Done && col.DoubleClicked() && task.Kind == transfer.Download {
		mygo.Shell.OpenPath(task.Local)
	}
}

// blockBar shows a task's progress as a row of cells, each a slice of
// the file: filled when fetched, outlined while the connections work on
// it, dark while it waits.
func blockBar(c *ui.Context, t *ui.Theme, p *palette, task transfer.Task, frac float32) ui.Element {
	color := t.Accent
	switch task.State {
	case transfer.Paused, transfer.Queued:
		color = t.TextMuted
	case transfer.Failed:
		color = t.Danger
	}
	running := task.State == transfer.Running
	waiting := p.Hairline
	bar := ui.Box(c).Height(12)
	shown := bar.AnimateTo("p", frac, motionPanel)
	return bar.Draw(func(pt *ui.Painter, r ui.Rect) {
		const w, gap = 5, 2
		n := int((r.W + gap) / (w + gap))
		if n <= 0 {
			return
		}
		filled := int(shown * float32(n))
		live := 0
		if running && filled < n {
			live = min(3, n-filled)
		}
		for i := range n {
			cell := ui.Rect{X: r.X + float32(i)*(w+gap), Y: r.Y, W: w, H: r.H}
			switch {
			case i < filled:
				pt.Fill(cell, color, 1)
			case i < filled+live:
				pt.Fill(cell, color, 1)
				pt.Fill(ui.Rect{X: cell.X + 1, Y: cell.Y + 1, W: cell.W - 2, H: cell.H - 2}, t.Background, 0)
			default:
				pt.Fill(cell, waiting, 1)
			}
		}
	})
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
	stopped := 0
	for _, task := range a.transfers.tasks {
		if task.State == transfer.Paused || task.State == transfer.Failed {
			stopped++
		}
	}
	if a.activeTransfers() > 0 {
		if pill(c, t, p, "", "全部暂停", false).Clicked() {
			m.PauseAll()
		}
	} else if stopped > 0 && pill(c, t, p, "", "全部开始", false).Clicked() {
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

// tabs is an outlined segmented control of labels, the chosen one filled
// with the line color.
func tabs(c *ui.Context, t *ui.Theme, p *palette, chosen *int, labels ...string) ui.Element {
	seg := ui.SegmentedBase(c, chosen, len(labels))
	seg.Track.Height(28).Radius(5).Border(1, p.Hairline).Clip()
	x := seg.Track.AnimateTo("thumb", float32(*chosen), motionPanel)
	thumb := p.Hairline
	seg.Track.Draw(func(pt *ui.Painter, r ui.Rect) {
		w := (r.W - 2) / float32(len(labels))
		pt.Fill(ui.Rect{X: r.X + 1 + x*w, Y: r.Y + 1, W: w, H: r.H - 2}, thumb, 0)
	})
	seg.Track.Children(func() {
		for i, label := range labels {
			s := seg.Segment(i).MinWidth(84).Grow(1).Basis(0).PaddingX(12).Center().Label(label)
			color := t.TextMuted
			if i == *chosen {
				color = t.Text
			}
			s.TextColor(s.AnimateColor("fg", color, motionHover))
			s.Children(func() {
				ui.Text(c, label).FontWeight(500).SingleLine().FontFeatures("tnum").ContentTransition(ui.ContentTransition{Numeric: true})
			})
		}
	})
	return seg.Track
}
