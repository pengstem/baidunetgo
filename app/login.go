package app

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/pcsimport"
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

	pcs        *pcsimport.Account // BaiduPCS-Go's account, if any
	pcsChecked bool
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

// loginView is the sign-in screen: the QR code, and links to the other
// ways in. It shows only without a saved session or BaiduPCS-Go's account,
// or after signing out.
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
		// The top drags the window.
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(max(bar.Height, titleBarMin)).DragWindow()

		ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Center().Children(func() {
			ui.Column(c).Key("login").Width(324).Gap(20).AlignItems(ui.Center).Transition(popIn).Children(func() {
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					logoMark(c, t, p, 22)
					ui.Text(c, "登录百度网盘").FontSize(17).FontWeight(600)
				})
				if l.tab == 0 {
					ui.Column(c).Key("qr").AlignItems(ui.Center).Gap(14).Transition(popIn).Children(func() {
						a.qrPanel(c, t, p)
					})
				} else {
					ui.Column(c).Key("cookie").Width(324).Gap(12).Transition(popIn).Children(func() {
						a.cookiePanel(c, t, p)
					})
				}
				a.loginLinks(c, t, p)
			})
		})
	})
}

// loginLinks are the other ways in, as quiet links on one line.
func (a *App) loginLinks(c *ui.Context, t *ui.Theme, p *palette) {
	l := &a.login
	if !l.pcsChecked && !a.fake {
		l.pcsChecked = true
		l.pcs, _ = pcsimport.Find()
	}
	link := func(label string) bool {
		b := ui.ButtonBase(c).Label(label).Height(24).PaddingX(4).Radius(3).TextColor(t.Accent).Disabled(l.busy)
		if b.Hovered() {
			b.TextColor(t.AccentHover)
		}
		b.Children(func() { ui.Text(c, label).FontSize(12) })
		return b.Clicked()
	}
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		if l.pcs != nil {
			if link("用 BaiduPCS-Go 的账号（" + l.pcs.Name + "）") {
				l.busy, l.err = true, ""
				cr := l.pcs.Creds
				go a.signIn(cr, true)
			}
			ui.Text(c, "|").TextColor(p.Hairline)
		}
		other := []string{"Cookie 登录", "扫码登录"}[l.tab]
		if link(other) {
			l.tab = 1 - l.tab
			l.err = ""
		}
	})
}

func (a *App) qrPanel(c *ui.Context, t *ui.Theme, p *palette) {
	l := &a.login
	frame := ui.Box(c).Size(184, 184).Radius(5).Padding(10).Background(ui.Hex("#ffffff")).Clip()
	frame.Children(func() {
		switch {
		case l.qr != nil:
			img := ui.Image(c, l.qr).Fill().Fit(ui.Contain)
			veiled := l.qrState == baidu.QRScanned || l.qrState == baidu.QRExpired || l.busy
			img.Opacity(img.AnimateTo("veil", map[bool]float32{true: 0.12, false: 1}[veiled], motionHover))
			img.Transition(ui.ElementTransition{Enter: &ui.Motion{Opacity: 0}, Animation: &motionFade})
		case l.qrErr == "":
			ui.Box(c).Fill().Center().Children(func() { ui.Spinner(c).Size(24, 24).Label("加载二维码").TextColor(ui.Hex("#5e5e58")) })
		}
		a.qrOverlay(c, t, p)
	})

	hint := "百度网盘 App 扫码"
	switch {
	case l.busy:
		hint = "正在登录…"
	case l.qrErr != "":
		hint = l.qrErr
	case l.qrState == baidu.QRScanned:
		hint = "已扫描，在手机上确认"
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
	color := lightTheme.Success
	switch {
	case l.busy:
		ic, label = "loader-circle", "登录中"
	case l.qrErr != "" || l.qrState == baidu.QRExpired:
		ic, label = "refresh-cw", "刷新二维码"
	case l.qrState == baidu.QRScanned:
		ic, label = "circle-check", "已扫描"
	default:
		return
	}
	ui.Column(c).Key(label).Absolute().Top(0).Left(0).Right(0).Bottom(0).Center().Gap(8).
		Transition(popIn).Children(func() {
		if label == "刷新二维码" {
			btn := ui.ButtonBase(c).Label(label).Height(30).PaddingX(12).Radius(5).Gap(6).AlignItems(ui.Center).
				Background(lightTheme.Accent).TextColor(lightTheme.AccentText)
			if btn.Hovered() {
				btn.Background(lightTheme.AccentHover)
			}
			btn.Children(func() {
				ui.Icon(c, icon(ic)).FontSize(14)
				ui.Text(c, label).FontWeight(500)
			})
			if btn.Clicked() {
				a.startQR()
			}
			return
		}
		i := ui.Icon(c, icon(ic)).FontSize(28).TextColor(color)
		if ic == "loader-circle" {
			i.Rotate(i.Loop("spin", 900*time.Millisecond, ui.Linear) * 360)
		}
		ui.Text(c, label).FontWeight(500).TextColor(lightTheme.Text)
	})
}

func (a *App) cookiePanel(c *ui.Context, t *ui.Theme, p *palette) {
	l := &a.login
	ui.Text(c, "粘贴 pan.baidu.com 的 Cookie，或 BDUSS 与 STOKEN").TextColor(t.TextMuted)
	in := ui.TextArea(c, &l.cookie).Placeholder("BDUSS=…; STOKEN=…").Lines(4, 6).Font(fontMono).
		FontSize(12).Label("Cookie")
	if l.err != "" {
		in.Error(l.err)
		ui.Text(c, l.err).TextColor(t.Danger).Transition(rowEnter)
	}
	btn := pill(c, t, p, "", "登录", true).Disabled(l.busy || l.cookie == "")
	if l.busy {
		btn.Children(func() { ui.Spinner(c).TextColor(t.AccentText) })
	}
	if btn.Clicked() {
		cr, err := baidu.ParseCredentials(l.cookie)
		if err != nil {
			l.err = err.Error()
			return
		}
		l.err, l.busy = "", true
		go a.signIn(cr, true)
	}
	ui.Text(c, "加密保存在本机，密钥在系统钥匙串").TextColor(p.Faint).FontSize(12)
}
