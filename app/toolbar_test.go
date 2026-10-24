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

// pathWide is a window the folder's path and tools both fit in, whatever
// the font; pathNarrowest is the narrowest window the app allows (its
// MinWidth in main.go), below which the title bar has no room to fold into.
const (
	pathWide      = 2000
	pathNarrowest = 780
)

// TestPathFolds folds the folders above a deep folder with long names into
// "…" in a narrow window, keeping the folder's own name, and goes to one
// of them from its menu. The tools fold before the path does, and open
// again only when the path shows whole beside them. Where each happens
// depends on the font, so the test finds those widths rather than assuming
// them.
func TestPathFolds(t *testing.T) {
	a, tt, srv := signedIn(t)
	dir := "/数学建模美赛资料合集（2026版）/历年真题与优秀论文完整汇编/附录与评审标准说明文件夹"
	srv.Add(fakepan.Node{Path: dir + "/说明.txt", Data: []byte("hi")})
	a.router.Push(filesRoute(dir))
	waitFor(t, tt, "说明.txt")

	if folded, tools := foldAt(a, tt, pathWide); folded || tools {
		t.Fatalf("at %d px the path folded (%v) or the tools did (%v)", pathWide, folded, tools)
	}
	save(t, tt, "path-wide")

	// Both fold only as the window narrows, so the narrowest width at which
	// the path shows whole, and the narrowest at which the tools stay open,
	// are where a search finds them.
	whole := firstWidth(pathNarrowest, pathWide, func(w int) bool {
		folded, _ := foldAt(a, tt, w)
		return !folded
	})
	open := firstWidth(pathNarrowest, pathWide, func(w int) bool {
		_, tools := foldAt(a, tt, w)
		return !tools
	})
	if whole > open {
		t.Fatalf("at %d px the path folded before the tools did", whole-1)
	}

	// Just narrower than the path shows whole, the path and the tools have
	// both folded.
	narrow := whole - 1
	if folded, tools := foldAt(a, tt, narrow); !folded || !tools {
		t.Errorf("at %d px the path folded (%v) and the tools did (%v), want both", narrow, folded, tools)
	}
	save(t, tt, "path-narrow")

	// Wide again, the folded tools open.
	tt.SetSize(pathWide, 760)
	tt.Advance(time.Second)
	if a.compact {
		t.Errorf("at %d px the tools stayed folded with room for them and the path", pathWide)
	}

	// Where the tools open, the path shows whole beside them.
	if folded, tools := foldAt(a, tt, open); folded || tools {
		t.Errorf("at %d px the path folded (%v) or the tools did (%v)", open, folded, tools)
	}

	// At the narrowest width the path is folded, the folder's own name
	// shows, and the "…" menu leads with the folder above it.
	if folded, _ := foldAt(a, tt, pathNarrowest); !folded {
		t.Fatalf("at %d px the path did not fold", pathNarrowest)
	}
	save(t, tt, "path-narrowest")
	if !tt.HasText(path.Base(dir)) {
		t.Errorf("the folder's name is gone; texts %q", tt.Texts())
	}
	if err := tt.Click("上层文件夹"); err != nil {
		t.Fatal(err)
	}
	parent := path.Base(path.Dir(dir))
	m := tt.Menu()
	if len(m) == 0 || m[0] != parent {
		t.Fatalf("menu %q, want %q first", m, parent)
	}
	if err := tt.ChooseMenuItem(m[0]); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if want := path.Dir(dir); a.currentDir() != want {
		t.Errorf("dir %q, want %q", a.currentDir(), want)
	}
}

// foldAt sizes the window to w, after a wide one so that the folds are
// those w alone gives, and reports whether the path folded and the tools
// did.
func foldAt(a *App, tt *ui.Tester, w int) (pathFolded, toolsFolded bool) {
	tt.SetSize(pathWide, 760)
	tt.Advance(time.Second)
	tt.SetSize(w, 760)
	tt.Advance(time.Second)
	_, ok := tt.Find("上层文件夹")
	return ok, a.compact
}

// firstWidth returns the narrowest width in [lo, hi] at which ok holds, for
// ok holding at hi and holding at every width above some width.
func firstWidth(lo, hi int, ok func(w int) bool) int {
	for lo < hi {
		mid := (lo + hi) / 2
		if ok(mid) {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo
}
