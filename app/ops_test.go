package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/fakepan"
)

// settle builds frames until the netdisk has answered and animations end.
func settle(tt *ui.Tester) {
	for range 20 {
		time.Sleep(15 * time.Millisecond)
		tt.Advance(60 * time.Millisecond)
	}
}

func TestNewFolderAndRename(t *testing.T) {
	a, tt, srv := signedIn(t)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyN)
	tt.Advance(300 * time.Millisecond)
	if a.dlg.kind != dlgNewFolder || !a.dlg.open {
		t.Fatalf("dialog %+v", a.dlg.kind)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("相册")
	tt.Key(0, ui.KeyEnter)
	settle(tt)
	if !srv.Exists("/相册") || !tt.HasText("相册") {
		t.Fatalf("folder not made; texts %q", tt.Texts())
	}

	a.ask(dlgRename, "/", []baidu.File{fileNamed(t, a, "/", "README.md")})
	tt.Advance(300 * time.Millisecond)
	save(t, tt, "dialog-rename")
	// The name without its extension is chosen: typing replaces it.
	tt.Type("说明")
	tt.Key(0, ui.KeyEnter)
	settle(tt)
	if !srv.Exists("/说明.md") {
		t.Errorf("rename failed: %q", a.dlg.err)
	}
}

func TestDeleteUndoAndRecycle(t *testing.T) {
	a, tt, srv := signedIn(t)
	f := fileNamed(t, a, "/", "安装包.dmg")
	l := a.listings["/"]
	l.sel.Add(f.FsID)
	tt.Advance(500 * time.Millisecond)
	save(t, tt, "selection-bar")
	if !tt.HasText("已选 1 项") {
		t.Fatalf("selection bar missing: %q", tt.Texts())
	}
	// Delete goes at once, with no question: it can be undone.
	tt.Key(0, ui.KeyDelete)
	if a.dlg.open {
		t.Fatal("delete asks first")
	}
	settle(tt)
	if srv.Exists("/安装包.dmg") {
		t.Fatal("not deleted")
	}
	if err := tt.Click("撤销"); err != nil {
		t.Fatalf("no undo: %v; %q", err, tt.Texts())
	}
	settle(tt)
	if !srv.Exists("/安装包.dmg") {
		t.Fatal("undo did not restore")
	}

	// Delete again and restore from the recycle bin page.
	a.remove("/", []baidu.File{fileNamed(t, a, "/", "安装包.dmg")})
	settle(tt)
	a.router.Push("/recycle")
	waitFor(t, tt, "安装包.dmg")
	tt.Advance(500 * time.Millisecond)
	save(t, tt, "recycle")
	if err := tt.Click("还原"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !srv.Exists("/安装包.dmg") || len(a.recycle.items) != 0 {
		t.Errorf("restore from bin failed")
	}
}

func TestMoveWithPicker(t *testing.T) {
	a, tt, srv := signedIn(t)
	a.ask(dlgMove, "/", []baidu.File{fileNamed(t, a, "/", "安装包.dmg")})
	tt.Advance(400 * time.Millisecond)
	save(t, tt, "dialog-move")
	if err := tt.Click("打开 代码"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	if a.dlg.picker.dir != "/代码" {
		t.Fatalf("picker in %q", a.dlg.picker.dir)
	}
	if err := tt.Click("移动到这里"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !srv.Exists("/代码/安装包.dmg") || srv.Exists("/安装包.dmg") {
		t.Errorf("move failed: %q", a.dlg.err)
	}
}

func TestSearch(t *testing.T) {
	a, tt, srv := signedIn(t)
	a.search("img_231")
	waitFor(t, tt, "找到 10 个结果")
	tt.Advance(500 * time.Millisecond)
	save(t, tt, "search")

	// From a folder, the search can keep to it.
	srv.Add(fakepan.Node{Path: "/文档/img_2319-副本.txt", Data: []byte("x")})
	a.router.Push(filesRoute("/照片"))
	waitFor(t, tt, "IMG_2310.jpg")
	a.search("img_231")
	waitFor(t, tt, "找到 11 个结果")
	if err := tt.Click("「照片」"); err != nil {
		t.Fatalf("no scope switch: %v; %q", err, tt.Texts())
	}
	waitFor(t, tt, "在「照片」中找到 10 个结果")
	tt.Advance(500 * time.Millisecond)
	save(t, tt, "search-in-folder")
}

// TestNarrowSelectionBar keeps the selection bar on one line in the
// narrowest window: its actions drop their labels for icons, which still
// act.
func TestNarrowSelectionBar(t *testing.T) {
	a, tt, _ := signedIn(t)
	l := a.listings["/"]
	l.sel.Add(fileNamed(t, a, "/", "安装包.dmg").FsID)
	tt.Advance(time.Second)
	if r, _ := tt.Find("重命名"); r.W < 60 {
		t.Fatalf("no label at full width: %v", r)
	}
	tt.SetSize(780, 600)
	tt.Advance(time.Second)
	save(t, tt, "selection-bar-narrow")
	if r, _ := tt.Find("重命名"); r.W > 40 {
		t.Errorf("a label in a narrow window: %v", r)
	}
	if r, ok := tt.Find("已选 1 项"); !ok || r.H > 20 {
		t.Errorf("count wraps: %v", r)
	}
	if err := tt.Click("重命名"); err != nil {
		t.Fatal(err)
	}
	if !a.dlg.open || a.dlg.kind != dlgRename {
		t.Error("the icon did not rename")
	}
}
