package app

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// offlineState is the page of offline downloads, which the netdisk does
// itself.
type offlineState struct {
	items   []baidu.Offline
	loaded  bool
	loading bool
	err     string
	polled  time.Time
	list    ui.ListState
	url     string
	picker  picker
}

func (a *App) loadOffline() {
	o := &a.offline
	if a.sess == nil || o.loading {
		return
	}
	o.loading = true
	c := a.sess.client
	go func() {
		items, err := c.Offlines(context.Background())
		a.update(func() {
			o.loading, o.polled = false, time.Now()
			if err != nil {
				o.err = err.Error()
				return
			}
			slices.SortFunc(items, func(x, y baidu.Offline) int { return int(y.Created - x.Created) })
			for _, it := range items {
				if it.Status == 0 && !slices.ContainsFunc(o.items, func(p baidu.Offline) bool { return p.ID == it.ID && p.Status == 0 }) && o.loaded {
					a.reload(it.SavePath)
				}
			}
			o.items, o.loaded, o.err = items, true, ""
		})
	}()
}

func (a *App) offlinePage(c *ui.Context, t *ui.Theme, p *palette) {
	o := &a.offline
	running := false
	for _, it := range o.items {
		running = running || it.Status == 1
	}
	// Follow running tasks every few seconds while the page shows.
	if !o.loading && (!o.loaded || (running && time.Since(o.polled) > 3*time.Second)) {
		a.loadOffline()
	}
	if running {
		c.After(time.Second)
	}
	ui.Column(c).Grow(1).Children(func() {
		switch {
		case !o.loaded && o.err != "":
			emptyState(c, t, p, "triangle-alert", "无法读取离线任务", o.err, a.loadOffline)
		case !o.loaded:
			skeletonRows(c, t, p)
		case len(o.items) == 0:
			emptyState(c, t, p, "cloud-download", "没有离线下载", "把 HTTP 或磁力链接交给网盘，它会替你下载", nil)
		default:
			ui.List(c, &o.list, len(o.items), func(i int) {
				it := o.items[i]
				row := ui.Row(c.Key(it.ID)).Height(66).PaddingX(12).Gap(14).AlignItems(ui.Center).Radius(12)
				if row.Hovered() {
					row.Background(p.Hover)
				}
				f := baidu.File{Name: it.Name, Path: it.SavePath + "/" + it.Name}
				row.Children(func() {
					a.fileIcon(c, t, p, &f, 38)
					ui.Column(c).Grow(1).Shrink(1).Gap(6).Children(func() {
						ui.Text(c, it.Name).FontWeight(500).SingleLine()
						if it.Status == 1 {
							frac := float32(0)
							if it.Size > 0 {
								frac = float32(float64(it.Done) / float64(it.Size))
							}
							ui.Box(c).Height(4).Radius(2).Background(p.Hairline).Clip().Children(func() {
								bar := ui.Box(c).Height(4).Radius(2).Background(t.Accent)
								bar.WidthPercent(bar.AnimateTo("p", frac*100, springSoft))
							})
						}
						detail := it.StatusText() + " · " + humanSize(it.Size) + " · 保存到 " + dirLabel(it.SavePath)
						ui.Text(c, detail).FontSize(t.Rem(0.84)).TextColor(t.TextMuted).SingleLine()
					})
					switch it.Status {
					case 0:
						if iconButton(c, t, p, "folder-open", "在网盘中显示").Clicked() {
							a.router.Push(filesRoute(it.SavePath))
						}
					case 1:
						if iconButton(c, t, p, "circle-x", "取消").Clicked() {
							a.offlineAct(it, false)
						}
					}
					if iconButton(c, t, p, "x", "删除记录").Clicked() {
						a.offlineAct(it, true)
					}
				})
			}).Grow(1).Padding(4, 12, 24)
		}
	})
}

func (a *App) offlineAct(it baidu.Offline, del bool) {
	c := a.sess.client
	go func() {
		var err error
		if del {
			err = c.DeleteOffline(context.Background(), it.ID)
		} else {
			err = c.CancelOffline(context.Background(), it.ID)
		}
		a.update(func() {
			if err != nil {
				a.toast(err.Error(), "error")
				return
			}
			a.loadOffline()
		})
	}()
}

// offlineDialog adds an offline download.
func (a *App) offlineDialog(c *ui.Context, t *ui.Theme, p *palette) {
	o := &a.offline
	dialogTitle(c, t, "新建离线下载", "网盘会在云端下载，完成后出现在目标文件夹")
	ui.TextArea(c, &o.url).Lines(2, 4).AutoFocus().Placeholder("https://… 或 magnet:?xt=…").Font("monospace").FontSize(12).Label("链接")
	ui.Text(c, "保存到").TextColor(t.TextMuted).FontSize(t.Rem(0.9))
	a.folderChooser(c, t, p, &o.picker, 170, nil)
	src := strings.TrimSpace(o.url)
	valid := strings.HasPrefix(src, "magnet:") || strings.Contains(src, "://")
	if a.dialogButtons(c, t, p, "开始下载", false, !valid) {
		dir := o.picker.dir
		a.run(func(ctx context.Context, c *baidu.Client) error {
			return c.AddOffline(ctx, src, dir)
		}, nil, "已添加离线任务", func() {
			o.url = ""
			a.loadOffline()
		})
	}
}

// offlineToolbar is the title bar's controls on the offline page.
func (a *App) offlineToolbar(c *ui.Context, t *ui.Theme, p *palette) {
	if iconButton(c, t, p, "refresh-cw", "刷新").Clicked() {
		a.loadOffline()
	}
	if pill(c, t, p, "plus", "新建任务", true).Clicked() {
		a.ask(dlgOffline, "/", nil)
		a.offline.picker = picker{dir: "/"}
	}
}
