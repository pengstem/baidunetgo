// Package app is BaiduNetGo's interface, drawn with MyGo's native ui
// package. Its state lives in App; views only read it, and work done on
// other goroutines writes back through App.update.
package app

import (
	"os"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/loading"
	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/auth"
	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/stream"
	"github.com/pengstem/baidunetgo/internal/transfer"
)

// App is the state of the window.
type App struct {
	win    *mygo.Window
	router *ui.Router

	store *auth.Store
	// saveMu guards saving, the client whose session is kept, and saved,
	// what was last written.
	saveMu sync.Mutex
	saving *baidu.Client
	saved  *baidu.Credentials
	phase  phase
	login  loginState
	sess   *session
	fake   bool // signed in to a pretend netdisk (BAIDUNETGO_FAKE)

	listings    map[string]*listing
	listingTick uint64
	order       baidu.Order
	gridView    bool
	tileNames   map[string]string // names laid out for grid tiles (tileName)
	gridSize    [2]float32        // the grid's tiles' width and its height, as last laid out
	thumbs      *thumbs
	dying       map[int64]*dying   // files deleted, dissolving (arrivals.go)
	gathers     map[string]*gather // uploads, by the entry of their folder they make
	passed      time.Time          // when passTime last moved them on
	dropped     time.Time          // when files were last dropped on the browser
	gpu         bool               // the window draws on a GPU, so shaders draw at speed

	pv            *previewState
	dlg           dialog
	recycle       recycleState
	recycleDirty  bool
	found         searchState
	xfer          *transfer.Manager
	stream        *stream.Server
	tuning        bool   // measuring download speeds
	tuneErr       string // why the last measurement failed
	tuneStage     string // what the measurement measures now
	tuneOpen      bool   // the measurement's details unfolded in settings
	transfers     transfersState
	sys           systemState
	shareForm     shareForm
	shares        sharesState
	sharesDirty   bool
	offline       offlineState
	copied        string // the toast after copying
	query         string
	copyText      string // to put on the clipboard in the next frame
	copyPNG       []byte // the same, for a picture
	settings      settings
	pendingToasts []ui.Toast

	sidebarCollapsed bool
	compact          bool // the title bar is narrow: choices fold into pop-up buttons
	profileOpen      bool
	navAt            string // the sidebar page shown, to notice a change
	navRun           uint64 // counts changes, to replay the icon's gesture

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
	r.Animation = &motionPanel
	return &App{router: r, order: baidu.Order{By: "name"}, gridView: os.Getenv("BAIDUNETGO_GRID") != ""}
}

// Attach gives the app the window that shows it.
func (a *App) Attach(win *mygo.Window) {
	a.win = win
	a.store = OpenStore()
	a.settings = loadSettings()
	a.useSettings()
	a.watchSystem()
	a.boot()
}

// copy puts text on the clipboard in the next frame, saying msg.
func (a *App) copy(text, msg string) { a.copyText, a.copied = text, msg }

// copyPicture puts a PNG picture on the clipboard in the next frame,
// saying msg.
func (a *App) copyPicture(png []byte, msg string) { a.copyPNG, a.copied = png, msg }

// Shutdown saves what must outlast the app, as it quits.
func (a *App) Shutdown() {
	if a.sess != nil {
		a.persist(a.sess.client)
	}
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
	a.passTime(c.Now())
	if a.moving() {
		c.Invalidate()
	}
	t, p := applyTheme(c)
	switch a.phase {
	case phaseBooting:
		ui.Column(c).Fill().Center().Background(t.Background).Children(func() {
			// The one thing on the window: loading.dev's Pulse, in the accent.
			loading.Pulse(c, loading.Options{Size: 32, Color: t.Accent}).Label("加载中")
		})
	case phaseLogin:
		a.loginView(c, t, p)
	default:
		ui.Box(c).Key("shell").Fill().Transition(ui.ElementTransition{
			Animation: &motionPanel,
			Enter:     &ui.Motion{Scale: 0.97},
		}).Children(func() { a.shell(c, t, p) })
		a.dialogs(c, t, p)
		a.previewOverlay(c, t, p)
	}
	a.toasts(c, t, p)
	if a.copyText != "" || a.copyPNG != nil {
		if a.copyText != "" {
			c.WriteClipboard(a.copyText)
		} else {
			writePNG(c, a.copyPNG)
		}
		msg := a.copied
		if msg == "" {
			msg = "已复制"
		}
		a.toast(msg, "success")
		a.copyText, a.copyPNG, a.copied = "", nil, ""
	}
}
