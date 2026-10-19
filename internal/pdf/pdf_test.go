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
