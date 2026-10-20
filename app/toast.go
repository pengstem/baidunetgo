package app

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// toast shows a message at the bottom of the window. It runs on the UI
// thread, from the view or an update.
func (a *App) toast(msg, kind string) {
	a.pendingToasts = append(a.pendingToasts, ui.Toast{Title: msg, Type: kind})
}

// toasts shows the messages asked for, with the app's look.
func (a *App) toasts(c *ui.Context, t *ui.Theme, p *palette) {
	for _, tt := range a.pendingToasts {
		c.AddToast(tt)
	}
	a.pendingToasts = nil
	ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
		viewport.Padding(20).AlignItems(ui.Center).Gap(8)
		for _, tt := range toasts {
			toast := ui.ToastBase(c, tt)
			toast.Root.Row().Gap(10).Padding(8, 10, 8, 14).Radius(12).AlignItems(ui.Center).MaxWidth(520).
				Background(p.Card).Border(1, p.Hairline).Shadow(0, 14, 36, -10, p.Shadow).
				Transition(ui.ElementTransition{Animation: &motionFade, Enter: &ui.Motion{Y: 8, Opacity: 0}, Exit: &ui.Motion{Opacity: 0}})
			toast.Root.Children(func() {
				ic, color := "info", t.Accent
				switch tt.Type {
				case "error":
					ic, color = "triangle-alert", t.Danger
				case "success":
					ic, color = "circle-check", t.Success
				}
				ui.Icon(c, icon(ic)).FontSize(17).TextColor(color)
				ui.Column(c).Shrink(1).Gap(2).Children(func() {
					ui.Text(c, tt.Title).FontWeight(500)
					if tt.Description != "" {
						ui.Text(c, tt.Description).FontSize(t.Rem(0.86)).TextColor(t.TextMuted)
					}
				})
				if tt.Action != "" {
					toast.ActionButton().Padding(4, 10).Radius(7).Background(p.Control).TextColor(t.Accent).
						Children(func() { ui.Text(c, tt.Action).FontWeight(500) })
				}
				close := toast.CloseButton().Label("关闭").Size(24, 24).Radius(12).Center().TextColor(t.TextMuted)
				if close.Hovered() {
					close.Background(p.Hover)
				}
				close.Children(func() { ui.Icon(c, icon("x")).FontSize(13) })
			})
		}
	})
}

var mdOptions markdown.Options

// mdView renders Markdown with the app's options.
func mdView(c *ui.Context, src string) {
	if mdOptions.Theme == nil || mdThemeDark != c.Theme().Dark {
		mdOptions.Theme = markdown.DefaultTheme(c.Theme())
		mdOptions.WrapCode = true
		mdOptions.OnLink = func(url string) { c.OpenURL(url) }
		mdThemeDark = c.Theme().Dark
	}
	markdown.View(c, src, &mdOptions)
}

var mdThemeDark bool

var _ = time.Second
