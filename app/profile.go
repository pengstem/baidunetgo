package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// profile is the account at the foot of the sidebar: who is signed in and
// how full the netdisk is, opening a menu to sign out.
func (a *App) profile(c *ui.Context, t *ui.Theme, p *palette) {
	s := a.sess
	if s == nil || s.user == nil {
		return
	}
	card := ui.ButtonBase(c).Label("账户").Padding(10).Radius(12).Gap(10).Justify(ui.Start).Margin(0, 0, 6)
	if card.Hovered() || a.profileOpen {
		card.Background(p.Hover)
	}
	card.Children(func() {
		ui.Avatar(c, s.user.Name, s.avatar).Size(34, 34)
		if a.sidebarCollapsed {
			return
		}
		ui.Column(c).Grow(1).Gap(5).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, s.user.Name).FontWeight(600).SingleLine().Shrink(1)
				if v := s.user.VIPLabel(); v != "" {
					ui.Text(c, v).FontSize(10).FontWeight(700).Padding(1, 5).Radius(5).
						TextColor(ui.Hex("#5b3a00")).
						LinearGradient(ui.LinearGradient{Stops: []ui.GradientStop{{Color: ui.Hex("#fde68a"), At: 0}, {Color: ui.Hex("#f59e0b"), At: 1}}, Angle: 90})
				}
			})
			q := s.quota
			if q.Total > 0 {
				frac := float32(float64(q.Used) / float64(q.Total))
				track := ui.Box(c).Height(4).Radius(2).Background(p.Hairline).Clip()
				track.Children(func() {
					fill := ui.Box(c).Height(4).Radius(2).Background(quotaColor(t, frac))
					fill.WidthPercent(fill.AnimateTo("w", min(frac, 1)*100, springPanel))
				})
				ui.Text(c, fmt.Sprintf("%s / %s", humanSize(q.Used), humanSize(q.Total))).
					FontSize(11).TextColor(t.TextMuted).SingleLine()
			}
		})
	})
	if card.Clicked() {
		a.profileOpen = !a.profileOpen
	}
	ui.PopoverBase(c, card, &a.profileOpen, func(panel ui.Element) {
		panel.AttachTo(card, ui.AnchorTopLeft, ui.AnchorBottomLeft).Margin(0, 0, 6).Width(220).Padding(6).
			Radius(12).Background(p.Card).Border(1, p.Hairline).Shadow(0, 12, 32, -8, p.Shadow).
			Transition(popIn)
		item := ui.ButtonBase(c).Label("退出登录").Padding(9, 10).Radius(8).Gap(10).Justify(ui.Start).TextColor(t.Danger)
		if item.Hovered() {
			item.Background(t.Danger.Alpha(0.1))
		}
		item.Children(func() {
			ui.Icon(c, icon("log-out")).FontSize(16)
			ui.Text(c, "退出登录")
		})
		if item.Clicked() {
			a.profileOpen = false
			a.signOut()
		}
	})
}

func quotaColor(t *ui.Theme, frac float32) ui.Color {
	switch {
	case frac > 0.95:
		return t.Danger
	case frac > 0.8:
		return t.Warning
	}
	return t.Accent
}
