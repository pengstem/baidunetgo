package app

import (
	"math"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/player"
)

// version is the app's version, which mygo build links in from mygo.json.
// Builds run with go run or mygo dev have none.
func version() string {
	if v := mygo.App.Version(); v != "" {
		return v
	}
	return "开发版"
}

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
		a.xfer.Threads, a.xfer.Parallel, a.xfer.Upload, a.xfer.Block = cfg.Threads, cfg.Parallel, cfg.Upload, cfg.Block
	}
	if a.sess != nil {
		a.sess.client.SetHosts(cfg.Hosts)
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
	ui.Scroll(c).Grow(1).Padding(12, 24, 40).Children(func() {
		ui.Column(c).FillWidth().MaxWidth(600).Margin(0, ui.Auto).Gap(24).Children(func() {
			a.accountSection(c, t, p)
			a.tuneSection(c, t, p)
			section(c, t, p, "预览", func() {
				mb := float64(s.PreviewMB)
				if mb == 0 {
					mb = 50
				}
				settingRow(c, t, p, "image", tilePink, "内存预览上限", "更大的图片、文档会先下载再打开", func() {
					if numberField(c, t, p, &mb, 10, 2000, 10, "MB", "内存预览上限（MB）") {
						s.PreviewMB = int(math.Round(mb))
						a.applySettings()
					}
				})
				path, err := player.Find(s.MPV)
				detail := tildePath(path)
				if err != nil {
					detail = "未找到，视频无法播放"
				}
				settingRow(c, t, p, "film", tileIndigo, "mpv 播放器", detail, func() {
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
				settingRow(c, t, p, "info", tileGray, "BaiduNetGo", version()+" · 百度网盘的原生桌面客户端", func() {
					if pill(c, t, p, "external-link", "项目主页", false).Clicked() {
						c.OpenURL("https://github.com/pengstem/baidunetgo")
					}
				})
			})
		})
	})
}

// Colors of the tiles marking settings, the system's as the sidebar's.
var (
	tileBlue   = ui.Hex("#2f7cf6")
	tileGreen  = ui.Hex("#34c759")
	tileOrange = ui.Hex("#ff9500")
	tileTeal   = ui.Hex("#30b0c7")
	tilePurple = ui.Hex("#af52de")
	tilePink   = ui.Hex("#ff2d55")
	tileIndigo = ui.Hex("#5856d6")
	tileGray   = ui.Hex("#8e8e93")
)

// section is a group of settings under its title: a rounded gray panel,
// its rows parted by lines inset from its edges, as System Settings'.
func section(c *ui.Context, t *ui.Theme, p *palette, title string, rows func()) {
	ui.Column(c).Gap(8).Transition(rowEnter).Children(func() {
		ui.Text(c, title).FontSize(13).FontWeight(600).PaddingX(6)
		group(c, p).PaddingX(12).Dividers(1, p.Hairline).Children(rows)
	})
}

// settingTile is a setting's icon, white on a small tile of its color, as
// the sidebar's pages.
func settingTile(c *ui.Context, ic string, tint ui.Color) {
	ui.Box(c).Size(24, 24).Radius(6).Center().Shrink(0).Background(tint).Children(func() {
		ui.Icon(c, icon(ic)).FontSize(14).TextColor(ui.Hex("#ffffff"))
	})
}

// settingRow is one setting: its tile, what it is with a line more under
// it when needed, and its control at the end.
func settingRow(c *ui.Context, t *ui.Theme, p *palette, ic string, tint ui.Color, title, detail string, control func()) {
	ui.Row(c).MinHeight(48).Padding(8, 0).Gap(12).AlignItems(ui.Center).Children(func() {
		settingTile(c, ic, tint)
		ui.Column(c).Grow(1).Shrink(1).Gap(2).Children(func() {
			ui.Text(c, title).SingleLine()
			if detail != "" {
				ui.Text(c, detail).FontSize(11).TextColor(t.TextMuted).SingleLine()
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Shrink(0).Children(control)
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

// accountSection is who is signed in, their membership and how full
// their netdisk is.
func (a *App) accountSection(c *ui.Context, t *ui.Theme, p *palette) {
	sess := a.sess
	if sess == nil || sess.user == nil {
		return
	}
	u := sess.user
	section(c, t, p, "账户", func() {
		ui.Row(c).MinHeight(64).Padding(12, 0).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(40, 40).Radius(20).Center().Shrink(0).Background(t.Accent).Children(func() {
				ui.Icon(c, icon("user")).FontSize(20).TextColor(t.AccentText)
			})
			ui.Column(c).Grow(1).Shrink(1).Gap(3).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, u.Name).FontSize(15).FontWeight(600).SingleLine().Shrink(1)
					if l := u.VIPLabel(); l != "" {
						chip(c, l, p.VIP)
					}
				})
				detail := ""
				switch {
				case u.VIP > 0 && !u.VIPUntil.IsZero():
					detail = "会员至 " + u.VIPUntil.Format("2006年1月2日")
				case u.VIP == 0:
					detail = "普通用户，下载限速"
				}
				if detail != "" {
					figure(c, detail).FontSize(11).TextColor(t.TextMuted)
				}
			})
			b := pill(c, t, p, "log-out", "退出登录", false).TextColor(t.Danger)
			if b.Clicked() {
				a.signOut()
			}
		})
		if q := sess.quota; q.Total > 0 {
			frac := float32(q.Used) / float32(q.Total)
			settingRow(c, t, p, "hard-drive", tileBlue, "网盘空间", "已用 "+humanSize(q.Used)+"，共 "+humanSize(q.Total), func() {
				fill := t.Accent
				if frac > 0.9 {
					fill = t.Danger
				}
				bar := ui.Box(c).Size(140, 6).Radius(3).Background(p.Hairline)
				bar.Draw(func(pt *ui.Painter, r ui.Rect) {
					if w := r.W * min(frac, 1); w > 0 {
						pt.Fill(ui.Rect{X: r.X, Y: r.Y, W: max(w, r.H), H: r.H}, fill, r.H/2)
					}
				})
				figure(c, itoa(int(math.Round(float64(frac)*100)))+"%").FontSize(12).TextColor(t.TextMuted).Width(34).TextAlign(ui.End)
			})
		}
	})
}
