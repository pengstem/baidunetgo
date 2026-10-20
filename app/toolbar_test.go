package app

import (
	"slices"
	"testing"
	"time"
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
