package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// palette holds the colors of the app beyond those of the theme.
type palette struct {
	Sidebar    ui.Color // the sidebar's backdrop
	Card       ui.Color // raised panels
	Hover      ui.Color // rows under the pointer
	Selected   ui.Color // chosen rows
	Scrim      ui.Color // behind dialogs and previews
	Hairline   ui.Color // thin separators
	Shadow     ui.Color
	AccentSoft ui.Color // tinted backgrounds of accent things
	Folder     ui.Color
}

var (
	lightTheme, darkTheme     *ui.Theme
	lightPalette, darkPalette palette
)

func init() {
	accentLight := ui.Oklch(0.58, 0.19, 262)
	accentDark := ui.Oklch(0.7, 0.15, 262)

	t := ui.LightTheme()
	t.Background = ui.Hex("#fbfbfc")
	t.Surface = ui.Hex("#f1f2f5")
	t.SurfaceHover = ui.Hex("#e8e9ee")
	t.SurfacePressed = ui.Hex("#dedfe5")
	t.Border = ui.Hex("#e2e3e8")
	t.Text = ui.Hex("#16171b")
	t.TextMuted = ui.Hex("#6c707b")
	t.Accent = accentLight
	t.AccentHover = ui.Oklch(0.53, 0.19, 262)
	t.AccentPressed = ui.Oklch(0.48, 0.19, 262)
	t.Selection = accentLight.Alpha(0.22)
	t.Focus = accentLight.Alpha(0.5)
	t.Radius = 8
	lightTheme = t
	lightPalette = palette{
		Sidebar:    ui.Hex("#f3f4f7"),
		Card:       ui.Hex("#ffffff"),
		Hover:      ui.RGBA(20, 24, 40, 0.045),
		Selected:   accentLight.Alpha(0.12),
		Scrim:      ui.RGBA(12, 14, 20, 0.38),
		Hairline:   ui.RGBA(20, 24, 40, 0.08),
		Shadow:     ui.RGBA(16, 20, 40, 0.12),
		AccentSoft: accentLight.Alpha(0.1),
		Folder:     ui.Oklch(0.78, 0.13, 80),
	}

	d := ui.DarkTheme()
	d.Background = ui.Hex("#111216")
	d.Surface = ui.Hex("#1c1d22")
	d.SurfaceHover = ui.Hex("#25272d")
	d.SurfacePressed = ui.Hex("#2e3037")
	d.Border = ui.Hex("#2b2d34")
	d.Text = ui.Hex("#eceef2")
	d.TextMuted = ui.Hex("#8d919c")
	d.Accent = accentDark
	d.AccentHover = ui.Oklch(0.75, 0.14, 262)
	d.AccentPressed = ui.Oklch(0.64, 0.15, 262)
	d.AccentText = ui.Hex("#0b0c10")
	d.Selection = accentDark.Alpha(0.3)
	d.Focus = accentDark.Alpha(0.6)
	d.Radius = 8
	darkTheme = d
	darkPalette = palette{
		Sidebar:    ui.Hex("#16171b"),
		Card:       ui.Hex("#18191e"),
		Hover:      ui.RGBA(255, 255, 255, 0.045),
		Selected:   accentDark.Alpha(0.16),
		Scrim:      ui.RGBA(0, 0, 0, 0.55),
		Hairline:   ui.RGBA(255, 255, 255, 0.07),
		Shadow:     ui.RGBA(0, 0, 0, 0.45),
		AccentSoft: accentDark.Alpha(0.12),
		Folder:     ui.Oklch(0.8, 0.12, 80),
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

// Motion shared across the app, so that it moves as one.
var (
	springSoft  = ui.Animation{Spring: ui.SpringOptions{Duration: 420 * time.Millisecond, Bounce: 0.08}}
	springPop   = ui.Animation{Spring: ui.SpringOptions{Duration: 380 * time.Millisecond, Bounce: 0.22}}
	springPanel = ui.Animation{Spring: ui.SpringOptions{Duration: 520 * time.Millisecond, Bounce: 0.04}}
	fadeQuick   = ui.Animation{Duration: 140 * time.Millisecond, Ease: ui.EaseOut}

	// rowEnter brings rows in from a little below, faded.
	rowEnter = ui.ElementTransition{
		Animation: &springSoft,
		Enter:     &ui.Motion{Y: 10, Opacity: 0},
		Exit:      &ui.Motion{Opacity: 0, Collapse: true},
	}
	// popIn grows panels in from a little smaller.
	popIn = ui.ElementTransition{
		Animation: &springPop,
		Enter:     &ui.Motion{Scale: 0.94, Opacity: 0},
		Exit:      &ui.Motion{Scale: 0.96, Opacity: 0},
	}
)

// stagger returns a copy of a with a delay growing with i, capped so that
// long lists do not wait.
func stagger(a ui.Animation, i int) *ui.Animation {
	if i > 14 {
		i = 14
	}
	a.Delay = time.Duration(i) * 22 * time.Millisecond
	return &a
}
