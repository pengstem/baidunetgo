package app

import (
	"context"
	"strconv"
	"time"

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
		settingRow(c, t, p, "下载位置", "", func() {
			figure(c, tildePath(s.downloadDir())).FontSize(12).TextColor(t.TextMuted).MaxWidth(320)
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
		detail := ""
		if !auto {
			detail = "非会员建议 1–4"
		}
		settingRow(c, t, p, "每个文件的连接数", detail, func() {
			if ui.NumberInput(c, &threads, 1, 64, 1).Font(fontMono).Label("连接数").Changed() {
				s.Threads, s.Parallel, s.Upload = int(threads), cfg.Parallel, cfg.Upload
				manual()
			}
		})
		settingRow(c, t, p, "同时传输的文件数", "", func() {
			if ui.NumberInput(c, &parallel, 1, 6, 1).Font(fontMono).Label("同时传输数").Changed() {
				s.Threads, s.Parallel, s.Upload = cfg.Threads, int(parallel), cfg.Upload
				manual()
			}
		})
		settingRow(c, t, p, "每个上传的连接数", "", func() {
			if ui.NumberInput(c, &upload, 1, 32, 1).Font(fontMono).Label("上传连接数").Changed() {
				s.Threads, s.Parallel, s.Upload = cfg.Threads, cfg.Parallel, int(upload)
				manual()
			}
		})
	})
}

// tuneBar is one measured value, its speed drawn as a row of cells.
type tuneBar struct {
	label  string
	speed  float64
	chosen bool
	failed bool // refused at times
}

// tuneResult shows the last measurement: for each setting, a row of
// cells for each value measured, the chosen one in the accent color.
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
	ui.Column(c).Padding(0, 0, 12).Gap(6).Children(func() {
		settingRow(c, t, p, "实测速度", detail, func() {
			b := pill(c, t, p, "", "重测", false).Disabled(a.tuning)
			if a.tuning {
				b.Children(func() { ui.Spinner(c) })
			}
			if b.Clicked() {
				a.runTune(true)
			}
		})
		if r == nil {
			return
		}
		row := 0
		group := func(title, note string, labelW float32, bars []tuneBar) {
			if len(bars) == 0 {
				return
			}
			fastest := 0.0
			for _, b := range bars {
				fastest = max(fastest, b.speed)
			}
			ui.Column(c.Key(title)).Gap(5).Padding(6, 0, 0).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
					ui.Text(c, title).Font(fontMono).FontSize(11).FontWeight(500).TextColor(p.Faint)
					if note != "" {
						ui.Text(c, note).FontSize(11).TextColor(p.Faint).SingleLine().Shrink(1)
					}
				})
				for _, b := range bars {
					a.tuneBarRow(c, t, p, b, fastest, labelW, row, r.At)
					row++
				}
			})
		}

		var hosts []tuneBar
		for _, s := range r.Hosts {
			hosts = append(hosts, tuneBar{label: s.Host, speed: s.Speed, failed: s.Errors > 0,
				chosen: len(cfg.Hosts) > 0 && s.Host == cfg.Hosts[0]})
		}
		group("下载线路", "", 168, hosts)
		group("每个文件的连接数", "", 24, counts(r.Threads, int64(cfg.Threads)))
		var blocks []tuneBar
		for _, s := range r.Blocks {
			blocks = append(blocks, tuneBar{label: humanSize(s.Value), speed: s.Speed, failed: s.Errors > 0, chosen: s.Value == cfg.Block})
		}
		group("请求块大小", "", 52, blocks)
		group("同时传输的文件数", "总速度", 24, counts(r.Files, int64(cfg.Parallel)))
		note := ""
		if r.UploadErr != "" {
			note = r.UploadErr
		}
		group("每个上传的连接数", note, 24, counts(r.Upload, int64(cfg.Upload)))
		ui.Text(c, "播放与预览预读 "+itoa(cfg.Ahead)+" 个连接").Font(fontMono).FontSize(11).TextColor(p.Faint).Padding(4, 0, 0)
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

// tuneBarRow draws one value's speed as cells filling in, after those
// above it.
func (a *App) tuneBarRow(c *ui.Context, t *ui.Theme, p *palette, b tuneBar, fastest float64, labelW float32, i int, at time.Time) {
	ui.Row(c.Key(b.label)).Gap(12).AlignItems(ui.Center).Font(fontMono).FontSize(12).FontFeatures("tnum").
		TextColor(map[bool]ui.Color{true: t.Text, false: p.Faint}[b.chosen]).Children(func() {
		ui.Text(c, b.label).Width(labelW).TextAlign(ui.End).SingleLine()
		frac := float32(0)
		if fastest > 0 {
			frac = float32(b.speed / fastest)
		}
		fill, empty := p.Faint.Alpha(0.6), p.Hairline
		if b.chosen {
			fill = t.Accent
		}
		bar := ui.Box(c).Grow(1).Height(10)
		shown := bar.Keyframes("grow", []ui.Keyframe{{At: 0, Value: 0}, {At: 360 * time.Millisecond, Value: frac, Ease: easeStandard}},
			ui.KeyframeOptions{Delay: time.Duration(i) * 30 * time.Millisecond, Run: uint64(at.UnixNano())})
		bar.Draw(func(pt *ui.Painter, r ui.Rect) {
			const w, gap = 5, 2
			n := int((r.W + gap) / (w + gap))
			on := int(shown*float32(n) + 0.5)
			for k := range n {
				color := empty
				if k < on {
					color = fill
				}
				pt.Fill(ui.Rect{X: r.X + float32(k)*(w+gap), Y: r.Y, W: w, H: r.H}, color, 1)
			}
		})
		speed := humanSpeed(b.speed)
		if b.failed {
			speed += "!"
		}
		ui.Text(c, speed).Width(80).TextAlign(ui.End)
	})
}

// useStreaming lets previews and players follow the transfer settings.
func (a *App) useStreaming(cfg tune.Config) {
	remote.Parallel = min(max(cfg.Ahead, 2), 8)
	if a.stream != nil {
		a.stream.Ahead = min(max(cfg.Ahead, 2), 8)
	}
}
