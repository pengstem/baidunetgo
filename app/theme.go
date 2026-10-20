package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// The look is that of a native macOS app, as Pulse's: the sidebar over
// the window's translucent material, its pages marked by small tiles of
// color, the system's typeface and blue, content on white with rounded
// gray groups, and lists striped rather than ruled.

// palette holds the colors of the app beyond those of the theme.
type palette struct {
	Sidebar  ui.Color // the sidebar's backdrop where the window has no material
	Card     ui.Color // dialogs, menus and popovers
	Group    ui.Color // rounded groups on the content: settings, summaries
	Control  ui.Color // the face of quiet buttons, fields and segmented tracks
	Raised   ui.Color // the thumb of segmented controls
	Hover    ui.Color // rows under the pointer
	Stripe   ui.Color // every other row of a list
	Selected ui.Color // chosen rows while their list has not the focus
	Scrim    ui.Color // behind dialogs and previews
	Hairline ui.Color // separators and outlines
	Faint    ui.Color // the quietest text: section titles, units, hints
	Shadow   ui.Color // under dialogs, menus and thumbs
	Folder   ui.Color
	VIP      ui.Color // membership labels
}

var (
	lightTheme, darkTheme     *ui.Theme
	lightPalette, darkPalette palette
)

func init() {
	accentLight := ui.Hex("#007aff")
	accentDark := ui.Hex("#0a84ff")

	t := ui.LightTheme()
	t.Background = ui.Hex("#ffffff")
	t.Surface = ui.Hex("#f2f2f4")
	t.SurfaceHover = ui.Hex("#e8e8eb")
	t.SurfacePressed = ui.Hex("#dddde1")
	t.Border = ui.Hex("#e3e3e6")
	t.Text = ui.Hex("#1d1d1f")
	t.TextMuted = ui.Hex("#6e6e73")
	t.Accent = accentLight
	t.AccentHover = ui.Hex("#006ee6")
	t.AccentPressed = ui.Hex("#0062cc")
	t.AccentText = ui.Hex("#ffffff")
	t.Danger = ui.Hex("#ff3b30")
	t.Success = ui.Hex("#34c759")
	t.Warning = ui.Hex("#ff9500")
	t.Selection = accentLight.Alpha(0.22)
	t.Focus = accentLight.Alpha(0.5)
	t.Radius = 6
	t.FontSize = 13
	lightTheme = t
	lightPalette = palette{
		Sidebar:  ui.Hex("#ececee"),
		Card:     ui.Hex("#ffffff"),
		Group:    ui.Hex("#f5f5f7"),
		Control:  ui.Hex("#ececef"),
		Raised:   ui.Hex("#ffffff"),
		Hover:    ui.RGBA(0, 0, 0, 0.045),
		Stripe:   ui.RGBA(0, 0, 0, 0.028),
		Selected: ui.RGBA(0, 0, 0, 0.09),
		Scrim:    ui.RGBA(0, 0, 0, 0.28),
		Hairline: ui.Hex("#e5e5ea"),
		Faint:    ui.Hex("#a1a1a6"),
		Shadow:   ui.RGBA(0, 0, 0, 0.16),
		Folder:   ui.Hex("#4aa8f0"),
		VIP:      ui.Hex("#c27c0e"),
	}

	d := ui.DarkTheme()
	d.Background = ui.Hex("#1e1e1e")
	d.Surface = ui.Hex("#2c2c2e")
	d.SurfaceHover = ui.Hex("#353538")
	d.SurfacePressed = ui.Hex("#3e3e41")
	d.Border = ui.Hex("#3a3a3c")
	d.Text = ui.Hex("#f5f5f7")
	d.TextMuted = ui.Hex("#98989d")
	d.Accent = accentDark
	d.AccentHover = ui.Hex("#2b95ff")
	d.AccentPressed = ui.Hex("#0077e6")
	d.AccentText = ui.Hex("#ffffff")
	d.Danger = ui.Hex("#ff453a")
	d.Success = ui.Hex("#30d158")
	d.Warning = ui.Hex("#ff9f0a")
	d.Selection = accentDark.Alpha(0.35)
	d.Focus = accentDark.Alpha(0.6)
	d.Radius = 6
	d.FontSize = 13
	darkTheme = d
	darkPalette = palette{
		Sidebar:  ui.Hex("#262628"),
		Card:     ui.Hex("#2a2a2c"),
		Group:    ui.Hex("#252527"),
		Control:  ui.Hex("#323235"),
		Raised:   ui.Hex("#5a5a5e"),
		Hover:    ui.RGBA(255, 255, 255, 0.05),
		Stripe:   ui.RGBA(255, 255, 255, 0.03),
		Selected: ui.RGBA(255, 255, 255, 0.1),
		Scrim:    ui.RGBA(0, 0, 0, 0.5),
		Hairline: ui.Hex("#38383a"),
		Faint:    ui.Hex("#6e6e73"),
		Shadow:   ui.RGBA(0, 0, 0, 0.45),
		Folder:   ui.Hex("#5ab4f5"),
		VIP:      ui.Hex("#ffb340"),
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
