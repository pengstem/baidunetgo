package app

import (
	"github.com/egoist/mygo/ui"
)

// profile is the foot of the sidebar: who is signed in, their membership
// and how full the netdisk is, opening a menu to sign out, and the gear
// of the settings.
func (a *App) profile(c *ui.Context, t *ui.Theme, p *palette) {
	s := a.sess
	if s == nil || s.user == nil {
		return
	}
	ui.Row(c).Padding(8, 10, 12).Gap(4).AlignItems(ui.Center).Children(func() {
		card := ui.ButtonBase(c).Label("账户").Column().Grow(1).Shrink(1).AlignItems(ui.Start).Padding(4, 6).Radius(6).Gap(1)
		if card.Hovered() || a.profileOpen {
			card.Background(p.Hover)
		}
		card.Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, s.user.Name).FontSize(12).FontWeight(600).SingleLine().Shrink(1)
				if v := s.user.VIPLabel(); v != "" {
					chip(c, v, p.VIP).FontSize(9).Padding(0, 5)
				}
			})
			if q := s.quota; q.Total > 0 {
				figure(c, "已用 "+humanSize(q.Used)+" / "+humanSize(q.Total)).FontSize(11).TextColor(t.TextMuted)
			}
		})
		if card.Clicked() {
			a.profileOpen = !a.profileOpen
		}
		ui.PopoverBase(c, card, &a.profileOpen, func(panel ui.Element) {
			panel.AttachTo(card, ui.AnchorTopLeft, ui.AnchorBottomLeft).Margin(0, 0, 6).Width(196).Padding(5).
				Radius(10).Background(p.Card).Border(1, p.Hairline).Shadow(0, 10, 30, -8, p.Shadow).
				Transition(popIn)
			if q := s.quota; q.Total > 0 {
				frac := float32(float64(q.Used) / float64(q.Total))
				ui.Column(c).Padding(6, 8, 8).Gap(6).Children(func() {
					ui.Row(c).Children(func() {
						ui.Text(c, "网盘空间").FontSize(12).TextColor(t.TextMuted)
						ui.Spacer(c)
						figure(c, itoa(int(frac*100+0.5))+"%").FontSize(12).FontWeight(600)
					})
					ui.Box(c).Height(4).Radius(2).Background(p.Control).Clip().Children(func() {
						ui.Box(c).Height(4).Radius(2).Background(quotaColor(t, frac)).WidthPercent(min(frac, 1) * 100)
					})
				})
				ui.Divider(c).Margin(2, 4)
			}
			item := ui.ButtonBase(c).Label("退出登录").Height(28).PaddingX(8).Radius(6).Gap(8).Justify(ui.Start).
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
		gear := iconButton(c, t, p, "settings", "设置")
		if a.section() == "/settings" {
			gear.Background(p.Selected).TextColor(t.Text)
		}
		if gear.Clicked() && a.router.Path() != "/settings" {
			a.router.Push("/settings")
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
