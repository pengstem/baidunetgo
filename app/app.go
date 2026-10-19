// Package app is BaiduNetGo's interface, drawn with MyGo's native ui
// package. Its state lives in App; views only read it, and work done on
// other goroutines writes back through App.update.
package app

import (
	"os"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/auth"
	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/transfer"
)

// App is the state of the window.
type App struct {
	win    *mygo.Window
	router *ui.Router

	store *auth.Store
	phase phase
	login loginState
	sess  *session
	fake  bool // signed in to a pretend netdisk (BAIDUNETGO_FAKE)

	listings    map[string]*listing
	listingTick uint64
	order       baidu.Order
	gridView    bool
	thumbs      *thumbs

	pv            *previewState
	dlg           dialog
	recycle       recycleState
	recycleDirty  bool
	found         searchState
	xfer          *transfer.Manager
	transfers     transfersState
	shareForm     shareForm
	shares        sharesState
	sharesDirty   bool
	offline       offlineState
	copied        string // the toast after copying
	query         string
	copyText      string // to put on the clipboard in the next frame
	settings      settings
	pendingToasts []ui.Toast

	sidebarCollapsed bool
	profileOpen      bool

	queueMu sync.Mutex
	queued  []func()
}

// New makes the app's state; Attach gives it its window.
func New() *App {
	start := "/files"
	if r := os.Getenv("BAIDUNETGO_ROUTE"); r != "" {
		start = r // for looking at a page from scripts
	}
	r := ui.NewRouter(start)
	r.Animation = &springPanel
	return &App{router: r, order: baidu.Order{By: "name"}, gridView: os.Getenv("BAIDUNETGO_GRID") != ""}
}

// Attach gives the app the window that shows it.
func (a *App) Attach(win *mygo.Window) {
	a.win = win
	a.store = openStore()
	a.settings = loadSettings()
	a.useSettings()
	a.boot()
}

// copy puts text on the clipboard in the next frame, saying msg.
func (a *App) copy(text, msg string) { a.copyText, a.copied = text, msg }

// Shutdown saves what must outlast the app, as it quits.
func (a *App) Shutdown() {
	if a.xfer != nil {
		a.xfer.Close()
	}
}

// update runs fn on the UI thread and redraws, from any goroutine. Without
// a window, as in tests, it runs fn at once.
func (a *App) update(fn func()) {
	if a.win == nil {
		a.queueMu.Lock()
		a.queued = append(a.queued, fn)
		a.queueMu.Unlock()
		return
	}
	a.win.Update(fn)
}

// runQueued runs the updates queued without a window, in tests, which
// build frames as a window would.
func (a *App) runQueued(c *ui.Context) {
	a.queueMu.Lock()
	q := a.queued
	a.queued = nil
	a.queueMu.Unlock()
	for _, fn := range q {
		fn()
	}
	c.After(16 * time.Millisecond)
}

// View builds the window's interface.
func (a *App) View(c *ui.Context) {
	if a.win == nil {
		a.runQueued(c)
	}
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
		a.dialogs(c, t, p)
		a.previewOverlay(c, t, p)
	}
	a.toasts(c, t, p)
	if a.copyText != "" {
		c.WriteClipboard(a.copyText)
		msg := a.copied
		if msg == "" {
			msg = "已复制"
		}
		a.toast(msg, "success")
		a.copyText, a.copied = "", ""
	}
}
