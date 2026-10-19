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
		ui.Column(c).MaxWidth(720).Gap(22).Children(func() {
			a.accountSection(c, t, p)
			a.tuneSection(c, t, p)
			section(c, t, p, "预览", func() {
				mb := float64(s.PreviewMB)
				if mb == 0 {
					mb = 50
				}
				settingRow(c, t, p, "image", "内存预览上限", "超过这个大小的图片和文本不在内存中打开", func() {
					ui.Text(c, itoa(int(mb))+" MB").Width(64).TextAlign(ui.End).FontFeatures("tnum").
						ContentTransition(ui.ContentTransition{Numeric: true})
					if ui.Slider(c, &mb, 10, 500).Width(180).Label("预览上限").Changed() {
						s.PreviewMB = int(math.Round(mb/10) * 10)
						a.applySettings()
					}
				})
				path, err := player.Find(s.MPV)
				detail := path
				if err != nil {
					detail = "未找到 mpv，视频将无法播放"
				}
				settingRow(c, t, p, "film", "mpv 播放器", detail, func() {
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
				settingRow(c, t, p, "sparkles", "BaiduNetGo "+Version, "以 MyGo 原生界面与 BaiduPCS-Go 构建，预览全程不落盘", func() {
					if pill(c, t, p, "external-link", "项目主页", false).Clicked() {
						c.OpenURL("https://github.com/pengstem/baidunetgo")
					}
				})
			})
		})
	})
}

// section is a titled card of settings.
func section(c *ui.Context, t *ui.Theme, p *palette, title string, rows func()) {
	ui.Column(c).Gap(8).Transition(rowEnter).Children(func() {
		ui.Text(c, title).FontWeight(650).TextColor(t.TextMuted).FontSize(t.Rem(0.9)).PaddingX(4)
		ui.Column(c).Radius(14).Background(p.Card).Border(1, p.Hairline).Dividers(1, p.Hairline).Clip().Children(rows)
	})
}

// settingRow is one setting: what it is, and its control at the end.
func settingRow(c *ui.Context, t *ui.Theme, p *palette, ic, title, detail string, control func()) {
	ui.Row(c).MinHeight(58).Padding(10, 16).Gap(14).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(32, 32).Radius(9).Center().Background(p.AccentSoft).Children(func() {
			ui.Icon(c, icon(ic)).FontSize(16).TextColor(t.Accent)
		})
		ui.Column(c).Grow(1).Shrink(1).Gap(2).Children(func() {
			ui.Text(c, title).FontWeight(560)
			if detail != "" {
				ui.Text(c, detail).FontSize(t.Rem(0.84)).TextColor(t.TextMuted).SingleLine()
			}
		})
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

// accountSection is who is signed in and their membership.
func (a *App) accountSection(c *ui.Context, t *ui.Theme, p *palette) {
	sess := a.sess
	if sess == nil || sess.user == nil {
		return
	}
	u := sess.user
	section(c, t, p, "账户", func() {
		ui.Row(c).MinHeight(64).Padding(12, 16).Gap(14).AlignItems(ui.Center).Children(func() {
			ui.Avatar(c, u.Name, sess.avatar).Size(40, 40)
			ui.Column(c).Grow(1).Shrink(1).Gap(3).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, u.Name).FontWeight(600).SingleLine().Shrink(1)
					if l := u.VIPLabel(); l != "" {
						vipBadge(c, l)
					}
				})
				detail := "普通用户：下载速度受限，自动优化会用较少的连接"
				switch {
				case u.VIP > 0 && !u.VIPUntil.IsZero():
					detail = u.VIPLabel() + " 会员，" + u.VIPUntil.Format("2006 年 1 月 2 日") + "到期"
				case u.VIP > 0:
					detail = u.VIPLabel() + " 会员"
				}
				ui.Text(c, detail).FontSize(t.Rem(0.84)).TextColor(t.TextMuted)
			})
			b := pill(c, t, p, "log-out", "退出登录", false).TextColor(t.Danger)
			if b.Clicked() {
				a.signOut()
			}
		})
	})
}

// vipBadge is the golden label of a membership.
func vipBadge(c *ui.Context, label string) ui.Element {
	return ui.Text(c, label).FontSize(10).FontWeight(700).Padding(1, 5).Radius(5).
		TextColor(ui.Hex("#5b3a00")).
		LinearGradient(ui.LinearGradient{Stops: []ui.GradientStop{{Color: ui.Hex("#fde68a"), At: 0}, {Color: ui.Hex("#f59e0b"), At: 1}}, Angle: 90})
}
