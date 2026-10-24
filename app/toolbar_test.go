package app

import (
	"path"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/fakepan"
)

// TestSortMenu sorts the grid from the title bar's menu.
func TestSortMenu(t *testing.T) {
	a, tt, _ := signedIn(t)
	a.gridView = true
	tt.Advance(time.Second)
	if err := tt.Click("排序方式"); err != nil {
		t.Fatal(err)
	}
	if m := tt.Menu(); !slices.Contains(m, "修改日期") || !slices.Contains(m, "降序") {
		t.Fatalf("menu %q", m)
	}
	if err := tt.ChooseMenuItem("大小"); err != nil {
		t.Fatal(err)
	}
	if a.order.By != "size" || !a.order.Desc {
		t.Errorf("order %+v", a.order)
	}
	tt.Click("排序方式")
	if err := tt.ChooseMenuItem("升序"); err != nil {
		t.Fatal(err)
	}
	if a.order.By != "size" || a.order.Desc {
		t.Errorf("order %+v", a.order)
	}
}

// TestCompactToolbar folds the view switch into a pop-up button in a
// narrow window, which still switches the view.
func TestCompactToolbar(t *testing.T) {
	a, tt, _ := signedIn(t)
	if a.compact {
		t.Fatal("compact at full width")
	}
	tt.SetSize(760, 600)
	tt.Advance(time.Second)
	if !a.compact {
		t.Fatal("not compact in a narrow window")
	}
	save(t, tt, "compact-files")
	// The search field folded to a round button, which opens it.
	r, _ := tt.Find("搜索网盘")
	tt.ClickAt(r.X, r.Y+r.H/2)
	tt.Advance(time.Second)
	if !tt.Focused("搜索网盘") {
		t.Error("a click on the folded search did not open it")
	}
	if r, _ := tt.Find("搜索网盘"); r.W < 100 {
		t.Errorf("the open search field is %v wide", r.W)
	}
	tt.Key(0, ui.KeyEscape)
	tt.ClickAt(600, 400)
	tt.Advance(time.Second)
	if err := tt.Click("视图"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("网格"); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	tt.Advance(time.Second)
	if !a.gridView {
		t.Error("the menu did not switch to the grid")
	}
	a.router.Push("/transfers")
	tt.Advance(time.Second)
	save(t, tt, "compact-transfers")
	if err := tt.Click("进行中 0"); err != nil {
		t.Fatal(err)
	}
	if m := tt.Menu(); len(m) != 2 {
		t.Fatalf("menu %q", m)
	}
	tt.ChooseMenuItem(tt.Menu()[1])
	if a.transfers.tab != 1 {
		t.Errorf("tab %d", a.transfers.tab)
	}
}

// TestPathFolds folds the folders above a deep folder with long names into
// "…" in a narrow window, keeping the folder's own name, and goes to one
// of them from its menu.
func TestPathFolds(t *testing.T) {
	a, tt, srv := signedIn(t)
	dir := "/数学建模美赛资料合集（2026版）/历年真题与优秀论文完整汇编/附录与评审标准说明文件夹"
	srv.Add(fakepan.Node{Path: dir + "/说明.txt", Data: []byte("hi")})
	a.router.Push(filesRoute(dir))
	waitFor(t, tt, "说明.txt")
	tt.SetSize(1500, 760)
	tt.Advance(time.Second)
	if _, ok := tt.Find("上层文件夹"); ok {
		t.Error("the path folded in a wide window")
	}
	save(t, tt, "path-wide")
	tt.SetSize(760, 600)
	tt.Advance(time.Second)
	save(t, tt, "path-narrow")
	if !tt.HasText("附录与评审标准说明文件夹") {
		t.Errorf("the folder's name is gone; texts %q", tt.Texts())
	}
	if err := tt.Click("上层文件夹"); err != nil {
		t.Fatal(err)
	}
	m := tt.Menu()
	if len(m) == 0 || m[0] != "历年真题与优秀论文完整汇编" {
		t.Fatalf("menu %q", m)
	}
	if err := tt.ChooseMenuItem(m[0]); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if want := path.Dir(dir); a.currentDir() != want {
		t.Errorf("dir %q, want %q", a.currentDir(), want)
	}
	tt.SetSize(560, 600)
	tt.Advance(time.Second)
	save(t, tt, "path-narrower")
}
