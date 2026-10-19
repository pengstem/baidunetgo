// Package app is BaiduNetGo's interface, drawn with MyGo's native ui
// package. Its state lives in App; views only read it, and work done on
// other goroutines writes back through App.update.
package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// App is the state of the window.
type App struct {
	win    *mygo.Window
	router *ui.Router

	sidebarCollapsed bool
}

// New makes the app's state; Attach gives it its window.
func New() *App {
	r := ui.NewRouter("/files")
	r.Animation = &springPanel
	return &App{router: r}
}

// Attach gives the app the window that shows it.
func (a *App) Attach(win *mygo.Window) { a.win = win }

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
	a.shell(c, t, p)
}
