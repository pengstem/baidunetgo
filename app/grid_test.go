package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/fakepan"
)

// TestGridReflow resizes the window across a number of columns: the tiles
// that change lines glide from where they were, keep being chosen, and the
// first line stays on the same tiles.
func TestGridReflow(t *testing.T) {
	a, tt, srv := signedIn(t)
	var names []string
	for i := 1; i <= 30; i++ {
		n := fmt.Sprintf("Show.S01E%02d.mkv", i)
		names = append(names, n)
		srv.Add(fakepan.Node{Path: "/剧集/" + n, Category: 1, Size: 1 << 30})
	}
	a.gridView = true
	a.router.Push(filesRoute("/剧集"))
	waitFor(t, tt, names[0])
	tt.Advance(time.Second)
	cols := a.listingFor("/剧集").grid.cols
	// The first tile of the second line moves up to the end of the first
	// once the window is wider by a column.
	moved := names[cols]
	tt.Click(moved)
	tt.Advance(time.Second)
	from, _ := tt.Find(moved)
	tt.SetSize(1180+160, 760)
	tt.Frame()
	tt.Frame()
	l := a.listingFor("/剧集")
	if l.grid.cols != cols+1 {
		t.Fatalf("%d columns after widening, want %d", l.grid.cols, cols+1)
	}
	var path []float32
	for range 8 {
		tt.Advance(30 * time.Millisecond)
		r, _ := tt.Find(moved)
		path = append(path, r.Y)
	}
	tt.Advance(time.Second)
	to, _ := tt.Find(moved)
	save(t, tt, "grid-reflow")
	if to.Y >= from.Y {
		t.Fatalf("%s at %.0f, not moved up from %.0f", moved, to.Y, from.Y)
	}
	between := 0
	for _, y := range path {
		if y < from.Y-1 && y > to.Y+1 {
			between++
		}
	}
	if between < 2 {
		t.Errorf("%s jumped from %.0f to %.0f instead of gliding: %v", moved, from.Y, to.Y, path)
	}
	if !l.sel.Has(l.files[cols].FsID) || l.sel.Len() != 1 {
		t.Errorf("choice lost after the resize")
	}
}

// TestGridKeys moves the choice with the arrows, extends it with Shift,
// scrolls to the last tile with End and opens a folder with Enter.
func TestGridKeys(t *testing.T) {
	a, tt, srv := signedIn(t)
	for i := 1; i <= 60; i++ {
		srv.Add(fakepan.Node{Path: fmt.Sprintf("/剧集/Show.S01E%02d.mkv", i), Category: 1, Size: 1 << 30})
	}
	srv.Add(fakepan.Node{Path: "/剧集/附件", IsDir: true})
	a.gridView = true
	a.router.Push(filesRoute("/剧集"))
	waitFor(t, tt, "Show.S01E01.mkv")
	tt.Advance(time.Second)
	l := a.listingFor("/剧集")
	cols := l.grid.cols
	tt.Click(l.files[0].Name)
	tt.Key(0, keyRight)
	tt.Key(ui.Shift, ui.KeyDown)
	tt.Advance(100 * time.Millisecond)
	if l.selected != 1+cols || l.sel.Len() != cols+1 {
		t.Errorf("at %d with %d chosen, want %d with %d", l.selected, l.sel.Len(), 1+cols, cols+1)
	}
	tt.Key(0, ui.KeyEnd)
	tt.Advance(time.Second)
	last := l.files[len(l.files)-1].Name
	if _, ok := tt.Find(last); !ok || l.sel.Len() != 1 {
		t.Fatalf("End did not show %s alone", last)
	}
	// Folders sort first: Home goes to 附件, whose name Enter edits, as in
	// Finder, and which Cmd-Down opens.
	tt.Key(0, ui.KeyHome)
	tt.Key(0, ui.KeyEnter)
	tt.Advance(100 * time.Millisecond)
	if r := a.renaming; enterRenames && (r == nil || r.draft != "附件") {
		t.Fatalf("Enter edited %+v", r)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(100 * time.Millisecond)
	tt.Key(ui.Cmd, ui.KeyDown)
	tt.Advance(time.Second)
	if p := a.router.Path(); p != "/files/剧集/附件" {
		t.Errorf("Enter went to %s", p)
	}
}
