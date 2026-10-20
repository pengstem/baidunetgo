package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// The look follows the 刻度 design system: warm neutral grays, one sky
// blue for what is happening (selection, progress, the main action), lines
// rather than cards, and figures set in a monospaced face.

// palette holds the colors of the app beyond those of the theme.
type palette struct {
	Sidebar  ui.Color // the sidebar's backdrop
	Card     ui.Color // dialogs, menus and popovers
	Raised   ui.Color // the chosen sidebar row, segmented thumbs
	Hover    ui.Color // rows under the pointer
	Selected ui.Color // chosen rows
	Scrim    ui.Color // behind dialogs and previews
	Hairline ui.Color // separators and outlines
	Faint    ui.Color // the quietest text: column heads, units, hints
	Shadow   ui.Color // under dialogs only
	Folder   ui.Color
	VIP      ui.Color // membership labels, outlined
}

var (
	lightTheme, darkTheme     *ui.Theme
	lightPalette, darkPalette palette
)

func init() {
	accentLight := ui.Hex("#0a6fb0")
	accentDark := ui.Hex("#2aa8f2")

	t := ui.LightTheme()
	t.Background = ui.Hex("#fafaf8")
	t.Surface = ui.Hex("#f0f0ec")
	t.SurfaceHover = ui.Hex("#e8e8e3")
	t.SurfacePressed = ui.Hex("#dfdfd9")
	t.Border = ui.Hex("#e4e4df")
	t.Text = ui.Hex("#1b1b19")
	t.TextMuted = ui.Hex("#5e5e58")
	t.Accent = accentLight
	t.AccentHover = ui.Hex("#0a64a0")
	t.AccentPressed = ui.Hex("#085a90")
	t.AccentText = ui.Hex("#ffffff")
	t.Danger = ui.Hex("#d2402f")
	t.Success = ui.Hex("#1f9d63")
	t.Warning = ui.Hex("#b97a12")
	t.Selection = accentLight.Alpha(0.2)
	t.Focus = accentLight.Alpha(0.5)
	t.Radius = 5
	t.Font = fontSans
	t.FontSize = 13
	lightTheme = t
	lightPalette = palette{
		Sidebar:  ui.Hex("#f2f2ef"),
		Card:     ui.Hex("#ffffff"),
		Raised:   ui.Hex("#e6e6e1"),
		Hover:    ui.RGBA(27, 27, 25, 0.045),
		Selected: accentLight.Alpha(0.12),
		Scrim:    ui.RGBA(20, 20, 18, 0.36),
		Hairline: ui.Hex("#e4e4df"),
		Faint:    ui.Hex("#6f6f69"),
		Shadow:   ui.RGBA(20, 20, 18, 0.14),
		Folder:   ui.Hex("#d4951f"),
		VIP:      ui.Hex("#a8700c"),
	}

	d := ui.DarkTheme()
	d.Background = ui.Hex("#131312")
	d.Surface = ui.Hex("#1f1f1e")
	d.SurfaceHover = ui.Hex("#262625")
	d.SurfacePressed = ui.Hex("#2c2c2a")
	d.Border = ui.Hex("#2a2a28")
	d.Text = ui.Hex("#ecece8")
	d.TextMuted = ui.Hex("#a3a39d")
	d.Accent = accentDark
	d.AccentHover = ui.Hex("#4db6f4")
	d.AccentPressed = ui.Hex("#1e95db")
	d.AccentText = ui.Hex("#061520")
	d.Danger = ui.Hex("#ef6b5a")
	d.Success = ui.Hex("#3fc28a")
	d.Warning = ui.Hex("#e3a93b")
	d.Selection = accentDark.Alpha(0.3)
	d.Focus = accentDark.Alpha(0.6)
	d.Radius = 5
	d.Font = fontSans
	d.FontSize = 13
	darkTheme = d
	darkPalette = palette{
		Sidebar:  ui.Hex("#191918"),
		Card:     ui.Hex("#1c1c1b"),
		Raised:   ui.Hex("#252524"),
		Hover:    ui.RGBA(255, 255, 255, 0.04),
		Selected: accentDark.Alpha(0.14),
		Scrim:    ui.RGBA(0, 0, 0, 0.55),
		Hairline: ui.Hex("#2a2a28"),
		Faint:    ui.Hex("#858580"),
		Shadow:   ui.RGBA(0, 0, 0, 0.45),
		Folder:   ui.Hex("#e3a93b"),
		VIP:      ui.Hex("#e3a93b"),
	}
}

// applyTheme sets the app's theme for this frame, following the system's
// light or dark appearance, and returns it with the matching palette.
func applyTheme(c *ui.Context) (*ui.Theme, *palette) {
	if c.Theme().Dark {
		c.SetTheme(darkTheme)
		return darkTheme, &darkPalette
	}
	c.SetTheme(lightTheme)
	return lightTheme, &lightPalette
}

// Motion shared across the app: one curve, no bounce, and three lengths
// by what moves.
var (
	easeStandard = ui.CubicBezier(0.2, 0, 0, 1)

	motionHover = ui.Animation{Duration: 120 * time.Millisecond, Ease: easeStandard} // colors under the pointer
	motionFade  = ui.Animation{Duration: 160 * time.Millisecond, Ease: easeStandard} // menus, tips, dialogs
	motionPanel = ui.Animation{Duration: 180 * time.Millisecond, Ease: easeStandard} // panels, pages, shared elements

	// navIconRun is how long a sidebar icon's gesture takes, delays
	// included: the one motion over 180 ms, as it goes and comes back.
	navIconRun = 360 * time.Millisecond

	// rowEnter brings rows in faded, from a little below.
	rowEnter = ui.ElementTransition{
		Animation: &motionPanel,
		Enter:     &ui.Motion{Y: 4, Opacity: 0},
		Exit:      &ui.Motion{Opacity: 0, Collapse: true},
	}
	// popIn fades panels in from a hair smaller.
	popIn = ui.ElementTransition{
		Animation: &motionFade,
		Enter:     &ui.Motion{Scale: 0.98, Opacity: 0},
		Exit:      &ui.Motion{Opacity: 0},
	}
)

// stagger returns a copy of a with a delay growing with i, capped so that
// long lists do not wait.
func stagger(a ui.Animation, i int) *ui.Animation {
	if i > 14 {
		i = 14
	}
	a.Delay = time.Duration(i) * 16 * time.Millisecond
	return &a
}
