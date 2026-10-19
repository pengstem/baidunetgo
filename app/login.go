package app

import (
	"context"
	"math"
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// loginState is the sign-in screen.
type loginState struct {
	tab int // 0 QR code, 1 cookie

	qr        *ui.Bitmap
	qrState   baidu.QRState
	qrErr     string
	qrLoading bool
	qrGen     int
	qrCancel  context.CancelFunc

	cookie string
	busy   bool
	err    string
}

func (l *loginState) stopQR() {
	if l.qrCancel != nil {
		l.qrCancel()
		l.qrCancel = nil
	}
	l.qrGen++
}

// startQR asks for a QR code and follows it until it is confirmed or
// expires.
func (a *App) startQR() {
	l := &a.login
	l.stopQR()
	gen := l.qrGen
	ctx, cancel := context.WithCancel(context.Background())
	l.qrCancel = cancel
	l.qrLoading, l.qrErr, l.qr, l.qrState = true, "", nil, baidu.QRWaiting
	current := func() bool { return a.login.qrGen == gen && a.phase == phaseLogin }

	go func() {
		q, err := baidu.NewQRLogin(ctx)
		if ctx.Err() != nil {
			return
		}
		a.update(func() {
			if !current() {
				return
			}
			a.login.qrLoading = false
			if err != nil {
				a.login.qrErr = err.Error()
				return
			}
			if bmp, err := ui.DecodeBitmap(q.Image); err == nil {
				a.login.qr = bmp.Pixelated()
			} else {
				a.login.qrErr = "二维码无法显示"
			}
		})
		if err != nil {
			return
		}
		for {
			st, err := q.Poll(ctx)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				time.Sleep(2 * time.Second)
				continue
			}
			a.update(func() {
				if current() {
					a.login.qrState = st
				}
			})
			switch st {
			case baidu.QRExpired:
				return
			case baidu.QRConfirmed:
				cr, err := q.Finish(ctx)
				if ctx.Err() != nil {
					return
				}
				if err != nil {
					a.update(func() {
						if current() {
							a.login.qrErr = err.Error()
							a.login.qrState = baidu.QRExpired
						}
					})
					return
				}
				a.update(func() { a.login.busy = true })
				a.signIn(cr, true)
				return
			}
		}
	}()
}

// loginView is the sign-in screen.
func (a *App) loginView(c *ui.Context, t *ui.Theme, p *palette) {
	l := &a.login
	if l.tab == 0 && l.qr == nil && !l.qrLoading && l.qrErr == "" {
		a.startQR()
	}
	if l.tab != 0 && l.qrCancel != nil {
		l.stopQR()
		l.qr, l.qrLoading = nil, false
	}

	bar := c.TitleBar()
	root := ui.Box(c).Fill().Background(t.Background).Clip()
	root.Children(func() {
		a.aurora(c, t)
		// The top drags the window.
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(max(bar.Height, titleBarMin)).DragWindow()

		ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Center().Children(func() {
			card := ui.Column(c).Key("login-card").Width(392).Padding(34, 34, 28).Gap(22).Radius(22).
				Material(glass.Glass{}).
				Shadow(0, 24, 60, -12, p.Shadow).Shadow(0, 2, 6, 0, p.Shadow.Alpha(0.06)).
				AlignItems(ui.Center).Transition(ui.ElementTransition{
				Animation: &springPanel, Size: true,
				Enter: &ui.Motion{Scale: 0.92, Y: 16},
				Exit:  &ui.Motion{Scale: 1.04},
			})
			card.Children(func() {
				ui.Column(c).Gap(6).AlignItems(ui.Center).Children(func() {
					logo := ui.Box(c).Size(52, 52).Radius(15).Center().Margin(0, 0, 8).
						LinearGradient(ui.LinearGradient{
							Stops: []ui.GradientStop{{Color: ui.Oklch(0.7, 0.16, 245), At: 0}, {Color: ui.Oklch(0.56, 0.22, 288), At: 1}},
							Angle: 135, Oklab: true,
						}).Shadow(0, 8, 20, -6, ui.Oklch(0.55, 0.2, 270).Alpha(0.6))
					logo.Children(func() {
						ui.Icon(c, icon("hard-drive")).FontSize(26).TextColor(ui.Hex("#ffffff"))
					})
					ui.Text(c, "登录百度网盘").FontSize(21).FontWeight(650)
					ui.Text(c, "你的文件，优雅地呈现").TextColor(t.TextMuted)
				})
				a.loginTabs(c, t, p)
				ui.Box(c).Width(324).Children(func() {
					if l.tab == 0 {
						ui.Column(c).Key("qr").AlignItems(ui.Center).Gap(14).Transition(popIn).Children(func() {
							a.qrPanel(c, t, p)
						})
					} else {
						ui.Column(c).Key("cookie").Gap(12).Transition(popIn).Children(func() {
							a.cookiePanel(c, t, p)
						})
					}
				})
			})
		})
	})
}

// aurora draws soft colored lights drifting behind the sign-in card.
func (a *App) aurora(c *ui.Context, t *ui.Theme) {
	alpha := float32(0.55)
	if t.Dark {
		alpha = 0.35
	}
	blobs := []struct {
		color      ui.Color
		x, y, size float32
		period     time.Duration
	}{
		{ui.Oklch(0.72, 0.15, 250), -0.22, -0.18, 520, 23 * time.Second},
		{ui.Oklch(0.7, 0.17, 300), 0.25, 0.22, 460, 29 * time.Second},
		{ui.Oklch(0.8, 0.12, 190), 0.3, -0.25, 380, 19 * time.Second},
	}
	w, h := c.Size()
	for i, b := range blobs {
		blob := ui.Box(c).Absolute().Size(b.size, b.size).Radius(b.size / 2).
			Background(b.color.Alpha(alpha)).Blur(90).PassThrough()
		phase := blob.Loop("drift", b.period, ui.Linear) * 2 * math.Pi
		dx := float32(math.Sin(float64(phase)+float64(i))) * 60
		dy := float32(math.Cos(float64(phase)*0.8+float64(i))) * 40
		blob.Left(w/2 + b.x*w - b.size/2 + dx).Top(h/2 + b.y*h - b.size/2 + dy)
	}
}

func (a *App) loginTabs(c *ui.Context, t *ui.Theme, p *palette) {
	l := &a.login
	labels := []struct{ icon, text string }{{"qr-code", "扫码登录"}, {"cookie", "Cookie 登录"}}
	seg := ui.SegmentedBase(c, &l.tab, len(labels))
	seg.Track.Width(324).Height(38).Padding(3).Radius(11).Background(t.Surface).Label("登录方式")
	// The thumb slides under the segment chosen, painted below the labels.
	x := seg.Track.AnimateTo("thumb", float32(l.tab), springSoft)
	card, shadow := p.Card, p.Shadow
	if t.Dark {
		card = t.SurfacePressed
	}
	seg.Track.Draw(func(pt *ui.Painter, r ui.Rect) {
		w := (r.W - 6) / float32(len(labels))
		th := ui.Rect{X: r.X + 3 + x*w, Y: r.Y + 3, W: w, H: r.H - 6}
		pt.Shadow(th, 8, 0, 1, 3, 0, shadow)
		pt.Fill(th, card, 8)
	})
	seg.Track.Children(func() {
		for i, lb := range labels {
			s := seg.Segment(i).Grow(1).Basis(0).Center().Gap(7).Label(lb.text)
			color := t.TextMuted
			if i == l.tab {
				color = t.Text
			}
			s.TextColor(s.AnimateColor("fg", color, fadeQuick))
			s.Children(func() {
				ui.Icon(c, icon(lb.icon)).FontSize(15)
				ui.Text(c, lb.text).FontWeight(560).SingleLine()
			})
		}
	})
}

func (a *App) qrPanel(c *ui.Context, t *ui.Theme, p *palette) {
	l := &a.login
	frame := ui.Box(c).Size(212, 212).Radius(18).Padding(12).Background(ui.Hex("#ffffff")).
		Border(1, p.Hairline).Shadow(0, 8, 24, -10, p.Shadow).Clip()
	frame.Children(func() {
		switch {
		case l.qr != nil:
			img := ui.Image(c, l.qr).Fill().Fit(ui.Contain)
			veiled := l.qrState == baidu.QRScanned || l.qrState == baidu.QRExpired || l.busy
			img.Opacity(img.AnimateTo("veil", map[bool]float32{true: 0.12, false: 1}[veiled], fadeQuick))
			img.Transition(ui.ElementTransition{Enter: &ui.Motion{Scale: 0.9}, Animation: &springPop})
		case l.qrErr == "":
			ui.Box(c).Fill().Center().Children(func() { ui.Spinner(c).Size(28, 28).Label("加载二维码") })
		}
		a.qrOverlay(c, t, p)
	})

	hint := "打开百度网盘 App，扫一扫登录"
	switch {
	case l.busy:
		hint = "正在登录…"
	case l.qrErr != "":
		hint = l.qrErr
	case l.qrState == baidu.QRScanned:
		hint = "已扫描，请在手机上确认"
	case l.qrState == baidu.QRExpired:
		hint = "二维码已过期"
	}
	ui.Text(c, hint).TextColor(t.TextMuted).TextAlign(ui.Center).
		ContentTransition(ui.ContentTransition{})
	if l.err != "" {
		ui.Text(c, l.err).TextColor(t.Danger).TextAlign(ui.Center)
	}
}

// qrOverlay shows what happened to the code over it.
func (a *App) qrOverlay(c *ui.Context, t *ui.Theme, p *palette) {
	l := &a.login
	var ic, label string
	color := t.Success
	switch {
	case l.busy:
		ic, label = "loader-circle", "登录中"
	case l.qrErr != "" || l.qrState == baidu.QRExpired:
		ic, label, color = "refresh-cw", "刷新二维码", t.Accent
	case l.qrState == baidu.QRScanned:
		ic, label = "circle-check", "已扫描"
	default:
		return
	}
	ui.Column(c).Key(label).Absolute().Top(0).Left(0).Right(0).Bottom(0).Center().Gap(8).
		Transition(popIn).Children(func() {
		if label == "刷新二维码" {
			btn := ui.ButtonBase(c).Label(label).Padding(8, 14).Radius(999).Gap(6).
				Background(t.Accent).TextColor(t.AccentText).Shadow(0, 6, 16, -6, t.Accent)
			if btn.Hovered() {
				btn.AnimatePose(ui.Pose{Scale: 1.04}, springPop)
			} else {
				btn.AnimatePose(ui.Pose{}, springPop)
			}
			btn.Children(func() {
				ui.Icon(c, icon(ic)).FontSize(15)
				ui.Text(c, label).FontWeight(600)
			})
			if btn.Clicked() {
				a.startQR()
			}
			return
		}
		badge := ui.Box(c).Size(52, 52).Radius(26).Center().Background(color).TextColor(ui.Hex("#ffffff")).
			Shadow(0, 8, 20, -6, color)
		badge.Children(func() {
			i := ui.Icon(c, icon(ic)).FontSize(26)
			if ic == "loader-circle" {
				i.Rotate(i.Loop("spin", 900*time.Millisecond, ui.Linear) * 360)
			}
		})
		ui.Text(c, label).FontWeight(600).TextColor(ui.Hex("#16171b"))
	})
}

func (a *App) cookiePanel(c *ui.Context, t *ui.Theme, p *palette) {
	l := &a.login
	ui.Text(c, "粘贴浏览器中 pan.baidu.com 的 Cookie，或 BDUSS 与 STOKEN").TextColor(t.TextMuted).FontSize(t.Rem(0.92))
	in := ui.TextArea(c, &l.cookie).Placeholder("BDUSS=…; STOKEN=…").Lines(4, 6).Font("monospace").
		FontSize(12).Label("Cookie")
	if l.err != "" {
		in.Error(l.err)
		ui.Text(c, l.err).TextColor(t.Danger).Transition(rowEnter)
	}
	btn := ui.PrimaryButton(c, "").Height(40).Radius(11).Disabled(l.busy || l.cookie == "")
	btn.Children(func() {
		if l.busy {
			ui.Spinner(c).TextColor(t.AccentText)
			ui.Text(c, "正在登录…").FontWeight(600)
		} else {
			ui.Text(c, "登录").FontWeight(600)
		}
	})
	if btn.Clicked() {
		cr, err := baidu.ParseCredentials(l.cookie)
		if err != nil {
			l.err = err.Error()
			return
		}
		l.err, l.busy = "", true
		go a.signIn(cr, true)
	}
	ui.Text(c, "凭据只加密保存在本机，密钥存放于系统钥匙串。").TextColor(t.TextMuted).FontSize(t.Rem(0.82))
}
