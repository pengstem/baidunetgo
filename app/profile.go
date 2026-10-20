package app

import (
	"github.com/egoist/mygo/ui"
)

// profile is the account at the foot of the sidebar: who is signed in and
// how full the netdisk is, opening a menu to sign out.
func (a *App) profile(c *ui.Context, t *ui.Theme, p *palette) {
	s := a.sess
	if s == nil || s.user == nil {
		return
	}
	q := s.quota
	frac := float32(0)
	if q.Total > 0 {
		frac = float32(float64(q.Used) / float64(q.Total))
	}
	card := ui.ButtonBase(c).Label("账户").Column().AlignItems(ui.Stretch).Padding(8, 10).Radius(5).Gap(6).
		BorderWidth(1, 0, 0, 0).BorderColor(p.Hairline)
	if card.Hovered() || a.profileOpen {
		card.Background(p.Hover)
	}
	card.Children(func() {
		if a.sidebarCollapsed {
			figure(c, itoa(int(frac*100+0.5))+"%").FontSize(11).TextColor(p.Faint).TextAlign(ui.Center)
			return
		}
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, s.user.Name).FontWeight(500).SingleLine().Shrink(1)
			if v := s.user.VIPLabel(); v != "" {
				tag(c, v, p.VIP)
			}
		})
		if q.Total > 0 {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				track := ui.Box(c).Grow(1).Height(4).Radius(1).Background(p.Hairline).Clip()
				track.Children(func() {
					fill := ui.Box(c).Height(4).Radius(1).Background(quotaColor(t, frac))
					fill.WidthPercent(fill.AnimateTo("w", min(frac, 1)*100, motionPanel))
				})
				figure(c, shortSize(q.Used)+"/"+shortSize(q.Total)).FontSize(11).TextColor(p.Faint)
			})
		}
	})
	if card.Clicked() {
		a.profileOpen = !a.profileOpen
	}
	ui.PopoverBase(c, card, &a.profileOpen, func(panel ui.Element) {
		panel.AttachTo(card, ui.AnchorTopLeft, ui.AnchorBottomLeft).Margin(0, 0, 4).Width(180).Padding(4).
			Radius(8).Background(p.Card).Border(1, p.Hairline).Shadow(0, 8, 24, -8, p.Shadow).
			Transition(popIn)
		item := ui.ButtonBase(c).Label("退出登录").Height(30).PaddingX(10).Radius(5).Gap(8).Justify(ui.Start).
			AlignItems(ui.Center).TextColor(t.Danger)
		if item.Hovered() {
			item.Background(t.Danger.Alpha(0.1))
		}
		item.Children(func() {
			ui.Icon(c, icon("log-out")).FontSize(14)
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
	case frac > 0.9:
		return t.Danger
	case frac > 0.8:
		return t.Warning
	}
	return t.Accent
}
