package app

import (
	"context"
	"path"
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// recycleState is the recycle bin page.
type recycleState struct {
	items   []baidu.Recycled
	loaded  bool
	loading bool
	err     string
	list    ui.ListState
	confirm bool // asking to empty it
	busy    map[int64]bool
}

func (a *App) loadRecycle() {
	r := &a.recycle
	if a.sess == nil || r.loading {
		return
	}
	r.loading, r.err = true, ""
	c := a.sess.client
	go func() {
		items, err := c.RecycleList(context.Background())
		a.update(func() {
			r.loading = false
			if err != nil {
				r.err = err.Error()
				return
			}
			r.items, r.loaded = items, true
		})
	}()
}

func (a *App) recyclePage(c *ui.Context, t *ui.Theme, p *palette) {
	r := &a.recycle
	if (!r.loaded || a.recycleDirty) && !r.loading {
		a.recycleDirty = false
		a.loadRecycle()
	}
	if r.busy == nil {
		r.busy = map[int64]bool{}
	}
	ui.Column(c).Grow(1).Children(func() {
		switch {
		case !r.loaded && r.err != "":
			emptyState(c, t, p, "triangle-alert", "无法读取回收站", r.err, a.loadRecycle)
		case !r.loaded:
			skeletonRows(c, t, p)
		case len(r.items) == 0:
			emptyState(c, t, p, "trash-2", "回收站是空的", "删除的文件会在这里保留一段时间", nil)
		default:
			now := c.Now()
			ui.List(c, &r.list, len(r.items), func(i int) {
				it := r.items[i]
				row := stripe(ui.Row(c.Key(it.FsID)).Height(44).PaddingX(10).Gap(10).AlignItems(ui.Center), p, i)
				row.Children(func() {
					a.fileIcon(c, t, p, &it.File, 28)
					ui.Column(c).Grow(1).Shrink(1).Gap(1).Children(func() {
						ui.Text(c, it.Name).SingleLine()
						ui.Text(c, "原位置："+dirLabel(path.Dir(it.Path))+" · "+humanTime(it.Mtime, now)).
							FontSize(11).TextColor(t.TextMuted).SingleLine()
					})
					days := itoa(it.LeftDays) + " 天后清除"
					if it.LeftDays <= 3 {
						chip(c, days, t.Warning)
					} else {
						figure(c, days).FontSize(12).TextColor(t.TextMuted)
					}
					if r.busy[it.FsID] {
						ui.Spinner(c).Label("处理中")
						return
					}
					if iconButton(c, t, p, "rotate-ccw", "还原").Clicked() {
						a.recycleAct(it, false)
					}
					del := iconButton(c, t, p, "x", "彻底删除")
					if del.Hovered() {
						del.TextColor(t.Danger)
					}
					if del.Clicked() {
						a.recycleAct(it, true)
					}
				})
			}).Grow(1).Padding(4, 10, 20)
		}
	})
	if r.confirm {
		ui.Modal(c, &r.confirm, func() {
			dialogTitle(c, t, "清空回收站？", "")
			ui.Text(c, "其中的 "+itoa(len(r.items))+" 项会被彻底删除，无法恢复。").TextColor(t.TextMuted)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if pill(c, t, p, "", "取消", false).Clicked() {
					r.confirm = false
				}
				b := pill(c, t, p, "", "清空", true).Background(t.Danger)
				if b.Clicked() {
					r.confirm = false
					a.emptyRecycle()
				}
			})
		})
	}
}

// recycleAct restores an item, or deletes it for good.
func (a *App) recycleAct(it baidu.Recycled, purge bool) {
	r := &a.recycle
	r.busy[it.FsID] = true
	c := a.sess.client
	go func() {
		var err error
		if purge {
			err = c.Purge(context.Background(), []int64{it.FsID})
		} else {
			err = c.Restore(context.Background(), []int64{it.FsID})
		}
		a.update(func() {
			delete(r.busy, it.FsID)
			if err != nil {
				a.toast(err.Error(), "error")
				return
			}
			r.items = slices.DeleteFunc(r.items, func(x baidu.Recycled) bool { return x.FsID == it.FsID })
			if purge {
				a.toast("已彻底删除「"+it.Name+"」", "success")
			} else {
				a.toast("已还原「"+it.Name+"」", "success")
				a.reload(path.Dir(it.Path))
			}
		})
	}()
}

func (a *App) emptyRecycle() {
	r := &a.recycle
	c := a.sess.client
	go func() {
		err := c.EmptyRecycle(context.Background())
		a.update(func() {
			if err != nil {
				a.toast(err.Error(), "error")
				return
			}
			r.items = nil
			a.toast("回收站已清空", "success")
		})
	}()
}

// recycleToolbar is the title bar's controls on the recycle bin.
func (a *App) recycleToolbar(c *ui.Context, t *ui.Theme, p *palette) {
	if toolButton(c, t, p, "refresh-cw", "刷新").Clicked() {
		a.loadRecycle()
	}
	if len(a.recycle.items) > 0 && pill(c, t, p, "trash-2", "清空回收站", false).Clicked() {
		a.recycle.confirm = true
	}
}
