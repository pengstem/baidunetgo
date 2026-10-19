// Package app is BaiduNetGo's interface, drawn with MyGo's native ui
// package. Its state lives in App; views only read it, and work done on
// other goroutines writes back through App.update.
package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/auth"
)

// App is the state of the window.
type App struct {
	win    *mygo.Window
	router *ui.Router

	store *auth.Store
	phase phase
	login loginState
	sess  *session

	sidebarCollapsed bool
	profileOpen      bool
}

// New makes the app's state; Attach gives it its window.
func New() *App {
	r := ui.NewRouter("/files")
	r.Animation = &springPanel
	return &App{router: r}
}

// Attach gives the app the window that shows it.
func (a *App) Attach(win *mygo.Window) {
	a.win = win
	a.store = openStore()
	a.boot()
}

// update runs fn on the UI thread and redraws, from any goroutine. Without
// a window, as in tests, it runs fn at once.
func (a *App) update(fn func()) {
	if a.win == nil {
		fn()
		return
	}
	a.win.Update(fn)
}

// View builds the window's interface.
func (a *App) View(c *ui.Context) {
	t, p := applyTheme(c)
	switch a.phase {
	case phaseBooting:
		ui.Column(c).Fill().Center().Background(t.Background).Children(func() {
			ui.Spinner(c).Size(28, 28).Label("加载中")
		})
	case phaseLogin:
		a.loginView(c, t, p)
	default:
		ui.Box(c).Key("shell").Fill().Transition(ui.ElementTransition{
			Animation: &springPanel,
			Enter:     &ui.Motion{Scale: 0.97},
		}).Children(func() { a.shell(c, t, p) })
	}
}
