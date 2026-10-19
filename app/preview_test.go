package app

import (
	"image"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

const (
	keyEscape = ui.KeyEscape
	keyRight  = ui.KeyRight
)

func fileNamed(t *testing.T, a *App, dir, name string) baidu.File {
	t.Helper()
	for _, f := range a.listings[dir].files {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("no %s in %s", name, dir)
	return baidu.File{}
}

func TestPreviewText(t *testing.T) {
	a, tt, _ := signedIn(t)
	a.router.Push(filesRoute("/文档"))
	waitFor(t, tt, "读我.txt")
	a.open(fileNamed(t, a, "/文档", "读我.txt"))
	deadline := time.Now().Add(5 * time.Second)
	for a.pv.buf == nil {
		if a.pv.err != "" || time.Now().After(deadline) {
			t.Fatalf("text did not load: %q", a.pv.err)
		}
		time.Sleep(10 * time.Millisecond)
		tt.Advance(50 * time.Millisecond)
	}
	if got := a.pv.buf.Line(0); got != "BaiduNetGo 演示文件" {
		t.Errorf("first line %q", got)
	}
	tt.Advance(time.Second)
	save(t, tt, "preview-text")
	tt.Key(0, keyEscape)
	tt.Advance(time.Second)
	if a.pv != nil {
		t.Error("Escape did not close the preview")
	}
}

func TestPreviewMarkdownAndPDF(t *testing.T) {
	a, tt, _ := signedIn(t)
	a.router.Push(filesRoute("/文档"))
	waitFor(t, tt, "旅行计划.md")
	a.open(fileNamed(t, a, "/文档", "旅行计划.md"))
	waitFor(t, tt, "慢慢走，比较快。")
	tt.Advance(time.Second)
	save(t, tt, "preview-md")
	a.open(fileNamed(t, a, "/文档", "年度报告.pdf"))
	deadline := time.Now().Add(20 * time.Second)
	for a.pv.doc == nil || a.pv.pages[0] == nil {
		if a.pv.err != "" || time.Now().After(deadline) {
			t.Fatalf("pdf did not render: %q", a.pv.err)
		}
		time.Sleep(20 * time.Millisecond)
		tt.Advance(50 * time.Millisecond)
	}
	tt.Advance(time.Second)
	save(t, tt, "preview-pdf")
	if len(a.pv.doc.Pages) != 4 {
		t.Errorf("pages %d", len(a.pv.doc.Pages))
	}
}

func TestPreviewImage(t *testing.T) {
	a, tt, _ := signedIn(t)
	a.gridView = true
	a.router.Push(filesRoute("/照片"))
	waitFor(t, tt, "IMG_2310.jpg")
	a.open(fileNamed(t, a, "/照片", "IMG_2312.jpg"))
	deadline := time.Now().Add(5 * time.Second)
	for a.pv.bmp == nil {
		if a.pv.err != "" || time.Now().After(deadline) {
			t.Fatalf("image did not load: %q", a.pv.err)
		}
		time.Sleep(10 * time.Millisecond)
		tt.Advance(50 * time.Millisecond)
	}
	tt.Advance(time.Second)
	save(t, tt, "preview-image")
	if !tt.HasText("图片 · 15 KB · 640×480 · 3 / 24") {
		t.Errorf("counter missing: %q", tt.Texts())
	}
	tt.Key(0, keyRight)
	tt.Advance(100 * time.Millisecond)
	if a.pv.file.Name != "IMG_2313.jpg" {
		t.Errorf("Right went to %s", a.pv.file.Name)
	}
}

func save(t *testing.T, tt interface {
	Image() *image.RGBA
}, name string) {
	dir := os.Getenv("BAIDUNETGO_SNAPSHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(dir + "/" + name + ".png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, tt.Image())
}

func TestPreviewAudioStreams(t *testing.T) {
	a, tt, _ := signedIn(t)
	a.router.Push(filesRoute("/音乐"))
	waitFor(t, tt, "和弦.wav")
	a.open(fileNamed(t, a, "/音乐", "和弦.wav"))
	deadline := time.Now().Add(5 * time.Second)
	for a.pv.player == nil {
		if a.pv.err != "" || time.Now().After(deadline) {
			t.Fatalf("audio did not open: %q", a.pv.err)
		}
		time.Sleep(10 * time.Millisecond)
		tt.Advance(50 * time.Millisecond)
	}
	if d := a.pv.player.Duration(); d < 5*time.Second || d > 7*time.Second {
		t.Errorf("duration %v", d)
	}
	tt.Advance(500 * time.Millisecond)
	save(t, tt, "preview-audio")
	a.closePreview()
}
