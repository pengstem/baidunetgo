package app

import (
	"testing"
	"time"

	"github.com/pengstem/baidunetgo/internal/transfer"
)

func TestTransferProgress(t *testing.T) {
	n, frac := transferProgress([]transfer.Task{
		{State: transfer.Running, Size: 300, Done: 150},
		{State: transfer.Queued, Size: 100},
		{State: transfer.Done, Size: 1000, Done: 1000}, // over: not counted
		{State: transfer.Paused, Size: 1000, Done: 10},
		{State: transfer.Queued}, // a size not known yet
	})
	if n != 3 || frac != 150.0/400 {
		t.Fatalf("got %d tasks at %v, want 3 at %v", n, frac, 150.0/400)
	}
	if n, frac := transferProgress(nil); n != 0 || frac != 0 {
		t.Fatalf("no tasks: %d at %v", n, frac)
	}
}

func TestTransfersWaitWhileOffline(t *testing.T) {
	a, tt, _ := signedIn(t)
	a.settings.DownloadDir = t.TempDir()
	a.sys.offline = true
	a.holdTransfers()
	a.router.Push(filesRoute("/文档"))
	waitFor(t, tt, "读我.txt")
	a.download(a.listings["/文档"].files)
	time.Sleep(200 * time.Millisecond)
	tt.Advance(100 * time.Millisecond)
	if len(a.transfers.tasks) != 4 {
		t.Fatalf("tasks %+v", a.transfers.tasks)
	}
	for _, task := range a.transfers.tasks {
		if task.State != transfer.Queued {
			t.Fatalf("%s is %v offline, want queued", task.Name, task.State)
		}
	}
	a.sys.offline = false
	a.holdTransfers()
	deadline := time.Now().Add(10 * time.Second)
	for {
		time.Sleep(20 * time.Millisecond)
		tt.Advance(50 * time.Millisecond)
		done := 0
		for _, task := range a.transfers.tasks {
			if task.State == transfer.Done {
				done++
			}
		}
		if done == 4 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("back online, downloads %+v", a.transfers.tasks)
		}
	}
}
