// Command appicon draws the app icon, resources/icon.png: a white cloud
// with a download arrow on a blue squircle, laid out on macOS's icon grid
// (an 824 px body in a 1024 px canvas, with its shadow). Run it from the
// repository root after changing the drawing.
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
)

const (
	size = 1024
	body = 412 // half the squircle's side
	ss   = 4   // samples per pixel along each axis
)

// Colors of the squircle from top to bottom, around the app's accent.
var top, bottom = rgb(0x4d, 0xb2, 0xff), rgb(0x00, 0x5e, 0xe0)

type vec struct{ r, g, b float64 }

func rgb(r, g, b uint8) vec { return vec{float64(r) / 255, float64(g) / 255, float64(b) / 255} }

func mix(a, b vec, t float64) vec {
	return vec{a.r + (b.r-a.r)*t, a.g + (b.g-a.g)*t, a.b + (b.b-a.b)*t}
}

// squircle is the signed distance, roughly, to the icon's body.
func squircle(x, y float64) float64 {
	const n = 5.0
	x, y = math.Abs(x-512), math.Abs(y-512)
	return math.Pow(math.Pow(x, n)+math.Pow(y, n), 1/n) - body
}

func circle(x, y, cx, cy, r float64) float64 { return math.Hypot(x-cx, y-cy) - r }

func box(x, y, cx, cy, hw, hh, r float64) float64 {
	qx, qy := math.Abs(x-cx)-hw+r, math.Abs(y-cy)-hh+r
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
}

func capsule(x, y, ax, ay, bx, by, r float64) float64 {
	px, py, dx, dy := x-ax, y-ay, bx-ax, by-ay
	t := math.Max(0, math.Min(1, (px*dx+py*dy)/(dx*dx+dy*dy)))
	return math.Hypot(px-dx*t, py-dy*t) - r
}

// cloud is the distance to the cloud, which sits a little below the
// middle so that it looks centered.
func cloud(x, y float64) float64 {
	y -= 30
	// Two lobes resting on the base line y = 680, a flat bottom between
	// their centers, and a big lobe on top, filleted into them.
	base := min(
		circle(x, y, 352, 560, 120),
		circle(x, y, 668, 540, 140),
		box(x, y, 510, 620, 158, 60, 0),
	)
	return smin(base, circle(x, y, 500, 455, 180), 24)
}

// smin joins two shapes with a fillet k px wide.
func smin(a, b, k float64) float64 {
	h := math.Max(k-math.Abs(a-b), 0) / k
	return math.Min(a, b) - h*h*k/4
}

// arrow is the distance to the download arrow cut out of the cloud.
func arrow(x, y float64) float64 {
	y -= 30
	const w = 30
	return min(
		capsule(x, y, 512, 410, 512, 590, w),
		capsule(x, y, 512, 600, 432, 520, w),
		capsule(x, y, 512, 600, 592, 520, w),
	)
}

func smoothstep(a, b, x float64) float64 {
	t := math.Max(0, math.Min(1, (x-a)/(b-a)))
	return t * t * (3 - 2*t)
}

// over composites color c with alpha a over the premultiplied pixel p.
func over(p *[4]float64, c vec, a float64) {
	p[0] = c.r*a + p[0]*(1-a)
	p[1] = c.g*a + p[1]*(1-a)
	p[2] = c.b*a + p[2]*(1-a)
	p[3] = a + p[3]*(1-a)
}

// sample is the premultiplied color at a point.
func sample(x, y float64) [4]float64 {
	var p [4]float64
	// The body's shadow, soft and a little below it.
	if a := 0.38 * (1 - smoothstep(-24, 36, squircle(x, y-14))); a > 0 {
		over(&p, vec{}, a)
	}
	if squircle(x, y) > 0 {
		return p
	}
	bg := mix(top, bottom, smoothstep(100, 924, y))
	over(&p, bg, 1)
	// A faint shine on the upper half.
	over(&p, vec{1, 1, 1}, 0.10*(1-smoothstep(100, 560, y)))
	// The cloud's shadow on the body.
	if a := 0.22 * (1 - smoothstep(-10, 40, cloud(x, y-16))); a > 0 {
		over(&p, rgb(0x00, 0x2a, 0x80), a)
	}
	if cloud(x, y) <= 0 {
		if arrow(x, y) <= 0 {
			over(&p, mix(rgb(0x2a, 0x9a, 0xff), rgb(0x00, 0x62, 0xe6), smoothstep(380, 640, y)), 1)
		} else {
			over(&p, mix(vec{1, 1, 1}, rgb(0xe4, 0xf0, 0xff), smoothstep(280, 720, y)), 1)
		}
	}
	return p
}

func main() {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for py := range size {
		for px := range size {
			var acc [4]float64
			for sy := range ss {
				for sx := range ss {
					s := sample(float64(px)+(float64(sx)+0.5)/ss, float64(py)+(float64(sy)+0.5)/ss)
					for i := range acc {
						acc[i] += s[i]
					}
				}
			}
			a := acc[3] / (ss * ss)
			if a == 0 {
				continue
			}
			c := func(v float64) uint8 { return uint8(math.Round(math.Min(1, v/(ss*ss)/a) * 255)) }
			img.SetNRGBA(px, py, color.NRGBA{c(acc[0]), c(acc[1]), c(acc[2]), uint8(math.Round(a * 255))})
		}
	}
	if err := os.MkdirAll("resources", 0o755); err != nil {
		log.Fatal(err)
	}
	f, err := os.Create("resources/icon.png")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}
