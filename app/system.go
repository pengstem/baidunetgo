package app

import (
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"github.com/pengstem/baidunetgo/internal/transfer"
)

// systemState is what the app has told the system, and heard from it.
type systemState struct {
	offline bool // the computer has no network
	asleep  bool // the computer sleeps, or just woke

	badge    int     // the count on the Dock icon
	progress float64 // the bar on it, in hundredths
	awake    func()  // releases keeping the computer awake
}

// watchSystem follows the network and sleep, holding transfers while the
// computer is offline or asleep and going on once it is back.
func (a *App) watchSystem() {
	if a.win == nil {
		return
	}
	a.sys.offline = !mygo.Network.Status().Online
	mygo.Network.OnChanged(func(s mygo.NetworkStatus) {
		a.update(func() {
			if a.sys.offline == !s.Online {
				return
			}
			a.sys.offline = !s.Online
			if a.activeTransfers() > 0 {
				if a.sys.offline {
					a.toast("网络已断开，传输会在恢复后继续", "")
				} else {
					a.toast("网络已恢复，继续传输", "success")
				}
			}
			a.holdTransfers()
		})
	})
	mygo.Power.OnSuspend(func() {
		a.update(func() {
			a.sys.asleep = true
			a.holdTransfers()
		})
	})
	mygo.Power.OnResume(func() {
		// Give Wi-Fi a moment to join again; if it stays down, the
		// network's change holds the transfers on.
		time.AfterFunc(3*time.Second, func() {
			a.update(func() {
				a.sys.asleep = false
				a.holdTransfers()
			})
		})
	})
}

// holdTransfers holds the queue while the computer is offline or asleep.
func (a *App) holdTransfers() {
	if a.xfer == nil {
		return
	}
	if a.sys.offline || a.sys.asleep {
		a.xfer.Hold()
	} else {
		a.xfer.Unhold()
	}
}

// showTransfers puts the transfers on the Dock icon, a count and a bar,
// and keeps the computer from sleeping while they run.
func (a *App) showTransfers(tasks []transfer.Task) {
	if a.win == nil {
		return
	}
	n, frac := transferProgress(tasks)
	if n != a.sys.badge {
		a.sys.badge = n
		mygo.App.SetBadgeCount(n)
	}
	if p := math.Floor(frac * 100); p != a.sys.progress {
		a.sys.progress = p
		bar := mygo.ProgressBar{}
		if n > 0 {
			// At least a sliver, so that a task starting shows a bar.
			bar.Value = max(p/100, 0.01)
		}
		a.win.SetProgressBar(bar)
	}
	switch {
	case n > 0 && a.sys.awake == nil:
		a.sys.awake = mygo.Power.KeepAwake("正在传输文件", false)
	case n == 0 && a.sys.awake != nil:
		a.sys.awake()
		a.sys.awake = nil
	}
}

// transferProgress returns how many tasks are running or waiting, and the
// fraction of their bytes done.
func transferProgress(tasks []transfer.Task) (int, float64) {
	n := 0
	var done, size int64
	for _, t := range tasks {
		if t.State != transfer.Running && t.State != transfer.Queued {
			continue
		}
		n++
		if t.Size > 0 {
			done += min(t.Done, t.Size)
			size += t.Size
		}
	}
	if size == 0 {
		return n, 0
	}
	return n, float64(done) / float64(size)
}

// Notification opens what a clicked notification tells of: a download in
// its folder, the folder an upload went into in the window. Its id is
// one finished gives, "file:<local path>" or "dir:<netdisk folder>".
func (a *App) Notification(id string) {
	kind, arg, _ := strings.Cut(id, ":")
	switch kind {
	case "file":
		mygo.Shell.ShowItemInFolder(arg)
	case "dir":
		if a.win == nil {
			return // the click launched the app: it opens on its files
		}
		a.update(func() {
			if a.phase == phaseReady {
				a.router.Push(filesRoute(arg))
			}
		})
		a.win.Show()
	}
}
