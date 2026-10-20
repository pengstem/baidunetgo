package app

import (
	"context"
	"crypto/rand"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// shareForm is the dialog sharing files, and saving a share's link.
type shareForm struct {
	period  int // 0 one day, 1 seven, 2 thirty, 3 for ever
	pwd     string
	link    string // made
	text    string // pasted link of a share to save
	picker  picker
	created bool
}

var sharePeriods = []int{1, 7, 30, 0}

// newPassword makes a share's 4-character password.
func newPassword() string {
	const chars = "abcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, 4)
	rand.Read(b)
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b)
}

// share asks how to share files.
func (a *App) share(files []baidu.File) {
	if len(files) == 0 {
		return
	}
	a.ask(dlgShare, path.Dir(files[0].Path), files)
	a.shareForm = shareForm{period: 1, pwd: newPassword()}
}

// shareText is what the clipboard holds for a link and its password.
func shareText(link, pwd string) string {
	if pwd == "" {
		return link
	}
	return "链接: " + link + "?pwd=" + pwd + " 提取码: " + pwd
}

func (a *App) shareDialog(c *ui.Context, t *ui.Theme, p *palette) {
	d, f := &a.dlg, &a.shareForm
	if f.created {
		ui.Column(c).Key("made").Gap(16).Transition(popIn).Children(func() {
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icon("circle-check")).FontSize(18).TextColor(t.Success)
				dialogTitle(c, t, "分享链接已创建", countLabel(d.files)+" · "+periodLabel(sharePeriods[f.period]))
			})
			ui.Column(c).Padding(12).Gap(8).Radius(5).Border(1, p.Hairline).Background(t.Surface).Children(func() {
				ui.Text(c, f.link).Font(fontMono).FontSize(t.Rem(0.9)).Selectable()
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "提取码").TextColor(t.TextMuted)
					ui.Text(c, f.pwd).Font(fontMono).FontSize(18).FontWeight(500).LetterSpacing(4).TextColor(t.Accent).Selectable()
				})
			})
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if pill(c, t, p, "", "完成", false).Clicked() {
					d.open = false
				}
				if pill(c, t, p, "copy", "复制链接和提取码", true).AutoFocus().Clicked() {
					a.copy(shareText(f.link, f.pwd), "已复制分享链接")
					d.open = false
				}
			})
		})
		return
	}
	ui.Column(c).Key("form").Gap(16).Transition(popIn).Children(func() {
		dialogTitle(c, t, "分享"+countLabel(d.files), "")
		ui.Column(c).Gap(8).Children(func() {
			ui.Text(c, "有效期").TextColor(t.TextMuted).FontSize(t.Rem(0.9))
			tabs(c, t, p, &f.period, "1 天", "7 天", "30 天", "永久").FillWidth()
		})
		ui.Column(c).Gap(8).Children(func() {
			ui.Text(c, "提取码").TextColor(t.TextMuted).FontSize(t.Rem(0.9))
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &f.pwd).Width(120).Height(36).Font("monospace").Label("提取码")
				if len(f.pwd) > 4 {
					f.pwd = f.pwd[:4]
				}
				if iconButton(c, t, p, "refresh-cw", "换一个").Clicked() {
					f.pwd = newPassword()
				}
				ui.Text(c, "4 位字母或数字").TextColor(t.TextMuted).FontSize(t.Rem(0.86))
			})
		})
		valid := len(f.pwd) == 4 && isAlnum(f.pwd)
		if a.dialogButtons(c, t, p, "创建链接", false, !valid) {
			files, pwd, days := d.files, f.pwd, sharePeriods[f.period]
			s := a.sess
			d.busy, d.err = true, ""
			go func() {
				link, err := s.client.CreateShare(context.Background(), paths(files), pwd, days)
				a.update(func() {
					d.busy = false
					if err != nil {
						d.err = err.Error()
						return
					}
					f.link, f.created = link, true
					a.sharesDirty = true
				})
			}()
		}
	})
}

func isAlnum(s string) bool {
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func periodLabel(days int) string {
	if days == 0 {
		return "永久有效"
	}
	return itoa(days) + " 天有效"
}

// saveLinkDialog saves a share link someone gave into the netdisk.
func (a *App) saveLinkDialog(c *ui.Context, t *ui.Theme, p *palette) {
	f := &a.shareForm
	dialogTitle(c, t, "转存分享链接", "粘贴链接，提取码会自动识别")
	ui.TextArea(c, &f.text).Lines(2, 4).AutoFocus().Placeholder("https://pan.baidu.com/s/1… 提取码: abcd").Label("分享链接")
	link, pwd, ok := baidu.ParseShareLink(f.text)
	if ok {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(8, 12).Radius(5).Background(p.Selected).Transition(rowEnter).Children(func() {
			ui.Icon(c, icon("link")).FontSize(15).TextColor(t.Accent)
			ui.Text(c, strings.TrimPrefix(link, "https://")).SingleLine().Shrink(1)
			ui.Spacer(c)
			if pwd != "" {
				ui.Text(c, "提取码 "+pwd).Font(fontMono).FontWeight(500).TextColor(t.Accent)
			} else {
				ui.Text(c, "未识别到提取码").TextColor(t.Warning)
			}
		})
	}
	ui.Text(c, "保存到").TextColor(t.TextMuted).FontSize(t.Rem(0.9))
	a.folderChooser(c, t, p, &f.picker, 180, nil)
	if a.dialogButtons(c, t, p, "保存到「"+dirLabel(f.picker.dir)+"」", false, !ok || len(pwd) != 4) {
		dir := f.picker.dir
		a.run(func(ctx context.Context, c *baidu.Client) error {
			_, err := c.SaveShare(ctx, link, pwd, dir)
			return err
		}, []string{dir}, "已转存到「"+dirLabel(dir)+"」", nil)
	}
}

// sharesState is the page of the account's shares.
type sharesState struct {
	items   []baidu.Share
	loaded  bool
	loading bool
	err     string
	list    ui.ListState
	busy    map[int64]bool
}

func (a *App) loadShares() {
	s := &a.shares
	if a.sess == nil || s.loading {
		return
	}
	s.loading, s.err = true, ""
	c := a.sess.client
	go func() {
		items, err := c.Shares(context.Background())
		a.update(func() {
			s.loading = false
			if err != nil {
				s.err = err.Error()
				return
			}
			s.items, s.loaded = items, true
		})
	}()
}

func (a *App) sharesPage(c *ui.Context, t *ui.Theme, p *palette) {
	s := &a.shares
	if (!s.loaded || a.sharesDirty) && !s.loading {
		a.sharesDirty = false
		a.loadShares()
	}
	if s.busy == nil {
		s.busy = map[int64]bool{}
	}
	ui.Column(c).Grow(1).Children(func() {
		switch {
		case !s.loaded && s.err != "":
			emptyState(c, t, p, "triangle-alert", "无法读取分享", s.err, a.loadShares)
		case !s.loaded:
			skeletonRows(c, t, p)
		case len(s.items) == 0:
			emptyState(c, t, p, "share-2", "还没有分享", "在文件上右键选择「分享」即可创建链接", nil)
		default:
			now := c.Now()
			ui.List(c, &s.list, len(s.items), func(i int) {
				it := s.items[i]
				row := ui.Row(c.Key(it.ID)).Height(rowHeight + 6).PaddingX(12).Gap(12).AlignItems(ui.Center).Radius(5)
				if row.Hovered() {
					row.Background(p.Hover)
				}
				f := baidu.File{Name: path.Base(it.Path), Path: it.Path, IsDir: path.Ext(it.Path) == ""}
				row.Children(func() {
					a.fileIcon(c, t, p, &f, 36)
					ui.Column(c).Grow(1).Shrink(1).Gap(3).Children(func() {
						name := f.Name
						if len(it.FsIDs) > 1 {
							name += " 等 " + itoa(len(it.FsIDs)) + " 项"
						}
						ui.Text(c, name).FontWeight(500).SingleLine()
						ui.Text(c, strings.TrimPrefix(it.Link, "https://")+" · "+itoa(it.Views)+" 次浏览").
							FontSize(t.Rem(0.84)).TextColor(t.TextMuted).SingleLine()
					})
					status, color := "永久有效", t.Success
					switch {
					case it.Expired(now):
						status, color = "已失效", t.TextMuted
					case it.Expires > 0:
						left := time.Unix(it.Expires, 0).Sub(now)
						status = itoa(int(left.Hours()/24)+1) + " 天后过期"
						if left < 48*time.Hour {
							color = t.Warning
						}
					}
					ui.Text(c, status).FontSize(t.Rem(0.84)).Padding(2, 8).Radius(6).Background(color.Alpha(0.12)).TextColor(color)
					if s.busy[it.ID] {
						ui.Spinner(c).Label("处理中")
						return
					}
					if iconButton(c, t, p, "copy", "复制链接和提取码").Clicked() {
						a.copyShare(it)
					}
					cancel := iconButton(c, t, p, "x", "取消分享")
					if cancel.Hovered() {
						cancel.TextColor(t.Danger)
					}
					if cancel.Clicked() {
						a.cancelShare(it)
					}
				})
			}).Grow(1).Padding(4, 12, 24)
		}
	})
}

func (a *App) copyShare(it baidu.Share) {
	s := &a.shares
	s.busy[it.ID] = true
	c := a.sess.client
	go func() {
		pwd, err := c.SharePassword(context.Background(), it.ID)
		a.update(func() {
			delete(s.busy, it.ID)
			if err != nil {
				a.toast(err.Error(), "error")
				return
			}
			a.copy(shareText(it.Link, pwd), "已复制分享链接")
		})
	}()
}

func (a *App) cancelShare(it baidu.Share) {
	s := &a.shares
	s.busy[it.ID] = true
	c := a.sess.client
	go func() {
		err := c.CancelShares(context.Background(), []int64{it.ID})
		a.update(func() {
			delete(s.busy, it.ID)
			if err != nil {
				a.toast(err.Error(), "error")
				return
			}
			s.items = slices.DeleteFunc(s.items, func(x baidu.Share) bool { return x.ID == it.ID })
			a.toast("已取消分享", "success")
		})
	}()
}

// sharesToolbar is the title bar's controls on the shares page.
func (a *App) sharesToolbar(c *ui.Context, t *ui.Theme, p *palette) {
	if iconButton(c, t, p, "refresh-cw", "刷新").Clicked() {
		a.loadShares()
	}
	if pill(c, t, p, "link", "转存链接", true).Clicked() {
		a.ask(dlgSaveLink, "/", nil)
		a.shareForm = shareForm{picker: picker{dir: "/"}}
	}
}
