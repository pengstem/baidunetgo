package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// iconButton is a quiet square button showing an icon, named for
// assistive technology and its tip.
func iconButton(c *ui.Context, t *ui.Theme, p *palette, ic, label string) ui.Element {
	b := ui.ButtonBase(c).Label(label).Tooltip(label).Size(30, 30).Radius(5).Center().TextColor(t.TextMuted)
	if b.IsDisabled() {
		b.Opacity(0.4)
	} else if b.Pressed() {
		b.Background(t.SurfacePressed).TextColor(t.Text)
	} else if b.Hovered() {
		b.Background(p.Hover).TextColor(t.Text)
	}
	b.Children(func() { ui.Icon(c, icon(ic)).FontSize(16) })
	return b
}

// pill is a button with an optional icon and a label: the main action of
// a view in the accent color, flat, or a quiet one on the surface.
func pill(c *ui.Context, t *ui.Theme, p *palette, ic, label string, primary bool) ui.Element {
	b := ui.ButtonBase(c).Label(label).Height(30).PaddingX(12).Radius(5).Gap(6).AlignItems(ui.Center)
	if primary {
		bg := t.Accent
		if b.Pressed() {
			bg = t.AccentPressed
		} else if b.Hovered() {
			bg = t.AccentHover
		}
		b.Background(b.AnimateColor("bg", bg, motionHover)).TextColor(t.AccentText)
	} else {
		bg := t.Surface
		if b.Pressed() {
			bg = t.SurfacePressed
		} else if b.Hovered() {
			bg = t.SurfaceHover
		}
		b.Background(b.AnimateColor("bg", bg, motionHover)).Border(1, p.Hairline)
	}
	b.Children(func() {
		if ic != "" {
			ui.Icon(c, icon(ic)).FontSize(14)
		}
		ui.Text(c, label).FontWeight(500).SingleLine()
	})
	return b
}

// figure is a number set in the monospaced face, its digits lined up.
func figure(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).Font(fontMono).FontFeatures("tnum").SingleLine()
}

// tag is a small outlined label, as for a membership or a state.
func tag(c *ui.Context, label string, color ui.Color) ui.Element {
	return ui.Text(c, label).Font(fontMono).FontSize(10).FontWeight(500).Padding(0, 5).Radius(3).
		Border(1, color.Alpha(0.45)).TextColor(color).SingleLine()
}

// emptyState is a centered message with an icon, as for an empty folder.
func emptyState(c *ui.Context, t *ui.Theme, p *palette, ic, title, detail string, action func()) {
	ui.Column(c).Grow(1).Center().Gap(8).Padding(40).Transition(popIn).Children(func() {
		ui.Icon(c, icon(ic)).FontSize(24).TextColor(p.Faint).Margin(0, 0, 4)
		ui.Text(c, title).FontSize(14).FontWeight(500)
		if detail != "" {
			ui.Text(c, detail).TextColor(t.TextMuted).TextAlign(ui.Center).MaxWidth(360)
		}
		if action != nil {
			if pill(c, t, p, "refresh-cw", "重试", false).Margin(8, 0, 0).Clicked() {
				action()
			}
		}
	})
}

// skeletonRows shows placeholder rows pulsing while a listing loads.
func skeletonRows(c *ui.Context, t *ui.Theme, p *palette) {
	ui.Column(c).Grow(1).Padding(6, 12).Gap(0).Children(func() {
		for i := range 12 {
			row := ui.Row(c).Height(rowHeight).PaddingX(10).Gap(10).AlignItems(ui.Center)
			pulse := row.Loop("pulse", 1400*time.Millisecond, ui.Bounce(ui.EaseInOut))
			row.Opacity(0.55 + 0.45*pulse - float32(i)*0.035)
			row.Children(func() {
				ui.Box(c).Size(18, 18).Radius(3).Background(t.Surface)
				ui.Box(c).Size(float32(140+(i*53)%160), 10).Radius(2).Background(t.Surface)
			})
		}
	})
}
