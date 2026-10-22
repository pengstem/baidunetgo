package app

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/transfer"
)

// transfersState is the transfers page.
type transfersState struct {
	tab     int // 0 going, 1 done
	tasks   []transfer.Task
	gen     int // counts changes of tasks
	list    ui.ListState
	speeds  []float64 // the total speed each second, the latest last
	sampled time.Time // when speeds last took a sample
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
				a.transfers.gen++
				a.showTransfers(tasks)
			}
		})
	}
	m.OnFinish = func(t transfer.Task) {
		a.update(func() { a.finished(t) })
	}
	m.OnSkip = func(dir string, err error) {
		a.update(func() {
			a.toast("无法读取「"+path.Base(dir)+"」，其中的文件没有加入下载："+err.Error(), "error")
		})
	}
	a.xfer = m
	a.transfers.tasks = m.Tasks()
	a.holdTransfers()
}

func (a *App) stopTransfers() {
	if a.xfer != nil {
		a.xfer.Close()
		a.xfer = nil
	}
	a.transfers = transfersState{}
	a.gathers = nil
	a.showTransfers(nil)
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
	// A click shows the file (Notification), so each has its own ID.
	verb, id := "下载", "file:"+t.Local
	if t.Kind == transfer.Upload {
		verb, id = "上传", "dir:"+path.Dir(t.Remote)
	}
	if a.win != nil && !a.win.IsFocused() {
		mygo.NewNotification(mygo.NotificationOptions{ID: id, Title: verb + "完成", Body: t.Name, Group: "transfers"}).Show()
	}
}

// download queues files to download.
func (a *App) download(files []baidu.File) {
	if a.xfer == nil || len(files) == 0 {
		return
	}
	a.xfer.AddDownloads(files, a.settings.downloadDir())
	a.pendingToasts = append(a.pendingToasts, ui.Toast{
		Title: "已加入下载：" + countLabel(files), Type: "success", Action: "查看", Timeout: briefToast,
		OnAction: func() { a.router.Push("/transfers") },
	})
}

// upload queues local files and folders to go into dir, asking first what
// to do with names it has already.
func (a *App) upload(locals []string, dir string) {
	if a.xfer == nil || len(locals) == 0 {
		return
	}
	names := make([]string, len(locals))
	for i, l := range locals {
		names[i] = filepath.Base(l)
	}
	a.place("上传", dir, names, nil, func(pl placement) {
		renamed := map[string]string{}
		var going []string
		for i, l := range locals {
			if pl.names[i] != "" {
				going = append(going, l)
				renamed[l] = pl.names[i]
			}
		}
		if len(pl.replaced) == 0 {
			if a.dlg.kind == dlgConflict {
				a.dlg.open = false
			}
			if len(going) > 0 {
				a.queueUploads(going, dir, renamed)
			}
			return
		}
		// What they replace goes to the recycle bin first, dissolving.
		a.run(func(ctx context.Context, c *baidu.Client) error { return c.Delete(ctx, paths(pl.replaced)) }, []string{dir}, "", func() {
			a.recycleDirty = true
			a.queueUploads(going, dir, renamed)
		})
		if l := a.listings[dir]; l != nil {
			ids := make([]int64, len(pl.replaced))
			for i, f := range pl.replaced {
				ids[i] = f.FsID
			}
			a.dissolve(l, ids)
		}
	})
}

func (a *App) queueUploads(locals []string, dir string, names map[string]string) {
	if a.xfer == nil {
		return
	}
	a.xfer.AddUploads(locals, dir, names)
	n := itoa(len(locals)) + " 项"
	if len(locals) == 1 {
		n = "「" + names[locals[0]] + "」"
	}
	a.pendingToasts = append(a.pendingToasts, ui.Toast{
		Title: "正在上传" + n + "到「" + dirLabel(dir) + "」", Type: "success", Action: "查看", Timeout: briefToast,
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

// transferCounts counts the tasks running down and up, those queued, and
// adds up their speed.
func (a *App) transferCounts() (down, up, queued int, speed float64) {
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
	return
}

// speedHistory is how many seconds of speed the transfers page draws.
const speedHistory = 120

// sampleSpeed notes the transfers' speed once a second while any runs,
// for the chart of the transfers page.
func (a *App) sampleSpeed(c *ui.Context) {
	n, speed := a.transferPulse()
	x := &a.transfers
	if n == 0 && len(x.speeds) == 0 {
		return
	}
	now := c.Now()
	if now.Sub(x.sampled) >= time.Second {
		x.sampled = now
		x.speeds = append(x.speeds, speed)
		if len(x.speeds) > speedHistory {
			x.speeds = x.speeds[len(x.speeds)-speedHistory:]
		}
		if n == 0 && slices.Max(x.speeds) == 0 {
			x.speeds = nil // idle for the whole chart: start afresh next time
		}
	}
	c.After(time.Second)
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
	reading := a.xfer != nil && a.xfer.Reading() > 0 && x.tab == 0
	if len(shown) == 0 && reading {
		ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c).Label("正在读取文件夹") })
		return
	}
	if len(shown) == 0 {
		if x.tab == 0 {
			emptyState(c, t, p, "arrow-up-down", "没有正在进行的传输", "下载或上传的文件会出现在这里", nil)
		} else {
			emptyState(c, t, p, "circle-check", "还没有完成的传输", "", nil)
		}
		return
	}
	x.list.Key = func(i int) any { return shown[i].ID }
	ui.Column(c).Grow(1).Children(func() {
		if x.tab == 0 {
			a.speedCard(c, t, p)
		}
		if reading {
			ui.Row(c).Gap(8).PaddingX(20).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "正在读取文件夹，其中的文件会陆续加入").FontSize(12).TextColor(t.TextMuted)
			})
		}
		ui.List(c, &x.list, len(shown), func(i int) {
			a.transferRow(c, t, p, shown[i], i)
		}).Grow(1).Padding(4, 10, 20)
	})
}

// speedCard is what the transfers do together, as Pulse's power card: the
// speed in large figures, the tasks and connections beside it, and the
// last two minutes of speed in dots.
func (a *App) speedCard(c *ui.Context, t *ui.Theme, p *palette) {
	down, up, queued, speed := a.transferCounts()
	cfg := a.transferConfig()
	green := navItems[1].tile
	group(c, p).Margin(4, 20, 8).Padding(16, 18).Gap(12).Transition(popIn).Children(func() {
		ui.Row(c).Gap(36).AlignItems(ui.End).Children(func() {
			ui.Column(c).Gap(2).Children(func() {
				ui.Text(c, "当前速度").FontSize(12).TextColor(t.TextMuted)
				ui.Row(c).Gap(5).AlignItems(ui.Baseline).Children(func() {
					n, unit := "0", "B/s"
					if speed > 0 {
						v, u, _ := strings.Cut(humanSpeed(speed), " ")
						n, unit = v, u
					}
					figure(c, n).FontSize(36).FontWeight(700).ContentTransition(ui.ContentTransition{Numeric: true})
					ui.Text(c, unit).FontSize(16).FontWeight(600).TextColor(t.TextMuted)
				})
			})
			stat := func(label, value string) {
				ui.Column(c).Gap(2).Padding(0, 0, 6).Children(func() {
					ui.Text(c, label).FontSize(12).TextColor(t.TextMuted)
					figure(c, value).FontWeight(600).ContentTransition(ui.ContentTransition{Numeric: true})
				})
			}
			stat("下载", itoa(down))
			stat("上传", itoa(up))
			stat("排队", itoa(queued))
			stat("连接", itoa(cfg.Threads)+" × "+itoa(cfg.Parallel))
		})
		speeds := a.transfers.speeds
		ui.Box(c).Height(64).FillWidth().Radius(8).Background(green.Alpha(0.07)).Draw(func(pt *ui.Painter, r ui.Rect) {
			dotChart(pt, ui.Rect{X: r.X + 10, Y: r.Y + 8, W: r.W - 20, H: r.H - 16}, speeds, green)
		})
		ui.Row(c).Margin(-6, 10, 0).FontSize(10).TextColor(p.Faint).Children(func() {
			ui.Text(c, "2 分钟前")
			ui.Spacer(c)
			ui.Text(c, "1 分钟前")
			ui.Spacer(c)
			ui.Text(c, "现在")
		})
	})
}

// dotChart draws values as columns of dots, the latest at the right, each
// as tall as its share of the largest, on a grid of faint dots.
func dotChart(pt *ui.Painter, r ui.Rect, values []float64, color ui.Color) {
	const d, step = 3.5, 6
	cols, rows := int((r.W+step-d)/step), int((r.H+step-d)/step)
	if cols <= 0 || rows <= 0 {
		return
	}
	x0 := r.X + r.W - float32(cols-1)*step - d
	top := 0.0
	for _, v := range values {
		top = max(top, v)
	}
	faint := color.Alpha(0.18)
	for col := range cols {
		i := len(values) - cols + col // the sample this column shows
		lit := 0
		if i >= 0 && top > 0 && values[i] > 0 {
			lit = max(1, int(values[i]/top*float64(rows)+0.5))
		}
		for row := range rows {
			c := faint
			if rows-row <= lit {
				c = color
			}
			pt.Fill(ui.Rect{X: x0 + float32(col)*step, Y: r.Y + float32(row)*step, W: d, H: d}, c, d/2)
		}
	}
}

// transferRow is one task, as the rows of Pulse's lists of apps: its name
// and how it goes on a line, a slim bar of its progress under them.
func (a *App) transferRow(c *ui.Context, t *ui.Theme, p *palette, task transfer.Task, i int) {
	col := ui.Column(c.Key(task.ID)).Padding(8, 10).Gap(6).Radius(8)
	if col.Hovered() {
		col.Background(p.Hover)
	}
	f := baidu.File{Name: task.Name, Path: task.Remote}
	col.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			a.fileIcon(c, t, p, &f, 18)
			ui.Text(c, task.Name).SingleLine().Shrink(1)
			arrow, color := "↓", t.Accent
			if task.Kind == transfer.Upload {
				arrow, color = "↑", t.Success
			}
			if task.State != transfer.Running {
				color = p.Faint
			}
			ui.Text(c, arrow).FontSize(12).FontWeight(700).TextColor(color)
			ui.Spacer(c)
			detail := figure(c, transferDetail(task)).FontSize(12).TextColor(t.TextMuted).Shrink(1)
			if task.State == transfer.Failed {
				detail.TextColor(t.Danger)
			} else if task.State == transfer.Queued && task.Retries > 0 {
				detail.TextColor(t.Warning)
			}
			ui.Row(c).Gap(0).Children(func() { a.transferActions(c, t, p, task) })
		})
		if task.State != transfer.Done {
			frac := float32(0)
			if task.Size > 0 {
				frac = float32(float64(task.Done) / float64(task.Size))
			}
			fill := t.Accent
			switch task.State {
			case transfer.Paused, transfer.Queued:
				fill = p.Faint
			case transfer.Failed:
				fill = t.Danger
			}
			if task.Kind == transfer.Upload && task.State == transfer.Running {
				fill = t.Success
			}
			ui.Row(c).Gap(10).AlignItems(ui.Center).PaddingStart(26).Children(func() {
				bar := ui.Box(c).Grow(1).Height(5).Radius(3).Background(p.Control)
				shown := bar.AnimateTo("p", frac, motionPanel)
				bar.Draw(func(pt *ui.Painter, r ui.Rect) {
					if w := r.W * shown; w > 0 {
						pt.Fill(ui.Rect{X: r.X, Y: r.Y, W: max(w, r.H), H: r.H}, fill, r.H/2)
					}
				})
				figure(c, itoa(int(frac*100))+"%").Width(34).FontSize(11).TextAlign(ui.End).TextColor(t.TextMuted)
			})
		}
	})
	if task.State == transfer.Done && col.DoubleClicked() && task.Kind == transfer.Download {
		mygo.Shell.OpenPath(task.Local)
	}
}

func transferDetail(task transfer.Task) string {
	sizes := humanSize(task.Done) + " / " + humanSize(task.Size)
	switch task.State {
	case transfer.Queued:
		if task.Retries > 0 {
			return "稍后重试（" + itoa(task.Retries) + "/5）：" + task.Err
		}
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
	stopped, going, done := 0, 0, 0
	for _, task := range a.transfers.tasks {
		if task.State == transfer.Paused || task.State == transfer.Failed {
			stopped++
		}
		if task.State == transfer.Done {
			done++
		} else {
			going++
		}
	}
	tabs := choice(c, t, p, &a.transfers.tab, a.compact, "进行中 "+itoa(going), "已完成 "+itoa(done)).Label("传输")
	if !a.compact {
		tabs.Width(190)
	}
	if a.activeTransfers() > 0 {
		if pill(c, t, p, "", "全部暂停", false).Clicked() {
			m.PauseAll()
		}
	} else if stopped > 0 && pill(c, t, p, "", "全部开始", false).Clicked() {
		m.ResumeAll()
	}
	if a.transfers.tab == 1 && done > 0 && toolButton(c, t, p, "trash-2", "清除已完成").Clicked() {
		m.ClearFinished()
	}
	if toolButton(c, t, p, "folder-open", "打开下载文件夹").Clicked() {
		dir := a.settings.downloadDir()
		os.MkdirAll(dir, 0o755)
		mygo.Shell.OpenPath(dir)
	}
}
