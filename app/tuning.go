package app

import (
	"context"
	"strconv"
	"time"

	"github.com/egoist/mygo/plugins/loading"
	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/remote"
	"github.com/pengstem/baidunetgo/internal/tune"
)

// tuneMaxAge is how long a measurement lasts before it is made again.
const tuneMaxAge = 7 * 24 * time.Hour

// tunePer is how long each measurement of a probe lasts, a variable for
// tests.
var tunePer = 5 * time.Second

// vip returns the membership of the signed-in account.
func (a *App) vip() int {
	if a.sess != nil && a.sess.user != nil {
		return a.sess.user.VIP
	}
	return 0
}

// transferConfig returns the transfer settings in force: those the
// membership and the last measurement call for, with the user's counts
// of connections and files when they chose them.
func (a *App) transferConfig() tune.Config {
	cfg := tune.Recommend(a.vip(), a.measured())
	if s := a.settings; s.Manual {
		cfg.Threads, cfg.Parallel, cfg.Upload = s.threads(), s.parallel(), s.upload()
	}
	return cfg
}

// measured returns the last measurement, if it was made with the
// membership the account has now, of all a probe measures now.
func (a *App) measured() *tune.Result {
	if t := a.settings.Tune; t != nil && t.VIP == a.vip() && t.Result != nil && t.Result.Version == tune.Version {
		return t.Result
	}
	return nil
}

// maybeTune measures download speeds once signed in, when there is no
// recent measurement of this membership.
func (a *App) maybeTune() {
	if a.settings.Manual || a.fake || a.tuning {
		return
	}
	if r := a.measured(); r != nil && time.Since(r.At) < tuneMaxAge {
		return
	}
	a.runTune(false)
}

// runTune measures transfers, on large files of the account read into
// nowhere and uploads never finished, and keeps the result.
func (a *App) runTune(announce bool) {
	s := a.sess
	if s == nil || a.tuning {
		return
	}
	a.tuning, a.tuneErr, a.tuneStage = true, "", ""
	vip := a.vip()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		files, err := tune.FindFiles(ctx, s.client, 128<<20, 2)
		if err != nil {
			files, err = tune.FindFiles(ctx, s.client, 24<<20, 2)
		}
		var res *tune.Result
		if err == nil {
			budget := int64(400 << 20)
			if vip == 2 {
				budget = 1200 << 20
			}
			res, err = tune.Probe(ctx, s.client, files, tune.Options{
				VIP: vip, Per: tunePer, Budget: budget, Upload: true,
				Stage: func(name string) {
					a.update(func() {
						if a.tuning {
							a.tuneStage = name
						}
					})
				},
			})
		}
		a.update(func() {
			a.tuning, a.tuneStage = false, ""
			if a.sess != s {
				return
			}
			if err != nil {
				a.tuneErr = err.Error()
				if announce {
					a.toast("测速失败："+err.Error(), "error")
				}
				return
			}
			a.settings.Tune = &tuneRecord{VIP: vip, Result: res}
			if err := a.settings.save(); err != nil {
				a.toast("无法保存设置："+err.Error(), "error")
			}
			a.useSettings()
			if announce {
				a.tuneOpen = true
			}
			cfg := a.transferConfig()
			a.toast("已根据测速优化：每个文件 "+itoa(cfg.Threads)+" 个连接，同时 "+itoa(cfg.Parallel)+" 个文件，约 "+humanSpeed(res.Total(cfg)), "success")
		})
	}()
}

// tuneSection is the settings of transfers, chosen for the user or by
// them.
func (a *App) tuneSection(c *ui.Context, t *ui.Theme, p *palette) {
	s := &a.settings
	cfg := a.transferConfig()
	section(c, t, p, "传输", func() {
		settingRow(c, t, p, "下载位置", tildePath(s.downloadDir()), func() {
			if pill(c, t, p, "", "更改…", false).Clicked() {
				a.pickDownloadDir()
			}
		})
		auto := !s.Manual
		settingRow(c, t, p, "自动优化传输", "按实测选线路、连接数、块大小", func() {
			if ui.Switch(c, &auto).Label("自动优化传输").Changed() {
				s.Manual = !auto
				if !s.Manual {
					s.Threads, s.Parallel, s.Upload = 0, 0, 0
				} else {
					s.Threads, s.Parallel, s.Upload = cfg.Threads, cfg.Parallel, cfg.Upload
				}
				a.applySettings()
				if auto {
					a.maybeTune()
				}
			}
		})
		if auto {
			a.tuneResult(c, t, p, cfg)
		}
		manual := func() {
			s.Manual = true
			a.applySettings()
		}
		threads, parallel, upload := float64(cfg.Threads), float64(cfg.Parallel), float64(cfg.Upload)
		chosen := "自动"
		if !auto {
			chosen = "手动"
		}
		threadsDetail := chosen
		if !auto && a.vip() == 0 {
			threadsDetail += " · 非会员建议 1–4"
		}
		settingRow(c, t, p, "每个文件的连接数", threadsDetail, func() {
			if numberField(c, t, p, &threads, 1, 64, 1, "", "每个文件的连接数") {
				s.Threads, s.Parallel, s.Upload = int(threads), cfg.Parallel, cfg.Upload
				manual()
			}
		})
		settingRow(c, t, p, "同时传输的文件数", chosen, func() {
			if numberField(c, t, p, &parallel, 1, 6, 1, "", "同时传输的文件数") {
				s.Threads, s.Parallel, s.Upload = cfg.Threads, int(parallel), cfg.Upload
				manual()
			}
		})
		settingRow(c, t, p, "每个上传的连接数", chosen, func() {
			if numberField(c, t, p, &upload, 1, 32, 1, "", "每个上传的连接数") {
				s.Threads, s.Parallel, s.Upload = cfg.Threads, cfg.Parallel, int(upload)
				manual()
			}
		})
		limit := func(name, what string, mbps *int) {
			v := float64(*mbps)
			detail := "不限速"
			if *mbps > 0 {
				detail = "所有" + what + "加起来最多 " + itoa(*mbps) + " MB/s"
			}
			settingRow(c, t, p, name, detail, func() {
				if numberField(c, t, p, &v, 0, 1000, 1, "MB/s", name+"（MB/s，0 为不限）") {
					*mbps = int(v)
					a.applySettings()
				}
			})
		}
		limit("下载限速", "下载", &s.DownLimit)
		limit("上传限速", "上传", &s.UpLimit)
	})
}

// tuneBar is one measured value, its speed drawn as a row of cells.
type tuneBar struct {
	label  string
	speed  float64
	chosen bool
	failed bool // refused at times
}

// tuneLabelWidth is the width of the values before the bars, one for every
// setting so that the bars line up.
const tuneLabelWidth = 132

// tuneResult shows the last measurement and, unfolded, for each setting
// a bar for each value measured, the chosen one in the accent color.
func (a *App) tuneResult(c *ui.Context, t *ui.Theme, p *palette, cfg tune.Config) {
	r := a.measured()
	detail := "还没有测速"
	switch {
	case a.tuning && a.tuneStage != "":
		detail = "测速中 · " + a.tuneStage
	case a.tuning:
		detail = "测速中…"
	case a.tuneErr != "":
		detail = a.tuneErr
	case r != nil:
		detail = humanTime(r.At.Unix(), c.Now()) + " · 约 " + humanSpeed(r.Total(cfg))
	}
	ui.Column(c).Children(func() {
		settingRow(c, t, p, "实测速度", detail, func() {
			if r != nil {
				d := ui.ButtonBase(c).Label("详情").Height(28).PaddingX(8).Radius(7).Gap(3).AlignItems(ui.Center).TextColor(t.TextMuted)
				if d.Hovered() {
					d.Background(p.Hover).TextColor(t.Text)
				}
				turn := d.AnimateTo("turn", map[bool]float32{true: 1, false: 0}[a.tuneOpen], motionPanel)
				d.Children(func() {
					ui.Text(c, "详情").FontSize(12).SingleLine()
					ui.Icon(c, icon("chevron-down")).FontSize(12).Rotate(turn * 180)
				})
				if d.Clicked() {
					a.tuneOpen = !a.tuneOpen
				}
			}
			b := pill(c, t, p, "", "重测", false).Disabled(a.tuning)
			if a.tuning {
				// A radar sweeps while the hosts and settings are measured.
				b.Children(func() { loading.Radar(c, loading.Options{Size: 14, Color: t.Accent}).Label("测速中") })
			}
			if b.Clicked() {
				a.runTune(true)
			}
		})
		if r == nil || !a.tuneOpen {
			return
		}
		ui.Column(c).Gap(12).Padding(4, 0, 14).Transition(rowEnter).Children(func() {
			row := 0
			group := func(title, note string, bars []tuneBar) {
				if len(bars) == 0 {
					return
				}
				fastest := 0.0
				for _, b := range bars {
					fastest = max(fastest, b.speed)
				}
				ui.Column(c.Key(title)).Gap(4).Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
						ui.Text(c, title).FontSize(11).FontWeight(600).TextColor(t.TextMuted)
						if note != "" {
							ui.Text(c, note).FontSize(11).TextColor(p.Faint).SingleLine().Shrink(1)
						}
					})
					for _, b := range bars {
						a.tuneBarRow(c, t, p, b, fastest, row, r.At)
						row++
					}
				})
			}

			var hosts []tuneBar
			for _, s := range r.Hosts {
				hosts = append(hosts, tuneBar{label: s.Host, speed: s.Speed, failed: s.Errors > 0,
					chosen: len(cfg.Hosts) > 0 && s.Host == cfg.Hosts[0]})
			}
			group("下载线路", "", hosts)
			group("每个文件的连接数", "", counts(r.Threads, int64(cfg.Threads)))
			var blocks []tuneBar
			for _, s := range r.Blocks {
				blocks = append(blocks, tuneBar{label: humanSize(s.Value), speed: s.Speed, failed: s.Errors > 0, chosen: s.Value == cfg.Block})
			}
			group("请求块大小", "", blocks)
			group("同时传输的文件数", "总速度", counts(r.Files, int64(cfg.Parallel)))
			group("每个上传的连接数", r.UploadErr, counts(r.Upload, int64(cfg.Upload)))
			ui.Text(c, "播放与预览预读 "+itoa(cfg.Ahead)+" 个连接").FontSize(11).TextColor(p.Faint)
		})
	})
}

// counts makes bars of samples whose values are counts.
func counts(samples []tune.Sample, chosen int64) []tuneBar {
	var bars []tuneBar
	for _, s := range samples {
		bars = append(bars, tuneBar{label: strconv.FormatInt(s.Value, 10), speed: s.Speed, failed: s.Errors > 0, chosen: s.Value == chosen})
	}
	return bars
}

// tuneBarRow draws one value's speed as a slim rounded bar growing in,
// after those above it: the chosen one in the accent color.
func (a *App) tuneBarRow(c *ui.Context, t *ui.Theme, p *palette, b tuneBar, fastest float64, i int, at time.Time) {
	ui.Row(c.Key(b.label)).Height(18).Gap(12).AlignItems(ui.Center).FontSize(12).FontFeatures("tnum").
		TextColor(map[bool]ui.Color{true: t.Text, false: t.TextMuted}[b.chosen]).Children(func() {
		ui.Text(c, b.label).Width(tuneLabelWidth).SingleLine()
		frac := float32(0)
		if fastest > 0 {
			frac = float32(b.speed / fastest)
		}
		fill := p.Faint
		if b.chosen {
			fill = t.Accent
		}
		bar := ui.Box(c).Grow(1).Height(6).Radius(3).Background(p.Hairline)
		shown := bar.Keyframes("grow", []ui.Keyframe{{At: 0, Value: 0}, {At: 360 * time.Millisecond, Value: frac, Ease: easeStandard}},
			ui.KeyframeOptions{Delay: time.Duration(i) * 30 * time.Millisecond, Run: uint64(at.UnixNano())})
		bar.Draw(func(pt *ui.Painter, r ui.Rect) {
			if w := r.W * shown; w > 0 {
				pt.Fill(ui.Rect{X: r.X, Y: r.Y, W: max(w, r.H), H: r.H}, fill, r.H/2)
			}
		})
		speed := humanSpeed(b.speed)
		if b.failed {
			speed += "!"
		}
		ui.Text(c, speed).Width(72).TextAlign(ui.End).FontWeight(map[bool]int{true: 600, false: 400}[b.chosen])
	})
}

// useStreaming lets previews and players follow the transfer settings.
func (a *App) useStreaming(cfg tune.Config) {
	remote.Parallel = min(max(cfg.Ahead, 2), 8)
	if a.stream != nil {
		a.stream.Ahead = min(max(cfg.Ahead, 2), 8)
	}
}
