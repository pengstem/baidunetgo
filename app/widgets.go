package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// iconButton is a quiet square button showing an icon, named for
// assistive technology and its tip.
func iconButton(c *ui.Context, t *ui.Theme, p *palette, ic, label string) ui.Element {
	b := ui.ButtonBase(c).Label(label).Tooltip(label).Size(32, 32).Radius(9).Center().TextColor(t.TextMuted)
	if b.IsDisabled() {
		b.Opacity(0.4)
	} else if b.Pressed() {
		b.Background(t.SurfacePressed).TextColor(t.Text)
	} else if b.Hovered() {
		b.Background(p.Hover).TextColor(t.Text)
	}
	b.Children(func() { ui.Icon(c, icon(ic)).FontSize(17) })
	return b
}

// pill is a small rounded button with an icon and a label.
func pill(c *ui.Context, t *ui.Theme, p *palette, ic, label string, primary bool) ui.Element {
	b := ui.ButtonBase(c).Label(label).Height(32).PaddingX(12).Radius(9).Gap(7).AlignItems(ui.Center)
	if primary {
		bg := t.Accent
		if b.Pressed() {
			bg = t.AccentPressed
		} else if b.Hovered() {
			bg = t.AccentHover
		}
		b.Background(b.AnimateColor("bg", bg, fadeQuick)).TextColor(t.AccentText).
			Shadow(0, 4, 12, -4, t.Accent.Alpha(0.6))
	} else {
		bg := t.Surface
		if b.Pressed() {
			bg = t.SurfacePressed
		} else if b.Hovered() {
			bg = t.SurfaceHover
		}
		b.Background(b.AnimateColor("bg", bg, fadeQuick))
	}
	b.Children(func() {
		if ic != "" {
			ui.Icon(c, icon(ic)).FontSize(15)
		}
		ui.Text(c, label).FontWeight(560).SingleLine()
	})
	return b
}

// emptyState is a centered message with an icon, as for an empty folder.
func emptyState(c *ui.Context, t *ui.Theme, p *palette, ic, title, detail string, action func()) {
	ui.Column(c).Grow(1).Center().Gap(10).Padding(40).Transition(popIn).Children(func() {
		badge := ui.Box(c).Size(72, 72).Radius(22).Center().Background(p.AccentSoft).Margin(0, 0, 6)
		badge.Children(func() {
			i := ui.Icon(c, icon(ic)).FontSize(32).TextColor(t.Accent)
			bob := i.Loop("bob", 3200*time.Millisecond, ui.EaseInOut)
			i.Translate(0, float32(sinTurn(bob))*3)
		})
		ui.Text(c, title).FontSize(16).FontWeight(600)
		if detail != "" {
			ui.Text(c, detail).TextColor(t.TextMuted).TextAlign(ui.Center).MaxWidth(360)
		}
		if action != nil {
			if pill(c, t, p, "refresh-cw", "重试", true).Margin(8, 0, 0).Clicked() {
				action()
			}
		}
	})
}

// skeletonRows shows placeholder rows pulsing while a listing loads.
func skeletonRows(c *ui.Context, t *ui.Theme, p *palette) {
	ui.Column(c).Grow(1).Padding(6, 18).Gap(2).Children(func() {
		for i := range 9 {
			row := ui.Row(c).Height(52).PaddingX(14).Gap(12).AlignItems(ui.Center)
			pulse := row.Loop("pulse", 1400*time.Millisecond, ui.Bounce(ui.EaseInOut))
			row.Opacity(0.55 + 0.45*pulse - float32(i)*0.045)
			row.Children(func() {
				ui.Box(c).Size(34, 34).Radius(9).Background(t.Surface)
				ui.Column(c).Gap(7).Children(func() {
					ui.Box(c).Size(float32(140+(i*53)%160), 11).Radius(4).Background(t.Surface)
					ui.Box(c).Size(float32(70+(i*31)%60), 9).Radius(4).Background(t.Surface)
				})
			})
		}
	})
}
