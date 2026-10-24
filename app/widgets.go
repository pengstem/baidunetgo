package app

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// iconButton is a quiet round button showing an icon, named for assistive
// technology and its tip: a gray disc shows under it as the pointer rests.
func iconButton(c *ui.Context, t *ui.Theme, p *palette, ic, label string) ui.Element {
	b := ui.ButtonBase(c).Label(label).Tooltip(label).Size(28, 28).Radius(14).Center().TextColor(t.TextMuted)
	if b.IsDisabled() {
		b.Opacity(0.35)
	} else if b.Pressed() {
		b.Background(t.SurfacePressed).TextColor(t.Text)
	} else if b.Hovered() {
		b.Background(p.Hover).TextColor(t.Text)
	}
	b.Children(func() { ui.Icon(c, icon(ic)).FontSize(15) })
	return b
}

// toolButton is a round button of the title bar, on a gray disc as
// Pulse's.
func toolButton(c *ui.Context, t *ui.Theme, p *palette, ic, label string) ui.Element {
	b := ui.ButtonBase(c).Label(label).Tooltip(label).Size(28, 28).Radius(14).Center().TextColor(t.Text)
	bg := p.Control
	if b.IsDisabled() {
		b.Opacity(0.4)
	} else if b.Pressed() {
		bg = t.SurfacePressed
	} else if b.Hovered() {
		bg = t.SurfaceHover
	}
	b.Background(b.AnimateColor("bg", bg, motionHover))
	b.Children(func() { ui.Icon(c, icon(ic)).FontSize(14) })
	return b
}

// pill is a push button with an optional icon and a label: the main action
// of a view filled with the accent color, a quiet one on the control gray.
func pill(c *ui.Context, t *ui.Theme, p *palette, ic, label string, primary bool) ui.Element {
	return pillOf(c, t, p, ic, label, primary, false)
}

// pillOf is pill, but folded to a round button of just its icon, as a
// title bar's tools fold when it is narrow.
func pillOf(c *ui.Context, t *ui.Theme, p *palette, ic, label string, primary, folded bool) ui.Element {
	b := ui.ButtonBase(c).Label(label).Height(28).PaddingX(12).Radius(7).Gap(5).AlignItems(ui.Center)
	if folded {
		b.Width(28).PaddingX(0).Radius(14).Center().Tooltip(label)
	}
	if primary {
		bg := t.Accent
		if b.Pressed() {
			bg = t.AccentPressed
		} else if b.Hovered() {
			bg = t.AccentHover
		}
		b.Background(b.AnimateColor("bg", bg, motionHover)).TextColor(t.AccentText)
	} else {
		bg := p.Control
		if b.Pressed() {
			bg = t.SurfacePressed
		} else if b.Hovered() {
			bg = t.SurfaceHover
		}
		b.Background(b.AnimateColor("bg", bg, motionHover))
	}
	if b.IsDisabled() {
		b.Opacity(0.45)
	}
	b.Children(func() {
		if ic != "" {
			ui.Icon(c, icon(ic)).FontSize(13)
		}
		if !folded {
			ui.Text(c, label).FontWeight(500).SingleLine()
		}
	})
	return b
}

// figure is a number, its digits of one width so that they line up and
// hold still as they change.
func figure(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontFeatures("tnum").SingleLine()
}

// numberField is a whole number in a capsule between a minus and a plus,
// its unit after the capsule: typed into, or stepped, and on and on while a sign is
// held, between lo and hi. Zero reads as none when that is given. It
// reports a new value.
func numberField(c *ui.Context, t *ui.Theme, p *palette, value *float64, lo, hi, step float64, unit, none, label string) bool {
	changed := false
	set := func(v float64) {
		if v = max(lo, min(hi, v)); v != *value {
			*value, changed = v, true
		}
	}
	format := func(v float64) string {
		if v == 0 && none != "" {
			return none
		}
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	var text *string
	ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
		capsule := ui.Row(c).Height(28).Radius(14).Padding(0, 2).AlignItems(ui.Center).Background(p.Control)
		text = ui.Local(capsule, "text", func() string { return format(*value) })
		capsule.Children(func() {
			sign := func(ic, name string, by float64) {
				can := by < 0 && *value > lo || by > 0 && *value < hi
				b := ui.ButtonBase(c).Label(name).Size(24, 24).Radius(12).Center().TextColor(t.Text).Disabled(!can)
				if !can {
					b.TextColor(t.TextMuted.Alpha(0.5))
				} else if b.Pressed() {
					b.Background(t.SurfacePressed)
				} else if b.Hovered() {
					b.Background(t.SurfaceHover)
				}
				b.Children(func() { ui.Icon(c, icon(ic)).FontSize(12) })
				if n := held(c, b); can && n > 0 {
					set(*value + by*float64(n))
				}
			}
			sign("minus", "减少"+label, -step)
			in := ui.TextInputBase(c, text).Label(label).Width(48).Height(24).TextAlign(ui.Center).FontFeatures("tnum").SingleLine()
			if in.Changed() {
				if v, err := strconv.ParseFloat(strings.TrimSpace(*text), 64); err == nil && v >= lo && v <= hi {
					set(v)
				}
			}
			if !in.Focused() {
				*text = format(*value)
			}
			sign("plus", "增加"+label, step)
		})
		// The unit has a place of its own width after every capsule, filled
		// or not, so that the capsules of a page line up and hold still
		// as a value turns to none.
		u := ui.Text(c, unit).Width(30).FontSize(12).TextColor(t.TextMuted).SingleLine()
		if *value == 0 && none != "" {
			u.Opacity(0)
		}
	})
	if changed {
		*text = format(*value)
	}
	return changed
}

// held is how many steps a stepping button owes this frame: one as it is
// pressed and, held, more after a while and then faster, as AppKit's.
func held(c *ui.Context, b ui.Element) int {
	type hold struct {
		start time.Time
		steps int
	}
	h := ui.Local(b, "hold", func() hold { return hold{} })
	if !b.Pressed() {
		h.start = time.Time{}
		return 0
	}
	now := c.Now()
	if h.start.IsZero() {
		*h = hold{start: now}
	}
	due := 1
	if d := now.Sub(h.start); d > 400*time.Millisecond {
		due += int((d - 400*time.Millisecond) / (80 * time.Millisecond))
	}
	c.After(80 * time.Millisecond)
	n := due - h.steps
	h.steps = due
	return n
}

// chip is a small rounded label tinted with a color, as for a membership
// or a state.
func chip(c *ui.Context, label string, color ui.Color) ui.Element {
	return ui.Text(c, label).FontSize(11).FontWeight(600).Padding(1, 7).Radius(9).
		Background(color.Alpha(0.14)).TextColor(color).SingleLine()
}

// segmented is a capsule of labels, the chosen one raised on a white thumb
// that glides to it, as the segmented controls of macOS.
func segmented(c *ui.Context, t *ui.Theme, p *palette, chosen *int, labels ...string) ui.Element {
	return segmentedOf(c, t, p, chosen, len(labels), func(i int, s ui.Element) {
		s.Label(labels[i]).PaddingX(12)
		s.Children(func() {
			ui.Text(c, labels[i]).FontSize(12).FontWeight(500).SingleLine().FontFeatures("tnum").
				ContentTransition(ui.ContentTransition{Numeric: true})
		})
	})
}

// choice is a segmented capsule of labels, folded into a pop-up button
// when compact, as Pulse's toolbars fold theirs in a narrow window.
func choice(c *ui.Context, t *ui.Theme, p *palette, chosen *int, compact bool, labels ...string) ui.Element {
	if compact {
		return popUp(c, t, p, chosen, labels, nil)
	}
	return segmented(c, t, p, chosen, labels...)
}

// popUp is a pop-up button of choices, as AppKit's: the one chosen, its
// icon when icons are given, else its label, over stacked chevrons,
// opening a menu of them all with the chosen one checked.
func popUp(c *ui.Context, t *ui.Theme, p *palette, chosen *int, labels, icons []string) ui.Element {
	b := ui.ButtonBase(c).Label(labels[*chosen]).Height(28).PaddingStart(10).PaddingEnd(7).Radius(14).Gap(4).AlignItems(ui.Center).Shrink(0)
	bg := p.Control
	if b.Pressed() {
		bg = t.SurfacePressed
	} else if b.Hovered() {
		bg = t.SurfaceHover
	}
	b.Background(b.AnimateColor("bg", bg, motionHover))
	b.Children(func() {
		if icons != nil {
			ui.Icon(c, icon(icons[*chosen])).FontSize(14)
		} else {
			ui.Text(c, labels[*chosen]).FontSize(12).FontWeight(500).SingleLine().FontFeatures("tnum")
		}
		ui.Icon(c, icon("chevrons-up-down")).FontSize(11).TextColor(t.TextMuted)
	})
	b.Menu(func(m *ui.Menu) {
		for i, l := range labels {
			if m.Item(l).Checked(i == *chosen).Chosen() {
				*chosen = i
			}
		}
	})
	return b
}

// segmentedOf is a segmented capsule whose segments build sets up.
func segmentedOf(c *ui.Context, t *ui.Theme, p *palette, chosen *int, n int, build func(i int, s ui.Element)) ui.Element {
	seg := ui.SegmentedBase(c, chosen, n)
	seg.Track.Height(28).Padding(2).Radius(14).Background(p.Control)
	x := seg.Track.AnimateTo("thumb", float32(*chosen), motionPanel)
	thumb, shadow, dark := p.Raised, p.Shadow, t.Dark
	seg.Track.Draw(func(pt *ui.Painter, r ui.Rect) {
		w := (r.W - 4) / float32(n)
		at := ui.Rect{X: r.X + 2 + x*w, Y: r.Y + 2, W: w, H: r.H - 4}
		if !dark {
			pt.Shadow(at, 12, 0, 1, 3, 0, shadow)
		}
		pt.Fill(at, thumb, 12)
	})
	seg.Track.Children(func() {
		for i := range n {
			s := seg.Segment(i).Grow(1).Basis(0).Height(24).Center()
			color := t.TextMuted
			if i == *chosen {
				color = t.Text
			}
			s.TextColor(s.AnimateColor("fg", color, motionHover))
			build(i, s)
		}
	})
	return seg.Track
}

// pageTitle is the title of a page in the title bar, and a line under it
// of what the page holds.
func pageTitle(c *ui.Context, t *ui.Theme, title, subtitle string) {
	ui.Column(c).Shrink(1).Gap(1).Children(func() {
		ui.Text(c, title).FontSize(14).FontWeight(700).SingleLine()
		if subtitle != "" {
			ui.Text(c, subtitle).FontSize(11).TextColor(t.TextMuted).SingleLine().FontFeatures("tnum")
		}
	})
}

// stripe gives a list's row i its fill: gray under the pointer, a faint
// band on every other row.
func stripe(r ui.Element, p *palette, i int) ui.Element {
	r.Radius(6)
	if r.Hovered() {
		r.Background(p.Hover)
	} else if i%2 == 1 {
		r.Background(p.Stripe)
	}
	return r
}

// group is a rounded gray panel on the content, holding a summary or a
// set of rows.
func group(c *ui.Context, p *palette) ui.Element {
	return ui.Column(c).Radius(12).Background(p.Group)
}

// emptyState is a centered message with an icon, as for an empty folder.
func emptyState(c *ui.Context, t *ui.Theme, p *palette, ic, title, detail string, action func()) {
	ui.Column(c).Grow(1).Center().Gap(6).Padding(40).Transition(popIn).Children(func() {
		ui.Box(c).Size(52, 52).Radius(14).Center().Background(p.Group).Margin(0, 0, 8).Children(func() {
			ui.Icon(c, icon(ic)).FontSize(24).TextColor(p.Faint)
		})
		ui.Text(c, title).FontSize(15).FontWeight(600)
		if detail != "" {
			ui.Text(c, detail).TextColor(t.TextMuted).TextAlign(ui.Center).MaxWidth(360)
		}
		if action != nil {
			if pill(c, t, p, "refresh-cw", "重试", false).Margin(10, 0, 0).Clicked() {
				action()
			}
		}
	})
}

// skeletonRows shows placeholder rows pulsing while a listing loads.
func skeletonRows(c *ui.Context, t *ui.Theme, p *palette) {
	ui.Column(c).Grow(1).Padding(6, 12).Gap(0).Children(func() {
		for i := range 14 {
			row := ui.Row(c).Height(rowHeight).PaddingX(10).Gap(10).AlignItems(ui.Center).Radius(6)
			if i%2 == 1 {
				row.Background(p.Stripe)
			}
			pulse := row.Loop("pulse", 1400*time.Millisecond, ui.Bounce(ui.EaseInOut))
			row.Opacity(0.55 + 0.45*pulse - float32(i)*0.03)
			row.Children(func() {
				ui.Box(c).Size(16, 16).Radius(4).Background(p.Control)
				ui.Box(c).Size(float32(140+(i*53)%160), 9).Radius(4).Background(p.Control)
			})
		}
	})
}
