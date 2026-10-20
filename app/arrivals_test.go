package app

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/transfer"
)

func listed(a *App, dir, name string) bool {
	return slices.ContainsFunc(a.listings[dir].files, func(f baidu.File) bool { return f.Name == name })
}

func TestDeleteDissolves(t *testing.T) {
	a, tt, srv := signedIn(t)
	f := fileNamed(t, a, "/", "安装包.dmg")
	a.remove("/", []baidu.File{f})
	tt.Advance(100 * time.Millisecond)
	// It stays while it dissolves, through the reload too, and is not
	// chosen or opened meanwhile.
	settleFor(tt, 300*time.Millisecond)
	if !listed(a, "/", "安装包.dmg") || a.dying[f.FsID] == nil {
		t.Fatal("left before dissolving")
	}
	a.listings["/"].sel.Add(f.FsID)
	tt.Advance(50 * time.Millisecond)
	if a.listings["/"].sel.Has(f.FsID) {
		t.Error("a dissolving file stays chosen")
	}
	settleFor(tt, dissolveFor)
	if listed(a, "/", "安装包.dmg") || len(a.dying) != 0 || srv.Exists("/安装包.dmg") {
		t.Fatal("not gone after dissolving")
	}
}

func TestUndoSparesDissolving(t *testing.T) {
	a, tt, _ := signedIn(t)
	f := fileNamed(t, a, "/", "安装包.dmg")
	a.remove("/", []baidu.File{f})
	waitFor(t, tt, "撤销")
	if err := tt.Click("撤销"); err != nil {
		t.Fatal(err)
	}
	settleFor(tt, 2*dissolveFor)
	if !listed(a, "/", "安装包.dmg") || len(a.dying) != 0 {
		t.Fatal("undone file dissolved anyway")
	}
}

func TestUploadShowsWhileItGoes(t *testing.T) {
	a, tt, _ := signedIn(t)
	a.transfers.tasks = []transfer.Task{
		{ID: 1, Kind: transfer.Upload, Name: "旅行.mov", Remote: "/旅行.mov", State: transfer.Running, Size: 100 << 20, Done: 50 << 20},
		{ID: 2, Kind: transfer.Upload, Name: "a.jpg", Remote: "/相册/a.jpg", State: transfer.Queued, Size: 1 << 20},
		{ID: 3, Kind: transfer.Upload, Name: "b.jpg", Remote: "/照片/新的/b.jpg", State: transfer.Running, Size: 1 << 20},
	}
	a.transfers.gen++
	waitFor(t, tt, "上传中 50%")
	if !tt.HasText("旅行.mov") || !tt.HasText("相册") || !tt.HasText("等待上传") {
		t.Fatalf("uploads not shown: %q", tt.Texts())
	}
	if listed(a, "/", "新的") {
		t.Error("an upload into a folder below shows here")
	}
	ghost := a.listings["/"].files[slices.IndexFunc(a.listings["/"].files, func(f baidu.File) bool { return f.Name == "相册" })]
	if ghost.FsID >= 0 || !ghost.IsDir {
		t.Errorf("folder upload shows as %+v", ghost)
	}
	settleFor(tt, time.Second)
	save(t, tt, "uploads-gathering")

	// A failed upload dissolves; one taken off the queue too.
	a.transfers.tasks = a.transfers.tasks[:1]
	a.transfers.tasks[0].State = transfer.Failed
	a.transfers.gen++
	settleFor(tt, dissolveFor+200*time.Millisecond)
	if listed(a, "/", "旅行.mov") || listed(a, "/", "相册") || len(a.gathers) != 0 {
		t.Fatalf("failed uploads still shown: %v", names(a.listings["/"].files))
	}
}

// settleFor builds frames for d of the tester's time, letting the netdisk
// answer meanwhile.
func settleFor(tt interface{ Advance(time.Duration) }, d time.Duration) {
	for step := 60 * time.Millisecond; d > 0; d -= step {
		time.Sleep(10 * time.Millisecond)
		tt.Advance(step)
	}
}

// TestDeleteKeepsThumbnails deletes one picture: the folder read again
// signs its thumbnail URLs afresh, and the other pictures keep showing
// the thumbnails they had, not fetching them again.
func TestDeleteKeepsThumbnails(t *testing.T) {
	a, tt, _ := signedIn(t)
	a.gridView = true
	a.router.Push(filesRoute("/照片"))
	waitFor(t, tt, "IMG_2310.jpg")
	l := a.listingFor("/照片")
	shown := map[int64]*ui.Bitmap{}
	deadline := time.Now().Add(5 * time.Second)
	for len(shown) < 4 && time.Now().Before(deadline) {
		settleFor(tt, 100*time.Millisecond)
		for _, f := range l.files {
			if bmp := a.thumb(&f); bmp != nil {
				shown[f.FsID] = bmp
			}
		}
	}
	if len(shown) < 4 {
		t.Fatalf("%d thumbnails arrived", len(shown))
	}
	a.remove("/照片", []baidu.File{fileNamed(t, a, "/照片", "IMG_2310.jpg")})
	loads := l.loads
	for l.loads == loads && time.Now().Before(deadline.Add(5*time.Second)) {
		settleFor(tt, 60*time.Millisecond)
	}
	if l.loads == loads {
		t.Fatal("not read again")
	}
	for _, f := range l.files {
		if was := shown[f.FsID]; was != nil && f.Name != "IMG_2310.jpg" {
			if bmp := a.thumb(&f); bmp != was {
				t.Fatalf("%s lost its thumbnail when the folder was read again", f.Name)
			}
		}
	}
}
