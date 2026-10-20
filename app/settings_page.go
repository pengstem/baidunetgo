package app

import (
	"math"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/player"
)

// Version is the app's version, set by the build.
var Version = "0.1.0"

// applySettings saves the settings and lets what runs follow them.
func (a *App) applySettings() {
	if err := a.settings.save(); err != nil {
		a.toast("无法保存设置："+err.Error(), "error")
	}
	a.useSettings()
}

// useSettings lets what runs follow the settings.
func (a *App) useSettings() {
	cfg := a.transferConfig()
	if a.xfer != nil {
		a.xfer.Threads, a.xfer.Parallel = cfg.Threads, cfg.Parallel
	}
	a.useStreaming(cfg)
	if mb := a.settings.PreviewMB; mb > 0 {
		imageLimit = int64(mb) << 20
		textLimit = int64(min(mb, 64)) << 20
		pdfLimit = int64(max(mb, 200)) << 20
	}
}

func (a *App) settingsPage(c *ui.Context, t *ui.Theme, p *palette) {
	s := &a.settings
	ui.Scroll(c).Grow(1).Padding(8, 24, 40).Children(func() {
		ui.Column(c).FillWidth().MaxWidth(720).Margin(0, ui.Auto).Gap(28).Children(func() {
			a.accountSection(c, t, p)
			a.tuneSection(c, t, p)
			section(c, t, p, "预览", func() {
				mb := float64(s.PreviewMB)
				if mb == 0 {
					mb = 50
				}
				settingRow(c, t, p, "内存预览上限", "", func() {
					in := ui.NumberInput(c, &mb, 10, 2000, 10).Font(fontMono).Label("内存预览上限（MB）")
					ui.Text(c, "MB").Font(fontMono).FontSize(12).TextColor(p.Faint)
					if in.Changed() {
						s.PreviewMB = int(math.Round(mb))
						a.applySettings()
					}
				})
				path, err := player.Find(s.MPV)
				settingRow(c, t, p, "mpv", "", func() {
					if err != nil {
						ui.Text(c, "未找到，视频无法播放").FontSize(12).TextColor(t.Danger).SingleLine()
					} else {
						figure(c, path).FontSize(12).TextColor(t.TextMuted).MaxWidth(320)
					}
					if s.MPV != "" && pill(c, t, p, "", "自动查找", false).Clicked() {
						s.MPV = ""
						a.applySettings()
					}
					if pill(c, t, p, "", "选择…", false).Clicked() {
						a.pickMPV()
					}
				})
			})
			section(c, t, p, "关于", func() {
				settingRow(c, t, p, "BaiduNetGo", "", func() {
					figure(c, Version).FontSize(12).TextColor(t.TextMuted)
					if pill(c, t, p, "", "项目主页", false).Clicked() {
						c.OpenURL("https://github.com/pengstem/baidunetgo")
					}
				})
			})
		})
	})
}

// section is a group of settings under a small label, its rows parted by
// lines rather than held in a card.
func section(c *ui.Context, t *ui.Theme, p *palette, title string, rows func()) {
	ui.Column(c).Gap(6).Transition(rowEnter).Children(func() {
		ui.Text(c, title).Font(fontMono).FontSize(11).FontWeight(500).TextColor(p.Faint)
		ui.Column(c).BorderWidth(1, 0, 1, 0).BorderColor(p.Hairline).Dividers(1, p.Hairline).Children(rows)
	})
}

// settingRow is one setting on one line: what it is, a word more when
// needed, and its control at the end.
func settingRow(c *ui.Context, t *ui.Theme, p *palette, title, detail string, control func()) {
	ui.Row(c).MinHeight(44).Padding(6, 0).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontWeight(500).SingleLine()
		if detail != "" {
			ui.Text(c, detail).FontSize(12).TextColor(p.Faint).SingleLine().Shrink(1)
		}
		ui.Spacer(c)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(control)
	})
}

func (a *App) pickDownloadDir() {
	win := a.win
	go func() {
		dirs, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: "选择下载位置", Directory: true, CreateDirectories: true, DefaultPath: a.settings.downloadDir()})
		if err != nil || len(dirs) == 0 {
			return
		}
		a.update(func() {
			a.settings.DownloadDir = dirs[0]
			a.applySettings()
		})
	}()
}

func (a *App) pickMPV() {
	win := a.win
	go func() {
		files, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: "选择 mpv"})
		if err != nil || len(files) == 0 {
			return
		}
		a.update(func() {
			a.settings.MPV = files[0]
			a.applySettings()
		})
	}()
}

// accountSection is who is signed in and their membership, on one line.
func (a *App) accountSection(c *ui.Context, t *ui.Theme, p *palette) {
	sess := a.sess
	if sess == nil || sess.user == nil {
		return
	}
	u := sess.user
	section(c, t, p, "账户", func() {
		ui.Row(c).MinHeight(44).Padding(6, 0).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, u.Name).FontWeight(500).SingleLine().Shrink(1)
			if l := u.VIPLabel(); l != "" {
				tag(c, l, p.VIP)
			}
			switch {
			case u.VIP > 0 && !u.VIPUntil.IsZero():
				figure(c, "至 "+u.VIPUntil.Format("2006-01-02")).FontSize(12).TextColor(p.Faint)
			case u.VIP == 0:
				ui.Text(c, "普通用户，下载限速").FontSize(12).TextColor(p.Faint)
			}
			ui.Spacer(c)
			b := ui.ButtonBase(c).Label("退出登录").Height(28).PaddingX(10).Radius(5).TextColor(t.Danger)
			if b.Hovered() {
				b.Background(t.Danger.Alpha(0.1))
			}
			b.Children(func() { ui.Text(c, "退出登录").FontWeight(500) })
			if b.Clicked() {
				a.signOut()
			}
		})
	})
}

// vipBadge is the outlined label of a membership.
func vipBadge(c *ui.Context, label string) ui.Element {
	return tag(c, label, darkPalette.VIP)
}
