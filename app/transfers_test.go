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
