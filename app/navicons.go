package app

import (
	"math"

	"github.com/egoist/mygo/ui"
)

// The sidebar's icons are drawn here rather than taken from Lucide, so
// that their parts can move: each page's icon makes a small gesture when
// the page opens, and comes back to rest.

// navGlyph draws the icon of a sidebar page into r, ms milliseconds into
// its gesture; from navIconRun on it is at rest.
func navGlyph(pt *ui.Painter, r ui.Rect, page string, ms float32, color ui.Color) {
	size := min(r.W, r.H)
	g := glyph{pt: pt, x: r.X + (r.W-size)/2, y: r.Y + (r.H-size)/2, s: size / 24, color: color}
	g.w = 1.8 * g.s
	switch page {
	case "/files":
		// The front of the folder tips back from its bottom edge, as a
		// drawer opening, and closes.
		b := bump(local(ms, 0))
		g.at(rest())
		g.move(3, 10).line(3, 6.5).quad(3, 5, 4.5, 5).line(8.8, 5).line(10.8, 7.2).line(19.5, 7.2).
			quad(21, 7.2, 21, 8.7).line(21, 10).stroke()
		g.at(pose{ox: 12, oy: 19.5, sx: 1, sy: 1 - 0.22*b, skew: -16 * b, ty: -b, alpha: 1})
		g.move(3, 10).line(21, 10).line(21, 18).quad(21, 19.5, 19.5, 19.5).line(4.5, 19.5).
			quad(3, 19.5, 3, 18).close().stroke()
	case "/transfers":
		// One arrow goes up and the other down, the second a beat later.
		g.at(pose{sx: 1, sy: 1, ty: -3 * bump(local(ms, 0)), alpha: 1})
		g.move(8, 19).line(8, 5).move(4.5, 8.5).line(8, 5).line(11.5, 8.5).stroke()
		g.at(pose{sx: 1, sy: 1, ty: 3 * bump(local(ms, 40)), alpha: 1})
		g.move(16, 5).line(16, 19).move(12.5, 15.5).line(16, 19).line(19.5, 15.5).stroke()
	case "/shares":
		// The nodes swell in turn, a signal passed from you to two others.
		g.at(rest())
		g.move(8.1, 10.9).line(14.9, 7.1).move(8.1, 13.1).line(14.9, 16.9).stroke()
		g.ring(6, 12, 2.4*(1+0.45*bump(local(ms, 0))))
		g.ring(17, 6, 2.4*(1+0.45*bump(local(ms, 60))))
		g.ring(17, 18, 2.4*(1+0.45*bump(local(ms, 120))))
	case "/offline":
		// The arrow drops out under the cloud and falls back in from above.
		g.at(rest())
		g.move(7, 18).line(6.5, 18).cube(4.3, 18, 2.5, 16.2, 2.5, 14).cube(2.5, 11.9, 4.1, 10.2, 6, 10.03).
			cube(6.5, 6.6, 9.2, 4, 12, 4).cube(14.6, 4, 16.9, 5.8, 17.65, 8.5).
			cube(20, 8.6, 22, 10.7, 22, 13.5).cube(22, 16, 20, 18, 17.5, 18).line(17, 18).stroke()
		l := local(ms, 0)
		arrow := pose{sx: 1, sy: 1, alpha: 1}
		if l > 0 && l < 1 {
			if l < 0.4 {
				arrow.ty, arrow.alpha = 4*l/0.4, 1-l/0.4
			} else {
				u := (l - 0.4) / 0.6
				arrow.ty, arrow.alpha = -5*(1-easeOut(u)), u
			}
		}
		g.at(arrow)
		g.move(12, 11).line(12, 20).move(9, 17).line(12, 20).line(15, 17).stroke()
	case "/recycle":
		// The lid lifts from its left end and drops back.
		b := bump(local(ms, 0))
		g.at(pose{ox: 4, oy: 6.5, sx: 1, sy: 1, rot: -16 * b, ty: -2 * b, alpha: 1})
		g.move(4, 6.5).line(20, 6.5).move(9.5, 6.5).line(9.5, 4.8).quad(9.5, 4, 10.3, 4).line(13.7, 4).
			quad(14.5, 4, 14.5, 4.8).line(14.5, 6.5).stroke()
		g.at(rest())
		g.move(6, 6.5).line(7, 19.5).quad(7.1, 20.9, 8.5, 20.9).line(15.5, 20.9).quad(16.9, 20.9, 17, 19.5).
			line(18, 6.5).move(10, 10.5).line(10, 16.5).move(14, 10.5).line(14, 16.5).stroke()
	case "/settings":
		// The gear turns one tooth, ending as it started.
		g.at(pose{ox: 12, oy: 12, sx: 1, sy: 1, rot: 60 * easeOut(local(ms, 0)), alpha: 1})
		// Six teeth: each a step out to radius 9.5 and back to 7.
		for i := range 24 {
			a := float64(i/4)*math.Pi/3 + []float64{-0.42, -0.2, 0.2, 0.42}[i%4]
			r := float32([]float64{7, 9.5, 9.5, 7}[i%4])
			x, y := 12+r*float32(math.Cos(a)), 12+r*float32(math.Sin(a))
			if i == 0 {
				g.move(x, y)
			} else {
				g.line(x, y)
			}
		}
		g.close().stroke()
		g.ring(12, 12, 3)
	}
}

// local returns how far a part of a gesture that starts delay ms in and
// lasts 240 ms has got, from 0 to 1.
func local(ms, delay float32) float32 { return min(max((ms-delay)/240, 0), 1) }

// bump goes out and back: 0 at either end of l and 1 in the middle.
func bump(l float32) float32 { return float32(math.Sin(math.Pi * float64(l))) }

func easeOut(l float32) float32 { return 1 - (1-l)*(1-l)*(1-l) }

// pose moves the points of a glyph's part, in its 24-unit grid: scaled
// and skewed about (ox, oy), turned rot degrees there, then shifted.
type pose struct {
	ox, oy, sx, sy, skew, rot, tx, ty, alpha float32
}

func rest() pose { return pose{sx: 1, sy: 1, alpha: 1} }

func (q pose) apply(x, y float32) (float32, float32) {
	x, y = (x-q.ox)*q.sx, (y-q.oy)*q.sy
	if q.skew != 0 {
		x += float32(math.Tan(float64(q.skew)*math.Pi/180)) * y
	}
	if q.rot != 0 {
		a := float64(q.rot) * math.Pi / 180
		cos, sin := float32(math.Cos(a)), float32(math.Sin(a))
		x, y = x*cos-y*sin, x*sin+y*cos
	}
	return x + q.ox + q.tx, y + q.oy + q.ty
}

// glyph strokes paths given in a 24-unit grid, as Lucide's are.
type glyph struct {
	pt         *ui.Painter
	x, y, s, w float32
	color      ui.Color
	q          pose
	path       ui.Path
}

func (g *glyph) at(q pose) { g.q = q }

func (g *glyph) pos(x, y float32) (float32, float32) {
	x, y = g.q.apply(x, y)
	return g.x + x*g.s, g.y + y*g.s
}

func (g *glyph) move(x, y float32) *glyph {
	g.path.MoveTo(g.pos(x, y))
	return g
}

func (g *glyph) line(x, y float32) *glyph {
	g.path.LineTo(g.pos(x, y))
	return g
}

func (g *glyph) quad(cx, cy, x, y float32) *glyph {
	px, py := g.pos(cx, cy)
	ex, ey := g.pos(x, y)
	g.path.QuadTo(px, py, ex, ey)
	return g
}

func (g *glyph) cube(c1x, c1y, c2x, c2y, x, y float32) *glyph {
	ax, ay := g.pos(c1x, c1y)
	bx, by := g.pos(c2x, c2y)
	ex, ey := g.pos(x, y)
	g.path.CubeTo(ax, ay, bx, by, ex, ey)
	return g
}

func (g *glyph) close() *glyph {
	g.path.Close()
	return g
}

// ring strokes a circle of radius r around (cx, cy).
func (g *glyph) ring(cx, cy, r float32) {
	x, y := g.pos(cx, cy)
	g.path.Circle(x, y, r*g.s)
	g.stroke()
}

func (g *glyph) stroke() {
	g.pt.StrokePath(&g.path, g.w, g.color.Alpha(g.q.alpha))
	g.path.Reset()
}
