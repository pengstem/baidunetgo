package app

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/remote"
	"github.com/pengstem/baidunetgo/internal/tune"
)

// tuneMaxAge is how long a measurement lasts before it is made again.
const tuneMaxAge = 7 * 24 * time.Hour

// vip returns the membership of the signed-in account.
func (a *App) vip() int {
	if a.sess != nil && a.sess.user != nil {
		return a.sess.user.VIP
	}
	return 0
}

// transferConfig returns the transfer settings in force: the user's, or
// those the membership and the last measurement call for.
func (a *App) transferConfig() tune.Config {
	s := a.settings
	if s.Manual {
		return tune.Config{Threads: s.threads(), Parallel: s.parallel()}
	}
	return tune.Recommend(a.vip(), a.measured())
}

// measured returns the last measurement, if it was made with the
// membership the account has now.
func (a *App) measured() *tune.Result {
	if t := a.settings.Tune; t != nil && t.VIP == a.vip() {
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

// runTune measures download speeds with a few numbers of connections, on
// a large file of the account read into nowhere, and keeps the result.
func (a *App) runTune(announce bool) {
	s := a.sess
	if s == nil || a.tuning {
		return
	}
	a.tuning, a.tuneErr = true, ""
	vip := a.vip()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		f, err := tune.FindFile(ctx, s.client, 128<<20)
		if err != nil {
			f, err = tune.FindFile(ctx, s.client, 24<<20)
		}
		var res *tune.Result
		if err == nil {
			res, err = tune.Probe(ctx, s.client, f, tune.Candidates(vip), 5*time.Second, 400<<20)
		}
		a.update(func() {
			a.tuning = false
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
			a.toast("已根据测速优化：每个文件 "+itoa(cfg.Threads)+" 个连接，约 "+humanSpeed(res.Speed(cfg.Threads)), "success")
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
		settingRow(c, t, p, "自动选择连接数", "", func() {
			if ui.Switch(c, &auto).Label("自动选择连接数").Changed() {
				s.Manual = !auto
				if !s.Manual {
					s.Threads, s.Parallel = 0, 0
				} else {
					s.Threads, s.Parallel = cfg.Threads, cfg.Parallel
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
		threads, parallel := float64(cfg.Threads), float64(cfg.Parallel)
		detail := ""
		if !auto {
			detail = "非会员建议 1–4"
		}
		settingRow(c, t, p, "每个文件的连接数", detail, func() {
			if ui.NumberInput(c, &threads, 1, 32, 1).Font(fontMono).Label("连接数").Changed() {
				s.Manual, s.Threads, s.Parallel = true, int(threads), cfg.Parallel
				a.applySettings()
			}
		})
		settingRow(c, t, p, "同时传输的文件数", "", func() {
			if ui.NumberInput(c, &parallel, 1, 6, 1).Font(fontMono).Label("同时传输数").Changed() {
				s.Manual, s.Threads, s.Parallel = true, cfg.Threads, int(parallel)
				a.applySettings()
			}
		})
	})
}

// tuneResult shows the last measurement as rows of cells, the chosen
// count in the accent color.
func (a *App) tuneResult(c *ui.Context, t *ui.Theme, p *palette, cfg tune.Config) {
	r := a.measured()
	detail := "还没有测速"
	switch {
	case a.tuning:
		detail = "测速中…"
	case a.tuneErr != "":
		detail = a.tuneErr
	case r != nil:
		detail = humanTime(r.At.Unix(), c.Now())
	}
	ui.Column(c).Padding(0, 0, 10).Gap(6).Children(func() {
		settingRow(c, t, p, "实测速度", detail, func() {
			b := pill(c, t, p, "", "重测", false).Disabled(a.tuning)
			if a.tuning {
				b.Children(func() { ui.Spinner(c) })
			}
			if b.Clicked() {
				a.runTune(true)
			}
		})
		if r == nil || len(r.Samples) == 0 {
			return
		}
		fastest := 0.0
		for _, s := range r.Samples {
			fastest = max(fastest, s.Speed)
		}
		for i, s := range r.Samples {
			chosen := s.Threads == cfg.Threads
			ui.Row(c.Key(s.Threads)).Gap(12).AlignItems(ui.Center).Font(fontMono).FontSize(12).FontFeatures("tnum").
				TextColor(map[bool]ui.Color{true: t.Text, false: p.Faint}[chosen]).Children(func() {
				ui.Text(c, itoa(s.Threads)).Width(20).TextAlign(ui.End)
				frac := float32(0)
				if fastest > 0 {
					frac = float32(s.Speed / fastest)
				}
				fill, empty := p.Faint.Alpha(0.6), p.Hairline
				if chosen {
					fill = t.Accent
				}
				bar := ui.Box(c).Grow(1).Height(10)
				shown := bar.Keyframes("grow", []ui.Keyframe{{At: 0, Value: 0}, {At: 360 * time.Millisecond, Value: frac, Ease: easeStandard}},
					ui.KeyframeOptions{Delay: time.Duration(i) * 40 * time.Millisecond, Run: uint64(r.At.UnixNano())})
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
				ui.Text(c, humanSpeed(s.Speed)).Width(72).TextAlign(ui.End)
			})
		}
	})
}

// useStreaming lets previews and players follow the transfer settings.
func (a *App) useStreaming(cfg tune.Config) {
	remote.Parallel = min(max(cfg.Threads, 2), 8)
	if a.stream != nil {
		a.stream.Ahead = min(max(cfg.Threads/2, 2), 8)
	}
}
