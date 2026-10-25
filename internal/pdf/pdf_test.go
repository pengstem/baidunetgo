package pdf

import (
	"context"
	"testing"

	"github.com/pengstem/baidunetgo/internal/fakepan"
)

func TestRender(t *testing.T) {
	d, err := Open(context.Background(), fakepan.SamplePDF(3))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if len(d.Pages) != 3 || d.Pages[0].Width != 595 {
		t.Fatalf("pages %+v", d.Pages)
	}
	img, err := d.Render(1, 300)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 300 || b.Dy() < 420 {
		t.Errorf("size %v", b)
	}
	// Something was drawn: not every pixel is the white page.
	inked := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] < 200 {
			inked++
		}
	}
	if inked < 100 {
		t.Errorf("page looks blank: %d inked pixels", inked)
	}
}

func TestRenderPart(t *testing.T) {
	d, err := Open(context.Background(), fakepan.SamplePDF(1))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	whole, err := d.Render(0, 600)
	if err != nil {
		t.Fatal(err)
	}
	// From whole pixels, as the app asks for it.
	part, err := d.RenderPart(0, Part{X: 0.25, Y: 85 / 849.0, W: 0.5, H: 0.4}, 600)
	if err != nil {
		t.Fatal(err)
	}
	if b := part.Bounds(); b.Dx() < 299 || b.Dx() > 300 || b.Dy() < 338 || b.Dy() > 340 {
		t.Fatalf("size %v", b)
	}
	// The part is the whole page's pixels there, drawn at the same scale.
	ox, oy := 150, 85
	off := 0
	for y := 0; y < part.Bounds().Dy(); y += 3 {
		for x := 0; x < part.Bounds().Dx(); x += 3 {
			a, b := part.RGBAAt(x, y), whole.RGBAAt(ox+x, oy+y)
			if diff(a.R, b.R) > 40 || diff(a.G, b.G) > 40 || diff(a.B, b.B) > 40 {
				off++
			}
		}
	}
	if n := part.Bounds().Dx() * part.Bounds().Dy() / 9; off > n/30 {
		t.Errorf("%d of %d pixels differ from the whole page's", off, n)
	}
}

func diff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}
