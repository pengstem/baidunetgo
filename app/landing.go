package app

import (
	"path"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// Files come into a folder from where they were put: the rows after the
// place each goes make room for it, and it flies there from where it was
// dropped, from the button that picked it or made it, or, a folder made,
// from where it was named. It waits for the room (motionLand's delay), so
// the two do not cross.
//
// What flies is a card over the page (flight), not the row: a list clips
// its rows to their places, so a row coming from elsewhere would show
// only once it got there. The row stays hidden while its card is on the
// way, and shows as it lands, where the card is.

// flight is a file's card on its way into its folder: waiting where it
// was put until the folder's listing has the file, then flying to its row
// or tile.
type flight struct {
	dir  string
	file baidu.File // what the card shows
	from ui.Rect    // where it was put, in the window
	to   ui.Rect    // its row or tile, as the frame before showed it
	born time.Time  // when it was put, set by the first frame to show it
	at   time.Time  // when it set off, zero while it waits
}

// flightWait is how long a card waits for its file to show, as for an
// upload a conflict holds up, before it fades; flightCards how many cards
// a drop shows, one over another, the files after them just coming in.
const (
	flightWait  = 8 * time.Second
	flightCards = 5
)

// draftKey keys the flight of the folder being named in dir, whose path
// changes as it is named.
func draftKey(dir string) string { return "\x00draft:" + dir }

// flightKey keys the flight to f's row or tile in l.
func flightKey(l *listing, f *baidu.File) string {
	if f.FsID == draftID {
		return draftKey(l.dir)
	}
	return f.Path
}

// fly starts a card of f on its way into dir from a box in the window.
func (a *App) fly(dir, key string, f baidu.File, from ui.Rect) {
	if from.W <= 0 {
		return
	}
	if a.flights == nil {
		a.flights = map[string]*flight{}
	}
	a.flights[key] = &flight{dir: dir, file: f, from: from}
}

// land starts files from local paths on their way into dir, their cards
// centered on a box in the window.
func (a *App) land(dir string, locals []string, from ui.Rect) {
	for i, local := range locals[:min(len(locals), flightCards)] {
		name := filepath.Base(local)
		k := float32(i) * 6
		box := ui.Rect{X: from.X + from.W/2 - 110 + k, Y: from.Y + from.H/2 - 15 + k, W: 220, H: 30}
		f := baidu.File{Name: name, Path: path.Join(dir, name), IsDir: path.Ext(name) == ""}
		a.fly(dir, f.Path, f, box)
	}
}

// followFlights sets off the cards whose files l now has, scrolling to
// them, and forgets those landed a while ago or that waited too long.
func (a *App) followFlights(l *listing, now time.Time) {
	for key, fl := range a.flights {
		if fl.dir != l.dir {
			continue
		}
		if fl.born.IsZero() {
			fl.born = now
		}
		if fl.at.IsZero() {
			if i := l.indexKey(key); i >= 0 {
				fl.at = now
				if a.gridView {
					l.grid.scrollIntoView(i, a.gridSize[1])
				} else {
					l.list.ScrollIntoView(i)
				}
			} else if now.Sub(fl.born) > flightWait {
				delete(a.flights, key)
			}
			continue
		}
		if now.Sub(fl.at) > motionLand.Delay+motionLand.Duration+300*time.Millisecond {
			delete(a.flights, key)
		}
	}
}

// indexKey returns where the row of a flight's key is in l, -1 if none.
func (l *listing) indexKey(key string) int {
	if key == draftKey(l.dir) {
		return l.index(draftID)
	}
	for i := range l.files {
		if l.files[i].Path == key && l.files[i].FsID != draftID {
			return i
		}
	}
	return -1
}

// progress is how far the card is on its way, from 0 to 1, eased: 0
// while it waits and the rows make room.
func (fl *flight) progress(now time.Time) float32 {
	if fl.at.IsZero() || fl.to.W <= 0 {
		return 0
	}
	if abs32(fl.from.X-fl.to.X) < 2 && abs32(fl.from.Y-fl.to.Y) < 2 {
		return 1 // there already, as a folder made where it was named
	}
	return easeStandard(since(now, fl.at.Add(motionLand.Delay), motionLand.Duration))
}

// awaited hides e, the row or tile of f, while its card is on the way
// there, and tells the card where it is.
func (a *App) awaited(e ui.Element, l *listing, f *baidu.File, now time.Time) {
	fl := a.flights[flightKey(l, f)]
	if fl == nil || fl.at.IsZero() {
		return
	}
	if b := e.Bounds(); b.W > 0 {
		fl.to = b
	}
	if fl.to.W <= 0 || fl.progress(now) < 1 {
		e.Opacity(0)
	}
}

// flightCards shows the cards of the files on their way into l's folder,
// over the page whose box is page.
func (a *App) flightCards(c *ui.Context, t *ui.Theme, p *palette, l *listing, page ui.Rect) {
	now := c.Now()
	for key, fl := range a.flights {
		if fl.dir != l.dir {
			continue
		}
		k := fl.progress(now)
		if k >= 1 {
			continue
		}
		c.Invalidate()
		box := fl.from
		if k > 0 {
			box = ui.Rect{X: lerp(fl.from.X, fl.to.X, k), Y: lerp(fl.from.Y, fl.to.Y, k), W: lerp(fl.from.W, fl.to.W, k), H: lerp(fl.from.H, fl.to.H, k)}
		}
		f := fl.file
		if m := a.creatingIn(l); m != nil && key == draftKey(l.dir) {
			f.Name = m.draft
		}
		lift := 1 - k // the card settles into the page as it lands
		tile := a.gridView && box.H > 60
		var card ui.Element
		if tile {
			card = ui.Column(c.Key("flight:" + key)).Justify(ui.Center)
		} else {
			card = ui.Row(c.Key("flight:" + key))
		}
		card.Absolute().Left(box.X-page.X).Top(box.Y-page.Y).Width(box.W).Height(box.H).
			PaddingX(10).Gap(8).AlignItems(ui.Center).Radius(8).Clip().PassThrough().
			Background(p.Card).Border(1, p.Hairline.Alpha(lift)).Shadow(0, 8*lift, 24*lift, -8, p.Shadow.Alpha(lift)).TextColor(t.Text).
			Transition(ui.ElementTransition{Animation: &motionFade, Enter: &ui.Motion{Opacity: 0, Scale: 0.9}})
		card.Children(func() {
			if tile && f.IsDir {
				folderGlyph(c, p, lerp(18, 72, k))
			} else {
				a.fileIcon(c, t, p, &f, 18)
			}
			ui.Text(c, f.Name).Grow(1).Shrink(1).SingleLine()
			if !tile && box.W > 480 {
				// The row's columns, coming in as the card becomes it.
				ui.Text(c, kindLabel(&f)).Width(colType).TextColor(t.TextMuted).SingleLine().Opacity(k)
			}
		})
	}
}

func lerp(a, b, k float32) float32 { return a + (b-a)*k }

// rowTops returns where each row of l starts in its list, the rows closing
// up as they leave counted as they show.
func (a *App) rowTops(l *listing, now time.Time) map[int64]float32 {
	tops := make(map[int64]float32, len(l.files))
	y := float32(0)
	for _, f := range l.files {
		tops[f.FsID] = y
		h := float32(rowHeight)
		if d := a.dying[f.FsID]; d != nil && !d.at.IsZero() {
			h *= 1 - smoothstep(0.7, 1, since(now, d.at, dissolveFor))
		} else if g := a.gathers[f.Path]; g != nil && g.failed {
			h *= 1 - smoothstep(0.7, 1, since(now, g.ended, dissolveFor))
		}
		y += h
	}
	return tops
}

// rowGlide is a row moving to its place from by DIPs away, from at.
type rowGlide struct {
	from float32
	at   time.Time
}

// glideRow moves row r of file id from where it showed in the frame
// before to its place at top, when another row coming or going moved it,
// making room for it or closing up after it: rows closing up as one
// dissolves move with the layout. The row keeps no animation state of its
// own, which would take its name's clicks (clickName), so the listing
// holds the glides.
func glideRow(c *ui.Context, r ui.Element, l *listing, id int64, top float32, now time.Time) {
	g, gliding := l.glides[id]
	dy := float32(0)
	if gliding {
		k := since(now, g.at, motionPanel.Duration)
		if dy = g.from * (1 - easeStandard(k)); k >= 1 {
			delete(l.glides, id)
		}
	}
	if was, ok := l.rowsShown[id]; ok && abs32(was-top-dy) > rowHeight/2 {
		dy = was - top
		l.glides[id] = rowGlide{from: dy, at: now}
	}
	if dy != 0 {
		r.Translate(0, dy)
		c.Invalidate()
	}
	l.rowsBuilt[id] = top + dy
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
