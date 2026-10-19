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
		settingRow(c, t, p, "folder-open", "下载位置", s.downloadDir(), func() {
			if pill(c, t, p, "", "更改…", false).Clicked() {
				a.pickDownloadDir()
			}
		})
		auto := !s.Manual
		settingRow(c, t, p, "sparkles", "自动优化", "按会员等级和实测速度选择连接数", func() {
			if ui.Switch(c, &auto).Label("自动优化").Changed() {
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
		detail := "非会员账号建议 1–4，过多可能被限速"
		if auto {
			detail = "自动"
		}
		settingRow(c, t, p, "arrow-up-down", "每个文件的连接数", detail, func() {
			ui.Text(c, itoa(int(threads))).Width(24).TextAlign(ui.End).FontFeatures("tnum").
				ContentTransition(ui.ContentTransition{Numeric: true})
			if ui.Stepper(c, &threads, 1, 32, 1).Label("连接数").Changed() {
				s.Manual, s.Threads, s.Parallel = true, int(threads), cfg.Parallel
				a.applySettings()
			}
		})
		settingRow(c, t, p, "layout-grid", "同时传输的文件数", "", func() {
			ui.Text(c, itoa(int(parallel))).Width(24).TextAlign(ui.End).FontFeatures("tnum").
				ContentTransition(ui.ContentTransition{Numeric: true})
			if ui.Stepper(c, &parallel, 1, 6, 1).Label("同时传输数").Changed() {
				s.Manual, s.Threads, s.Parallel = true, cfg.Threads, int(parallel)
				a.applySettings()
			}
		})
	})
}

// tuneResult shows the last measurement as bars, the one chosen in the
// accent color.
func (a *App) tuneResult(c *ui.Context, t *ui.Theme, p *palette, cfg tune.Config) {
	r := a.measured()
	detail := "还没有测速"
	switch {
	case a.tuning:
		detail = "正在测速…（读取网盘中一个大文件的片段，不保存）"
	case a.tuneErr != "":
		detail = a.tuneErr
	case r != nil:
		detail = "最快约 " + humanSpeed(r.Speed(cfg.Threads)) + " · " + humanTime(r.At.Unix(), c.Now())
	}
	settingRow(c, t, p, "zoom-in", "实测速度", detail, func() {
		b := pill(c, t, p, "refresh-cw", "重新测速", false).Disabled(a.tuning)
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
	ui.Column(c).Padding(4, 16, 14, 62).Gap(7).Children(func() {
		for i, s := range r.Samples {
			chosen := s.Threads == cfg.Threads
			ui.Row(c.Key(s.Threads)).Gap(10).AlignItems(ui.Center).Children(func() {
				label := ui.Text(c, itoa(s.Threads)+" 连接").Width(52).FontSize(t.Rem(0.84)).FontFeatures("tnum").TextColor(t.TextMuted)
				if chosen {
					label.TextColor(t.Accent).FontWeight(650)
				}
				track := ui.Box(c).Grow(1).Height(8).Radius(4).Background(p.Hairline).Clip()
				track.Children(func() {
					color := t.Accent.Alpha(0.35)
					if chosen {
						color = t.Accent
					}
					bar := ui.Box(c).Height(8).Radius(4).Background(color)
					delay := time.Duration(i) * 60 * time.Millisecond
					frac := float32(0)
					if fastest > 0 {
						frac = float32(s.Speed / fastest)
					}
					bar.WidthPercent(bar.Keyframes("grow", []ui.Keyframe{{At: 0, Value: 0}, {At: 600 * time.Millisecond, Value: frac * 100, Ease: ui.CubicBezier(0.2, 0.8, 0.2, 1)}},
						ui.KeyframeOptions{Delay: delay, Run: uint64(r.At.UnixNano())}))
				})
				ui.Text(c, humanSpeed(s.Speed)).Width(84).TextAlign(ui.End).FontSize(t.Rem(0.84)).FontFeatures("tnum")
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
