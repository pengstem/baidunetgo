package app

import (
	"math"

	"github.com/egoist/mygo/ui"
)

// The grid's measures. A slot is a tile with half the space between tiles
// around it: at least tileMinW+2*tileInsetX wide, gridPitch high.
const (
	tileMinW   = 152
	tileInsetX = 2
	tileInsetY = 5
	gridPitch  = 192
	gridPadTop = 7
	gridPadX   = 14
	gridPadEnd = 21
)

// gridState is where a listing's grid is: its scrolling, its columns, and
// where each tile showed in the last frame.
//
// The grid lays its tiles out itself rather than with ui.GridView, which
// builds each line of tiles as an element of its own: a tile a new number
// of columns moves to another line is then another element, built anew,
// and jumps there. Here every tile is a child of one element, keyed by its
// file, and glides from where it showed to its new place.
type gridState struct {
	scroll ui.ScrollState
	cols   int
	pivot  int // where Shift chooses from
	// spots are where the tiles of the last frame showed, by file, and
	// built those of this frame.
	spots, built map[int64]gridSpot
}

// gridSpot is the place of a tile: its cell, and where it showed, in the
// grid's viewport, glide included.
type gridSpot struct {
	col, row int
	x, y     float32
}

// gridCols is how many columns tiles take in a grid w wide.
func gridCols(w float32) int {
	return max(1, int(math.Floor(float64(w/(tileMinW+2*tileInsetX)))))
}

// tileGrid shows the files of l as tiles, built only as they show, with
// tile(i, e) filling the tile of file i: a click chooses it, as the arrows
// do in two dimensions while the grid has the focus, Cmd and Shift choose
// several, and a double click or Enter opens it.
func (a *App) tileGrid(c *ui.Context, l *listing, tile func(i int, e ui.Element)) {
	g := &l.grid
	n := len(l.files)
	grid := ui.Scroll(c.Key("grid")).Grow(1).Padding(gridPadTop, gridPadX, gridPadEnd).
		TrackScroll(&g.scroll).Focusable().FocusRing(false).Role(ui.RoleList).Label("文件")
	// The width the tiles had in the last frame, which the layout will
	// keep but for a resize; a resize that brings another number of
	// columns builds the frame after it again.
	box := grid.LayoutBounds()
	width, viewH := box.W-2*gridPadX, box.H
	if box.W > 0 {
		a.gridSize = [2]float32{width, viewH}
	} else {
		width, viewH = a.gridSize[0], a.gridSize[1]
	}
	if viewH <= 0 {
		viewH = 2000
	}
	cols := gridCols(width)
	grid.Draw(func(_ *ui.Painter, r ui.Rect) {
		if gridCols(r.W-2*gridPadX) != cols {
			c.Invalidate()
		}
	})
	if g.cols > 0 && cols != g.cols {
		// Keep the first line in view on the same tiles.
		y := max(g.scroll.Y-gridPadTop, 0)
		row := int(y / gridPitch)
		g.scroll.Y = gridPadTop + float32(row*g.cols/cols)*gridPitch + (y - float32(row)*gridPitch)
	}
	g.cols = cols
	rows := (n + cols - 1) / cols
	first := max(0, int((g.scroll.Y-gridPadTop)/gridPitch)-1)
	last := min(rows, int((g.scroll.Y+viewH)/gridPitch)+2)
	if g.built == nil {
		g.built = map[int64]gridSpot{}
	}
	pitchX := width / float32(cols)
	grid.Children(func() {
		ui.Box(c).FillWidth().Height(float32(rows) * gridPitch).Children(func() {
			for i := first * cols; i < min(n, last*cols); i++ {
				f := &l.files[i]
				col, row := i%cols, i/cols
				spot := gridSpot{col: col, row: row, x: float32(col) * pitchX, y: gridPadTop + float32(row)*gridPitch - g.scroll.Y}
				slot := ui.Column(c.Key(f.FsID)).Absolute().Top(float32(row)*gridPitch).Height(gridPitch).
					LeftPercent(float32(col)*100/float32(cols)).WidthPercent(100/float32(cols)).
					Padding(tileInsetY, tileInsetX)
				// A tile in another cell than in the last frame starts from
				// where it showed, and glides to its place.
				if was, ok := g.spots[f.FsID]; ok && (was.col != col || was.row != row) {
					slot.Jump("dx", was.x-spot.x)
					slot.Jump("dy", was.y-spot.y)
				}
				dx, dy := slot.AnimateTo("dx", 0, motionPanel), slot.AnimateTo("dy", 0, motionPanel)
				if dx != 0 || dy != 0 {
					slot.Translate(dx, dy)
				}
				spot.x, spot.y = spot.x+dx, spot.y+dy
				g.built[f.FsID] = spot
				slot.Children(func() {
					e := ui.Column(c).Grow(1).Role(ui.RoleListItem).Label(f.Name)
					if e.Clicked() {
						g.click(l, i, e.ClickModifiers())
						grid.Focus()
					}
					if e.DoubleClicked() {
						a.open(*f)
					}
					tile(i, e)
				})
			}
		})
	})
	g.spots, g.built = g.built, g.spots
	clear(g.built)
	if to, extend, ok := g.keys(grid, l, n); ok {
		if extend {
			g.extend(l, to)
		} else {
			g.choose(l, to)
		}
		g.scrollIntoView(to, viewH)
	}
	if grid.Shortcut(ui.Cmd, ui.KeyA) {
		for _, f := range l.files {
			l.sel.Add(f.FsID)
		}
	}
	if grid.Shortcut(0, ui.KeyEnter) && l.selected >= 0 && l.selected < n {
		a.open(l.files[l.selected])
	}
}

// click chooses tile i as a click with mods does: alone, Cmd adding it or
// taking it out, Shift choosing those from the one last chosen.
func (g *gridState) click(l *listing, i int, mods ui.Modifiers) {
	switch {
	case mods&ui.Shift != 0:
		g.extend(l, i)
	case mods&ui.Cmd != 0:
		if id := l.files[i].FsID; l.sel.Has(id) {
			l.sel.Remove(id)
		} else {
			l.sel.Add(id)
		}
		l.selected, g.pivot = i, i
	default:
		g.choose(l, i)
	}
}

// choose chooses tile i alone.
func (g *gridState) choose(l *listing, i int) {
	l.sel.Clear()
	l.sel.Add(l.files[i].FsID)
	l.selected, g.pivot = i, i
}

// extend chooses the tiles from the pivot to i.
func (g *gridState) extend(l *listing, i int) {
	p := g.pivot
	if p < 0 || p >= len(l.files) || l.sel.Len() == 0 {
		g.choose(l, i)
		return
	}
	l.sel.Clear()
	for j := min(p, i); j <= max(p, i); j++ {
		l.sel.Add(l.files[j].FsID)
	}
	l.selected = i
}

// keys returns the tile the arrows, Home and End move the choice to while
// the grid has the focus, and whether Shift extends the choice to it.
func (g *gridState) keys(grid ui.Element, l *listing, n int) (to int, extend, ok bool) {
	if n == 0 {
		return 0, false, false
	}
	cols, at := g.cols, l.selected
	in := at >= 0 && at < n
	to = -1
	for _, m := range []struct {
		key ui.Key
		d   int
	}{{ui.KeyRight, 1}, {ui.KeyLeft, -1}, {ui.KeyDown, cols}, {ui.KeyUp, -cols}} {
		plain, shift := grid.Shortcut(0, m.key), grid.Shortcut(ui.Shift, m.key)
		if !plain && !shift {
			continue
		}
		extend = shift
		switch {
		case !in:
			to = 0
		case at+m.d >= 0 && at+m.d < n:
			to = at + m.d
		case m.d == cols && at/cols < (n-1)/cols:
			to = n - 1 // the last line, shorter
		}
	}
	if grid.Shortcut(0, ui.KeyHome) {
		to, extend = 0, false
	}
	if grid.Shortcut(0, ui.KeyEnd) {
		to, extend = n-1, false
	}
	if extend && (g.pivot < 0 || !in) {
		g.pivot = max(at, 0)
	}
	return to, extend, to >= 0
}

// scrollIntoView scrolls the grid as little as shows tile i in a viewport
// viewH high.
func (g *gridState) scrollIntoView(i int, viewH float32) {
	top := gridPadTop + float32(i/max(g.cols, 1))*gridPitch
	switch {
	case top < g.scroll.Y:
		g.scroll.Y = top
	case top+gridPitch > g.scroll.Y+viewH:
		g.scroll.Y = top + gridPitch - viewH
	}
}
