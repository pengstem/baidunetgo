package app

import (
	_ "embed"
	"hash/fnv"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// The shader, in shaders/: a row breaking into specks as it is deleted
// and gathering from them as it uploads.
var (
	//go:embed shaders/specks.kage
	specksSource string

	specks = shader.MustCompile(specksSource)
)

// dissolveFor is how long a deleted row takes to break into specks and
// drift away, before the rows after it close up.
const dissolveFor = 1100 * time.Millisecond

// Slow, continuous drawings need no more than 60 frames a second, even
// on high-refresh displays. Other UI motion keeps the display's rate.
const effectFPS = 60

// epoch is the zero of the shaders' clocks.
var epoch = time.Now()

func shaderClock(now time.Time) float32 { return float32(now.Sub(epoch).Seconds()) }

// specksFx draws an element through the specks shader, progress(now)
// from 0, whole, to 1, scattered: gathering, the specks drift about as
// they wait and settle from the left, else they blow up and away from the
// left. It reads the progress and the time as it paints, not as the view
// builds: the frames AnimationFrame asks for paint the window again
// without building it, and values set while building would stand still.
type specksFx struct {
	progress  func(now time.Time) float32
	gathering bool
	seed      uint32
	tint      ui.Color
}

// reach returns how far the specks stray (offset in specks.kage): up and
// to the right as they blow away, about as they gather; and the margin
// the effect draws over around the element for it.
func (fx specksFx) reach() ([]float32, ui.Point) {
	if fx.gathering {
		return []float32{-21, 21, -10, 10}, ui.Point{X: 22, Y: 12}
	}
	return []float32{0, 26, -29, 0}, ui.Point{X: 28, Y: 30}
}

func (fx specksFx) EffectMargin() ui.Point { _, m := fx.reach(); return m }

func (fx specksFx) PaintEffect(p *ui.Painter, box ui.Rect, content *ui.Bitmap, inset ui.Point) {
	if content == nil || box.W <= 0 || box.H <= 0 {
		return
	}
	now := p.Now()
	progress, clock := fx.progress(now), shaderClock(now)
	scale := p.Scale()
	options := shader.Options{
		Images:   [4]*ui.Bitmap{content},
		Uniforms: map[string]any{"Tint": fx.tint, "Inset": [2]float32{inset.X, inset.Y}, "SourceScale": scale},
	}
	// One quad per source cell: no neighborhood search for every output
	// pixel. Flush bounded batches for unusually large previews or tiles.
	const cell = float32(3)
	cols, rows := int(math.Ceil(float64(box.W/cell))), int(math.Ceil(float64(box.H/cell)))
	capacity := min(cols*rows, shader.MaxInstances)
	rects := make([]ui.Rect, 0, capacity)
	data := make([]float32, 0, capacity*5)
	flush := func() {
		specks.DrawInstances(p, shader.Instances{Rects: rects, Data: data, Stride: 5}, options)
		rects, data = rects[:0], data[:0]
	}
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			r := speckRandom(uint32(x), uint32(y), fx.seed%9973)
			cx, cy := (float32(x)+0.5)*cell, (float32(y)+0.5)*cell
			along := cx / box.W
			if fx.gathering {
				along = 1 - along
			}
			t := min(max((progress-0.45*along-0.15*r[0])/0.4, 0), 1)
			if !fx.gathering && t >= 1 {
				continue
			}
			k := 1 - 0.45*t
			dx, dy := speckOffset(t, clock, r, fx.gathering)
			fade := 1 - smoothstep(0.45, 1, t)
			if fx.gathering {
				fade = 1 - 0.45*smoothstep(0.15, 1, t)
			}
			// Clip the final cells to the element, keeping their sampling
			// aligned even when its size is not a multiple of three.
			sx, sy := float32(x)*cell, float32(y)*cell
			w, h := min(cell, box.W-sx), min(cell, box.H-sy)
			rects = append(rects, ui.Rect{
				X: box.X + cx + dx - cell*k/2,
				Y: box.Y + cy + dy - cell*k/2,
				W: w * k, H: h * k,
			})
			data = append(data, sx, sy, 1/k, fade, smoothstep(0, 0.35, t))
			if len(rects) == shader.MaxInstances {
				flush()
			}
		}
	}
	flush()
	p.AnimationFrameRate(effectFPS)
}

// Match the old shader's hash so each file keeps its particle pattern.
func speckRandom(x, y, seed uint32) [4]float32 {
	s := (x*1973+y*9277+seed*26699)*747796405 + 2891336453
	w := ((s >> ((s >> 28) + 4)) ^ s) * 277803737
	h := (w >> 22) ^ w
	return [4]float32{
		(float32(h&255) + 0.5) / 256,
		(float32((h>>8)&255) + 0.5) / 256,
		(float32((h>>16)&255) + 0.5) / 256,
		(float32(h>>24) + 0.5) / 256,
	}
}

func speckOffset(t, clock float32, r [4]float32, gathering bool) (float32, float32) {
	if gathering {
		a, dist := float64(r[1])*2*math.Pi, float64(4+14*r[2])
		k := smoothstep(0, 1, t)
		dx := math.Cos(a)*dist + math.Sin(float64(clock)*1.6+float64(r[3])*2*math.Pi)*2.5
		dy := math.Sin(a)*dist*0.4 + math.Cos(float64(clock)*1.2+float64(r[0])*2*math.Pi)*2.5
		return float32(dx) * k, float32(dy) * k
	}
	a, dist := float64(-0.6+(r[1]-0.5)*0.9), 10+16*r[2]
	e := t * (0.35 + 0.65*t)
	return float32(math.Cos(a)) * dist * e, float32(math.Sin(a))*dist*e - 4*t*t
}

// seedOf picks a file's specks by its path, which an upload's row and the
// file it becomes share.
func seedOf(p string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(p))
	return h.Sum32()
}

// passing draws the row or tile of f as it comes or goes: dissolving after
// a delete or a failed upload, gathering as it uploads. It reports whether
// it does, the row then without a background of its own (its specks
// would carry it), and how far it has gone, from 0 to 1, for a list to
// close up the row.
func (a *App) passing(e ui.Element, f *baidu.File, t *ui.Theme, now time.Time) (bool, float32) {
	fx := specksFx{seed: seedOf(f.Path), tint: t.Accent.Alpha(0.5)}
	var gone float32
	switch g := a.gathers[f.Path]; {
	case a.dying[f.FsID] != nil:
		at := a.dying[f.FsID].at
		fx.progress = func(now time.Time) float32 { return since(now, at, dissolveFor) }
		gone = fx.progress(now)
	case g != nil && g.failed:
		at := g.ended
		fx.progress = func(now time.Time) float32 { return since(now, at, dissolveFor) }
		gone = fx.progress(now)
	case g != nil && (g.shown < 1 || f.FsID < 0):
		if g.shown >= 1 {
			return true, 0 // up, waiting for the listing to show the file
		}
		if !a.gpu {
			e.Opacity(0.35 + 0.65*g.shown)
			return true, 0
		}
		fx.gathering, fx.tint = true, t.Accent.Alpha(0.85)
		fx.progress = func(time.Time) float32 { return 1 - g.shown }
		e.Effect(fx)
		return true, 0
	default:
		return false, 0
	}
	if a.gpu {
		e.Effect(fx)
	} else {
		e.Opacity(1 - gone)
	}
	return true, gone
}

// moving reports whether files coming or going need the view built again
// in the next frame: rows dissolving, which close up and leave, and
// uploads catching up with their share. Their specks move as they paint.
func (a *App) moving() bool {
	if len(a.dying) > 0 {
		return true
	}
	for _, g := range a.gathers {
		if g.failed || g.shown < g.target {
			return true
		}
	}
	return false
}

// since returns how far now is into a span of d from start, from 0 to 1.
func since(now, start time.Time, d time.Duration) float32 {
	return float32(min(max(float64(now.Sub(start))/float64(d), 0), 1))
}

// smoothstep eases x from 0 at e0 to 1 at e1, as the shaders' does.
func smoothstep(e0, e1, x float32) float32 {
	k := min(max((x-e0)/(e1-e0), 0), 1)
	return k * k * (3 - 2*k)
}
