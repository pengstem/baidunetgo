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

// The shaders, in shaders/: a row breaking into specks as it is deleted
// and gathering from them as it uploads, and the drop zone's ring.
var (
	//go:embed shaders/specks.kage
	specksSource string
	//go:embed shaders/glow.kage
	glowSource string

	specks = shader.MustCompile(specksSource)
	glow   = shader.MustCompile(glowSource)
)

// dissolveFor is how long a deleted row takes to break into specks and
// drift away, before the rows after it close up.
const dissolveFor = 1100 * time.Millisecond

// epoch is the zero of the shaders' clocks.
var epoch = time.Now()

func shaderClock(now time.Time) float32 { return float32(now.Sub(epoch).Seconds()) }

// speckle draws an element through the specks shader: progress from 0,
// whole, to 1, scattered; gathering, the specks drift about as they wait
// and settle from the left, else they blow up and away from the left.
func speckle(progress float32, gathering bool, seed uint32, tint ui.Color, now time.Time) ui.Effect {
	// How far the specks stray (offset in specks.kage): up and to the
	// right as they blow away, about as they gather.
	reach, margin, g := []float32{0, 26, -29, 0}, ui.Point{X: 28, Y: 30}, 0
	if gathering {
		reach, margin, g = []float32{-21, 21, -10, 10}, ui.Point{X: 22, Y: 12}, 1
	}
	return specks.Effect(shader.Options{MaxSampleOffset: margin, Uniforms: map[string]any{
		"Progress": progress, "Gather": g, "Time": shaderClock(now), "Seed": seed % 9973, "Tint": tint, "Reach": reach,
	}})
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
	var gone float32
	switch g := a.gathers[f.Path]; {
	case a.dying[f.FsID] != nil:
		gone = since(now, a.dying[f.FsID].at, dissolveFor)
	case g != nil && g.failed:
		gone = since(now, g.ended, dissolveFor)
	case g != nil && (g.shown < 1 || f.FsID < 0):
		if g.shown >= 1 {
			return true, 0 // up, waiting for the listing to show the file
		}
		if a.gpu {
			e.Effect(speckle(1-g.shown, true, seedOf(f.Path), t.Accent.Alpha(0.85), now))
		} else {
			e.Opacity(0.35 + 0.65*g.shown)
		}
		e.Draw(func(p *ui.Painter, _ ui.Rect) { p.AnimationFrame() })
		return true, 0
	default:
		return false, 0
	}
	if a.gpu {
		e.Effect(speckle(gone, false, seedOf(f.Path), t.Accent.Alpha(0.5), now))
	} else {
		e.Opacity(1 - gone)
	}
	e.Draw(func(p *ui.Painter, _ ui.Rect) { p.AnimationFrame() })
	return true, gone
}

// since returns how far now is into a span of d from start, from 0 to 1.
func since(now, start time.Time, d time.Duration) float32 {
	return float32(min(max(float64(now.Sub(start))/float64(d), 0), 1))
}

// The shimmer's band runs down and across the window, along x + y/2 in
// DIPs, shimmerSpeed DIPs of that a second, one every shimmerPeriod, its
// brightness a bell shimmerWidth wide.
const (
	shimmerPeriod = 1100
	shimmerSpeed  = 520
	shimmerWidth  = 90
)

// drawShimmer fills r, rounded, as a placeholder, brightening as a band of
// light sweeps by on a GPU, else plainly. The band runs across the window,
// not each placeholder, so that all of them shimmer as one wave. Each is
// one plain fill, which the window draws together: a shader or a gradient
// apiece, as many as the placeholders showing, costs frames.
func drawShimmer(p *ui.Painter, r ui.Rect, t *ui.Theme, radius float32) {
	base := t.Background.Mix(t.Text, 0.07)
	if !p.Accelerated() {
		p.Fill(r, base, radius)
		return
	}
	p.AnimationFrame()
	u := r.X + r.W/2 + (r.Y+r.H/2)/2 // where its middle lies along the band's way
	x := float32(math.Mod(float64(u-shaderClock(p.Now())*shimmerSpeed), shimmerPeriod))
	if x < 0 {
		x += shimmerPeriod
	}
	x = (x - shimmerPeriod/2) / shimmerWidth
	p.Fill(r, base.Mix(t.Background.Mix(t.Text, 0.015), float32(math.Exp(float64(-x*x/2)))), radius)
}

// dropGlow is the ring around the browser while files are dragged over it,
// flaring for a moment as they are dropped.
func (a *App) dropGlow(c *ui.Context, t *ui.Theme, over bool) {
	now := c.Now()
	burst := 1 - since(now, a.dropped, 700*time.Millisecond)
	if !over && burst <= 0 {
		return
	}
	ring := ui.Box(c).Key("dropglow").Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().
		Transition(ui.ElementTransition{Animation: &motionFade, Enter: &ui.Motion{Opacity: 0}, Exit: &ui.Motion{Opacity: 0}})
	if !a.gpu {
		ring.Children(func() {
			ui.Box(c).Absolute().Top(6).Left(8).Right(8).Bottom(8).Radius(10).Border(2, t.Accent).PassThrough()
		})
		return
	}
	strength := float32(1)
	if !over {
		strength = burst // the flare fades the ring out
	}
	ring.Draw(func(p *ui.Painter, r ui.Rect) {
		glow.Draw(p, r, shader.Options{Uniforms: map[string]any{
			"Time": shaderClock(p.Now()), "Strength": strength, "Burst": burst * burst, "Pad": 7, "Radius": 10,
		}})
		p.AnimationFrame()
	})
}

// smoothstep eases x from 0 at e0 to 1 at e1, as the shaders' does.
func smoothstep(e0, e1, x float32) float32 {
	k := min(max((x-e0)/(e1-e0), 0), 1)
	return k * k * (3 - 2*k)
}
