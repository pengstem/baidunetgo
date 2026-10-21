package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pengstem/baidunetgo/internal/transfer"
)

func TestDownloadAndUploadPage(t *testing.T) {
	a, tt, srv := signedIn(t)
	dir := t.TempDir()
	a.settings.DownloadDir = dir
	a.router.Push(filesRoute("/文档"))
	waitFor(t, tt, "读我.txt")
	a.download(a.listings["/文档"].files)
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
			t.Fatalf("downloads %+v", a.transfers.tasks)
		}
	}
	if data, err := os.ReadFile(filepath.Join(dir, "读我.txt")); err != nil || len(data) == 0 {
		t.Fatalf("not downloaded: %v", err)
	}

	local := filepath.Join(dir, "新照片.png")
	os.WriteFile(local, []byte("png"), 0o644)
	a.upload([]string{local}, "/照片")
	a.router.Push("/transfers")
	waitFor(t, tt, "进行中 0")
	for srv.Data("/照片/新照片.png") == nil {
		if time.Now().After(deadline.Add(5 * time.Second)) {
			t.Fatal("upload never arrived")
		}
		time.Sleep(20 * time.Millisecond)
		tt.Advance(50 * time.Millisecond)
	}
	if err := tt.Click("已完成 5"); err != nil {
		t.Fatalf("%v; %q", err, tt.Texts())
	}
	tt.Advance(time.Second)
	save(t, tt, "transfers")
}

// TestDropUploads drops a file from Finder on a folder's page, which
// uploads it there.
func TestDropUploads(t *testing.T) {
	a, tt, srv := signedIn(t)
	a.router.Push(filesRoute("/照片"))
	waitFor(t, tt, "IMG_2310.jpg")
	local := filepath.Join(t.TempDir(), "拖进来.png")
	os.WriteFile(local, []byte("png"), 0o644)
	if !tt.DropFiles(700, 400, local) {
		t.Fatal("the page did not take the drop")
	}
	deadline := time.Now().Add(5 * time.Second)
	for srv.Data("/照片/拖进来.png") == nil {
		if time.Now().After(deadline) {
			t.Fatalf("upload never arrived: %+v", a.transfers.tasks)
		}
		time.Sleep(20 * time.Millisecond)
		tt.Advance(50 * time.Millisecond)
	}
}

func TestSpeedCard(t *testing.T) {
	a, tt, _ := signedIn(t)
	a.transfers.tasks = []transfer.Task{
		{ID: 1, Kind: transfer.Download, Name: "电影.mkv", State: transfer.Running, Size: 4 << 30, Done: 3 << 30, Speed: 12 << 20},
		{ID: 2, Kind: transfer.Upload, Name: "照片.zip", State: transfer.Running, Size: 600 << 20, Done: 120 << 20, Speed: 3 << 20},
		{ID: 3, Kind: transfer.Download, Name: "文档.pdf", State: transfer.Queued, Size: 8 << 20},
	}
	for i := range speedHistory {
		a.transfers.speeds = append(a.transfers.speeds, float64(8+i%17)*(1<<20))
	}
	a.router.Push("/transfers")
	waitFor(t, tt, "当前速度")
	tt.Advance(time.Second)
	for _, s := range []string{"电影.mkv", "1 个下载 · 1 个上传 · 1 个排队 · 15 MB/s"} {
		if !tt.HasText(s) {
			t.Fatalf("no %q in %q", s, tt.Texts())
		}
	}
	save(t, tt, "transfers-going")
}
