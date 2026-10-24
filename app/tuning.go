package app

import (
	"context"
	"strconv"
	"strings"
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
		settingRow(c, t, p, "下载位置", "", tildePath(s.downloadDir()), func() {
			if pill(c, t, p, "", "更改…", false).Clicked() {
				a.pickDownloadDir()
			}
		})
		auto := !s.Manual
		settingRow(c, t, p, "自动优化传输", "", "", func() {
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
		// Shown either way: the host and block size stay measured when
		// the counts are chosen by hand.
		a.tuneResult(c, t, p, cfg)
		// Chosen by hand only: what auto-tuning chose shows in the
		// measurement's details.
		threads, parallel, upload := cfg.Threads, cfg.Parallel, cfg.Upload
		count := func(name, detail string, v *int, presets []int) {
			settingRow(c, t, p, name, "", detail, func() {
				if numberChoice(c, t, p, v, presets, "", name) {
					s.Threads, s.Parallel, s.Upload = threads, parallel, upload
					a.applySettings()
				}
			})
		}
		threadsDetail := ""
		if a.vip() == 0 {
			threadsDetail = "非会员建议 1–4"
		}
		if !auto {
			count("每个文件的连接数", threadsDetail, &threads, []int{1, 2, 4, 8, 16, 32, 64})
			count("同时传输的文件数", "", &parallel, []int{1, 2, 3, 4, 5, 6})
			count("每个上传的连接数", "", &upload, []int{1, 2, 4, 8, 16, 32})
		}
		limit := func(name string, mbps *int) {
			settingRow(c, t, p, name, "MB/s", "", func() {
				if numberChoice(c, t, p, mbps, []int{0, 1, 2, 5, 10, 20, 50, 100}, "不限速", name) {
					a.applySettings()
				}
			})
		}
		limit("下载限速", &s.DownLimit)
		limit("上传限速", &s.UpLimit)
	})
}

// tuneBar is one measured value of a setting.
type tuneBar struct {
	label  string
	tip    string // the whole value, when label is short of it
	speed  float64
	chosen bool
	failed bool // refused at times
}

// tuneCard is one setting in the measurement's details: the value chosen,
// how fast it went and a column for each value tried.
type tuneCard struct {
	title, value, unit string
	note               string // what failed, if anything
	speed              float64
	bars               []tuneBar
}

// tuneResult shows the last measurement and, unfolded, a card for each
// setting with what was chosen.
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
		settingRow(c, t, p, "实测速度", "", detail, func() {
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
		cards := tuneCards(r, cfg)
		ui.Column(c).Gap(10).Padding(2, 0, 12).Transition(rowEnter).Children(func() {
			for i := 0; i < len(cards); i += 2 {
				ui.Row(c.Key(i)).Gap(10).AlignItems(ui.Stretch).Children(func() {
					for j := i; j < i+2; j++ {
						if j < len(cards) {
							a.tuneCardView(c, t, p, cards[j], j, r.At)
						} else {
							ui.Box(c.Key(j)).Grow(1).Basis(0)
						}
					}
				})
			}
		})
	})
}

// tuneCards turns a measurement into its cards, a note only on those
// where something failed.
func tuneCards(r *tune.Result, cfg tune.Config) []tuneCard {
	var cards []tuneCard

	var hosts []tuneBar
	host := ""
	if len(cfg.Hosts) > 0 {
		host = cfg.Hosts[0]
	}
	for _, s := range r.Hosts {
		hosts = append(hosts, tuneBar{label: shortHost(s.Host), tip: s.Host, speed: s.Speed, failed: s.Errors > 0, chosen: s.Host == host})
	}
	if len(hosts) > 0 {
		cards = append(cards, tuneCard{title: "下载线路", value: shortHost(host), bars: hosts})
	}

	if threads := counts(r.Threads, int64(cfg.Threads)); len(threads) > 0 {
		cards = append(cards, tuneCard{title: "每个文件的连接", value: itoa(cfg.Threads), unit: "个", bars: threads})
	}

	var blocks []tuneBar
	for _, s := range r.Blocks {
		blocks = append(blocks, tuneBar{label: humanSize(s.Value), speed: s.Speed, failed: s.Errors > 0, chosen: s.Value == cfg.Block})
	}
	if len(blocks) > 0 {
		size := strings.Fields(humanSize(cfg.Block))
		card := tuneCard{title: "每次请求的块", value: size[0], bars: blocks}
		if len(size) > 1 {
			card.unit = size[1]
		}
		cards = append(cards, card)
	}

	if files := counts(r.Files, int64(cfg.Parallel)); len(files) > 0 {
		cards = append(cards, tuneCard{title: "同时下载的文件", value: itoa(cfg.Parallel), unit: "个", bars: files})
	}

	upload := counts(r.Upload, int64(cfg.Upload))
	if len(upload) > 0 || r.UploadErr != "" {
		card := tuneCard{title: "每个上传的连接", value: itoa(cfg.Upload), unit: "个", bars: upload}
		if r.UploadErr != "" {
			card.note = "上传没测成：" + r.UploadErr
		}
		cards = append(cards, card)
	}

	cards = append(cards, tuneCard{title: "播放与预览", value: itoa(cfg.Ahead), unit: "个连接"})
	for i := range cards {
		cards[i].speed = speedOfBars(cards[i].bars)
		for _, b := range cards[i].bars {
			if b.failed && cards[i].note == "" {
				cards[i].note = "有的被拒绝"
			}
		}
	}
	return cards
}

// counts makes bars of samples whose values are counts.
func counts(samples []tune.Sample, chosen int64) []tuneBar {
	var bars []tuneBar
	for _, s := range samples {
		bars = append(bars, tuneBar{label: strconv.FormatInt(s.Value, 10), speed: s.Speed, failed: s.Errors > 0, chosen: s.Value == chosen})
	}
	return bars
}

// speedOfBars returns the speed of the chosen bar.
func speedOfBars(bars []tuneBar) float64 {
	for _, b := range bars {
		if b.chosen {
			return b.speed
		}
	}
	return 0
}

// spread returns the slowest and fastest speeds of bars.
func spread(bars []tuneBar) (lo, hi float64) {
	for i, b := range bars {
		if i == 0 || b.speed < lo {
			lo = b.speed
		}
		hi = max(hi, b.speed)
	}
	return lo, hi
}

// shortHost names a download host without the netdisk's domain, as
// allall06 for allall06.baidupcs.com.
func shortHost(h string) string {
	if i := strings.Index(h, ".baidupcs.com"); i > 0 {
		return h[:i]
	}
	return h
}

// tuneCardView draws one card: the setting, the value chosen large with
// its speed, and a column for each value tried growing in after those of
// the cards before it, the chosen one in the accent color.
func (a *App) tuneCardView(c *ui.Context, t *ui.Theme, p *palette, k tuneCard, i int, at time.Time) {
	ui.Column(c.Key(k.title)).Grow(1).Basis(0).Shrink(1).Gap(10).Padding(12, 14).Radius(10).Background(p.Card).Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, k.title).FontSize(11).FontWeight(500).TextColor(t.TextMuted).SingleLine()
			ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
				ui.Row(c).Gap(3).AlignItems(ui.End).Shrink(1).Children(func() {
					figure(c, k.value).FontSize(22).FontWeight(600).Shrink(1)
					if k.unit != "" {
						ui.Text(c, k.unit).FontSize(12).TextColor(t.TextMuted).SingleLine().Padding(0, 0, 3)
					}
				})
				ui.Box(c).Grow(1)
				if k.speed > 0 {
					figure(c, humanSpeed(k.speed)).FontSize(12).FontWeight(600).TextColor(t.Accent).Padding(0, 0, 3)
				}
			})
		})
		if len(k.bars) > 0 {
			a.tuneColumns(c, t, p, k.bars, i, at)
		} else {
			// A card with nothing measured of its own shows what it is for.
			ui.Box(c).Grow(1).MinHeight(48).Center().Children(func() {
				ui.Box(c).Size(44, 44).Radius(22).Center().Background(t.Accent.Alpha(0.12)).Children(func() {
					ui.Icon(c, icon("play")).FontSize(20).TextColor(t.Accent)
				})
			})
		}
		if k.note != "" {
			ui.Text(c, k.note).FontSize(11).TextColor(t.Warning).SingleLine()
		}
	})
}

// tuneColumns draws the speed of each value tried as a column over its
// label, all as tall as the fastest allows.
func (a *App) tuneColumns(c *ui.Context, t *ui.Theme, p *palette, bars []tuneBar, card int, at time.Time) {
	_, fastest := spread(bars)
	const gap = 8
	ui.Column(c).Gap(5).Children(func() {
		chart := ui.Box(c).Height(48)
		grown := make([]float32, len(bars))
		for j, b := range bars {
			frac := float32(0)
			if fastest > 0 {
				frac = float32(b.speed / fastest)
			}
			grown[j] = chart.Keyframes("grow"+itoa(j), []ui.Keyframe{{At: 0, Value: 0}, {At: 420 * time.Millisecond, Value: frac, Ease: easeStandard}},
				ui.KeyframeOptions{Delay: time.Duration(card*60+j*40) * time.Millisecond, Run: uint64(at.UnixNano())})
		}
		rest := p.Faint.Alpha(0.3)
		chart.Draw(func(pt *ui.Painter, r ui.Rect) {
			n := float32(len(bars))
			slot := (r.W - gap*(n-1)) / n
			w := min(slot, 22)
			pt.Fill(ui.Rect{X: r.X, Y: r.Y + r.H - 1, W: r.W, H: 1}, p.Hairline, 0)
			for j, b := range bars {
				x := r.X + float32(j)*(slot+gap) + (slot-w)/2
				h := (r.H - 2) * grown[j]
				if h <= 0 {
					continue
				}
				h = max(h, 4)
				fill := rest
				if b.chosen {
					fill = t.Accent
				} else if b.failed {
					fill = t.Warning.Alpha(0.55)
				}
				pt.Fill(ui.Rect{X: x, Y: r.Y + r.H - 2 - h, W: w, H: h}, fill, min(4, w/2))
			}
		})
		ui.Row(c).Gap(gap).Children(func() {
			for j, b := range bars {
				l := ui.Column(c.Key(j)).Grow(1).Basis(0).Shrink(1).AlignItems(ui.Center).Gap(1)
				if b.tip != "" {
					l.Tooltip(b.tip)
				}
				l.Children(func() {
					label := ui.Text(c, b.label).FontSize(11).FontFeatures("tnum").SingleLine().Shrink(1)
					speed := figure(c, humanSpeed(b.speed)).FontSize(10).TextColor(p.Faint)
					if b.chosen {
						label.FontWeight(600).TextColor(t.Text)
						speed.TextColor(t.TextMuted)
					} else {
						label.TextColor(t.TextMuted)
					}
				})
			}
		})
	})
}

// useStreaming lets previews and players follow the transfer settings.
func (a *App) useStreaming(cfg tune.Config) {
	remote.Parallel = min(max(cfg.Ahead, 2), 8)
	if a.stream != nil {
		a.stream.Ahead = min(max(cfg.Ahead, 2), 8)
	}
}
