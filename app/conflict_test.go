package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/fakepan"
)

// waitUntil builds frames until ok, or fails.
func waitUntil(t *testing.T, tt *ui.Tester, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("%s never happened", what)
		}
		time.Sleep(10 * time.Millisecond)
		tt.Advance(50 * time.Millisecond)
	}
}

// TestUploadConflict asks before an upload takes a name the folder has:
// keeping both gives it a free name, replacing puts the old one in the
// recycle bin, skipping leaves it out.
func TestUploadConflict(t *testing.T) {
	a, tt, srv := signedIn(t)
	orig := string(srv.Data("/README.md"))
	local := filepath.Join(t.TempDir(), "README.md")
	os.WriteFile(local, []byte("新的说明"), 0o644)

	a.upload([]string{local}, "/")
	waitUntil(t, tt, "the question", func() bool { return a.dlg.open && a.dlg.kind == dlgConflict })
	tt.Advance(300 * time.Millisecond)
	save(t, tt, "dialog-conflict")
	if !tt.HasText("「我的文件」中已有「README.md」") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if err := tt.Click("保留两者"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, tt, "the upload", func() bool { return string(srv.Data("/README (1).md")) == "新的说明" })
	if string(srv.Data("/README.md")) != orig {
		t.Fatal("keeping both changed the one there")
	}

	a.upload([]string{local}, "/")
	waitUntil(t, tt, "the question", func() bool { return a.dlg.open && a.dlg.kind == dlgConflict })
	if err := tt.Click("跳过"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if srv.Exists("/README (2).md") || string(srv.Data("/README.md")) != orig {
		t.Fatal("skipping uploaded")
	}

	a.upload([]string{local}, "/")
	waitUntil(t, tt, "the question", func() bool { return a.dlg.open && a.dlg.kind == dlgConflict })
	if err := tt.Click("替换"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, tt, "the replacement", func() bool { return string(srv.Data("/README.md")) == "新的说明" })
	if srv.Exists("/README (2).md") {
		t.Error("replacing kept both")
	}
	if a.dlg.open {
		t.Error("the dialog stays open")
	}
}

// TestCopyIntoItsFolder copies a file beside itself under a free name,
// with no question.
func TestCopyIntoItsFolder(t *testing.T) {
	a, tt, srv := signedIn(t)
	a.ask(dlgCopy, "/", []baidu.File{fileNamed(t, a, "/", "README.md")})
	tt.Advance(400 * time.Millisecond)
	if err := tt.Click("复制到这里"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, tt, "the copy", func() bool { return srv.Exists("/README (1).md") })
	settle(tt)
	if a.dlg.open {
		t.Errorf("dialog open, %v: %q", a.dlg.kind, a.dlg.err)
	}
}

// TestMoveReplace moves a file onto one of its name, which goes to the
// recycle bin.
func TestMoveReplace(t *testing.T) {
	a, tt, srv := signedIn(t)
	srv.Add(fakepan.Node{Path: "/代码/README.md", Data: []byte("旧的")})
	a.ask(dlgMove, "/", []baidu.File{fileNamed(t, a, "/", "README.md")})
	tt.Advance(400 * time.Millisecond)
	tt.Click("打开 代码")
	tt.Advance(300 * time.Millisecond)
	if err := tt.Click("移动到这里"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, tt, "the question", func() bool { return a.dlg.kind == dlgConflict })
	if err := tt.Click("替换"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, tt, "the move", func() bool { return !srv.Exists("/README.md") })
	if string(srv.Data("/代码/README.md")) == "旧的" {
		t.Error("not replaced")
	}
}
