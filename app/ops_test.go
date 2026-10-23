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

}

// TestRenameInPlace: the name is edited where it shows, as in Finder: F2,
// Enter, the menu or a slow second click start it, Enter or a click away
// keeps the name, Escape goes back, and a double click still opens.
func TestRenameInPlace(t *testing.T) {
	a, tt, srv := signedIn(t)
	defer func(was bool) { enterRenames = was }(enterRenames)
	enterRenames = true
	editing := func() bool { on, _ := tt.TextInputActive(); return on }
	tt.Advance(time.Second)
	if err := tt.Click("README.md"); err != nil {
		t.Fatal(err)
	}
	// A click choosing a file edits nothing, however long after.
	tt.Advance(time.Second)
	if editing() {
		t.Fatal("a click choosing a row edited its name")
	}
	tt.Key(0, ui.KeyF2)
	tt.Advance(300 * time.Millisecond)
	if !editing() || a.dlg.open {
		t.Fatalf("F2 did not edit the name in place: dialog %v", a.dlg.open)
	}
	save(t, tt, "rename-in-place")
	// The name without its extension is chosen: typing replaces it.
	tt.Type("说明")
	tt.Key(0, ui.KeyEnter)
	tt.Advance(16 * time.Millisecond)
	if editing() || !tt.HasText("说明.md") {
		t.Fatalf("Enter did not keep the name: %q", tt.Texts())
	}
	settle(tt)
	if !srv.Exists("/说明.md") || srv.Exists("/README.md") {
		t.Fatal("not renamed on the netdisk")
	}

	// A second click on the name of the file chosen edits it, once a
	// double click is out of reach; Escape goes back.
	clickName := func(name string) {
		t.Helper()
		r, ok := tt.Find(name)
		if !ok {
			t.Fatalf("no %s", name)
		}
		tt.ClickAt(r.X+60, r.Y+r.H/2)
	}
	clickName("说明.md")
	tt.Advance(200 * time.Millisecond)
	if editing() {
		t.Fatal("editing before the double click time")
	}
	tt.Advance(400 * time.Millisecond)
	if !editing() {
		t.Fatalf("a slow click did not edit the name: %+v %+v", a.renaming, a.slow)
	}
	tt.Type("别的")
	tt.Key(0, ui.KeyEscape)
	tt.Advance(100 * time.Millisecond)
	if editing() || !tt.HasText("说明.md") {
		t.Fatalf("Escape did not go back: %q", tt.Texts())
	}

	// Enter edits, the list having the focus back; a name the netdisk
	// would refuse keeps it editing; a click away keeps the name typed.
	tt.Key(0, ui.KeyEnter)
	tt.Advance(100 * time.Millisecond)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("a/b")
	tt.Key(0, ui.KeyEnter)
	tt.Advance(100 * time.Millisecond)
	if !editing() || !tt.HasText(`名称不能包含 / \ : * ? " < > |`) {
		t.Fatalf("a name with a slash ended editing without a word: %v %+v %q", editing(), a.renaming, tt.Texts())
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("读我.md")
	if err := tt.Click("安装包.dmg"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if editing() || !srv.Exists("/读我.md") {
		t.Fatalf("a click away did not keep the name: %+v", a.renaming)
	}

	// A double click opens, and edits nothing.
	tt.Click("读我.md")
	tt.Advance(time.Second)
	tt.Click("读我.md")
	tt.Click("读我.md")
	tt.Advance(time.Second)
	if editing() || a.pv == nil {
		t.Fatalf("a double click edited %v, previewed %v", editing(), a.pv != nil)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(time.Second)

	// From the selection bar, in the list.
	if err := tt.Click("重命名"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	if a.renaming == nil || !editing() {
		t.Fatal("the selection bar did not edit the name")
	}
	tt.Type("说明")
	tt.Key(0, ui.KeyEnter)
	settle(tt)
	if !srv.Exists("/说明.md") {
		t.Fatal("not renamed from the selection bar")
	}

	// In the grid: from the menu, Enter, and a slow click.
	a.gridView = true
	tt.Advance(time.Second)
	if err := tt.RightClick("安装包.dmg"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("重命名"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	save(t, tt, "rename-in-grid")
	tt.Type("安装器")
	tt.Key(0, ui.KeyEnter)
	settle(tt)
	if a.renaming != nil || !srv.Exists("/安装器.dmg") {
		t.Fatalf("not renamed from the grid: %+v", a.renaming)
	}
	tt.Key(0, ui.KeyEnter)
	tt.Advance(100 * time.Millisecond)
	if a.renaming == nil {
		t.Fatal("Enter did not edit the tile")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(100 * time.Millisecond)
	if a.renaming != nil || !srv.Exists("/安装器.dmg") {
		t.Fatal("Escape did not go back in the grid")
	}
	r, _ := tt.Find("安装器.dmg")
	tt.ClickAt(r.X+r.W/2, r.Y+138) // its name, under the thumbnail
	tt.Advance(600 * time.Millisecond)
	if a.renaming == nil {
		t.Fatal("a slow click did not edit the tile")
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
	if a.renaming == nil {
		t.Error("the icon did not rename")
	}
}

// TestShortcuts: Space looks at the file chosen and closes it again,
// Cmd-1 and Cmd-2 switch the view, Cmd-Up goes up, Cmd-Down opens,
// Cmd-F finds, Cmd-comma opens the settings.
func TestShortcuts(t *testing.T) {
	a, tt, _ := signedIn(t)
	a.router.Push(filesRoute("/照片"))
	waitFor(t, tt, "IMG_2310.jpg")
	l := a.listings["/照片"]
	l.sel.Add(fileNamed(t, a, "/照片", "IMG_2311.jpg").FsID)
	tt.Advance(100 * time.Millisecond)
	tt.Key(0, ui.KeySpace)
	tt.Advance(300 * time.Millisecond)
	if a.pv == nil || a.pv.file.Name != "IMG_2311.jpg" {
		t.Fatal("Space did not preview")
	}
	tt.Key(0, ui.KeySpace)
	tt.Advance(500 * time.Millisecond)
	if a.pv != nil {
		t.Fatalf("Space did not close the preview; focus %v", tt.Texts()[:3])
	}
	tt.Key(ui.Cmd, ui.Key2)
	tt.Advance(100 * time.Millisecond)
	if !a.gridView {
		t.Error("Cmd-2 kept the list")
	}
	tt.Key(ui.Cmd, ui.Key1)
	tt.Advance(100 * time.Millisecond)
	if a.gridView {
		t.Error("Cmd-1 kept the grid")
	}
	l.sel.Clear()
	l.sel.Add(fileNamed(t, a, "/照片", "京都").FsID)
	tt.Advance(100 * time.Millisecond)
	tt.Key(ui.Cmd, ui.KeyDown)
	waitFor(t, tt, "kyoto-1.png")
	tt.Key(ui.Cmd, ui.KeyUp)
	tt.Advance(500 * time.Millisecond)
	tt.Key(ui.Cmd, ui.KeyUp)
	tt.Advance(500 * time.Millisecond)
	if a.currentDir() != "/" {
		t.Errorf("Cmd-Up went to %q", a.currentDir())
	}
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Advance(100 * time.Millisecond)
	tt.Type("img_2310")
	tt.Key(0, ui.KeyEnter)
	waitFor(t, tt, "找到 1 个结果")
	tt.Key(ui.Cmd, ui.KeyComma)
	tt.Advance(500 * time.Millisecond)
	if a.router.Path() != "/settings" {
		t.Errorf("Cmd-comma went to %q", a.router.Path())
	}
}
