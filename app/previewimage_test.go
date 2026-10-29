package app

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/fakepan"
	"github.com/pengstem/baidunetgo/internal/remote"
)

const sampleSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="320" height="160" viewBox="0 0 320 160"><rect x="40" y="20" width="240" height="120" rx="24" fill="#007aff"/></svg>`

func waitForImage(t *testing.T, a *App, tt *ui.Tester) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !a.pv.hasImage() {
		if a.pv.err != "" || time.Now().After(deadline) {
			t.Fatalf("image did not load: %q", a.pv.err)
		}
		time.Sleep(10 * time.Millisecond)
		tt.Advance(50 * time.Millisecond)
	}
	tt.Advance(300 * time.Millisecond)
}

func TestPreviewSVG(t *testing.T) {
	a, tt, srv := signedIn(t)
	srv.Add(fakepan.Node{Path: "/格式/矢量.svg", Data: []byte(sampleSVG)})
	a.router.Push(filesRoute("/格式"))
	waitFor(t, tt, "矢量.svg")
	a.open(fileNamed(t, a, "/格式", "矢量.svg"))
	waitForImage(t, a, tt)
	if _, ok := a.pv.image.(*ui.SVG); !ok {
		t.Fatalf("preview is %T, want a vector", a.pv.image)
	}
	if w, h := a.pv.imageSize(); w != 320 || h != 160 {
		t.Fatalf("size %v×%v", w, h)
	}
	tt.Key(ui.Cmd, ui.KeyEqual)
	if a.pv.zoom != 1.5 {
		t.Fatalf("SVG zoom %v", a.pv.zoom)
	}
	tt.Advance(300 * time.Millisecond)
	save(t, tt, "preview-svg")
	tt.Key(ui.Cmd, ui.KeyC)
	copied := clipboardPicture(t, tt)
	if b := copied.Bounds(); b.Dx() != 320 || b.Dy() != 160 {
		t.Fatalf("clipboard size %v", b)
	}
	if _, _, b, alpha := copied.At(160, 80).RGBA(); b < 60000 || alpha != 65535 {
		t.Error("copied SVG lost its blue fill")
	}
	if _, _, _, alpha := copied.At(0, 0).RGBA(); alpha != 0 {
		t.Error("copied SVG lost its transparency")
	}
	a.closePreview()
}

func sampleGIF(t *testing.T) []byte {
	t.Helper()
	palette := color.Palette{color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}}
	red := image.NewPaletted(image.Rect(0, 0, 160, 120), palette)
	blue := image.NewPaletted(red.Rect, palette)
	for i := range blue.Pix {
		blue.Pix[i] = 1
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, &gif.GIF{Image: []*image.Paletted{red, blue}, Delay: []int{10, 10}}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPreviewGIF(t *testing.T) {
	a, tt, srv := signedIn(t)
	srv.Add(fakepan.Node{Path: "/格式/动画.gif", Data: sampleGIF(t)})
	a.router.Push(filesRoute("/格式"))
	waitFor(t, tt, "动画.gif")
	a.open(fileNamed(t, a, "/格式", "动画.gif"))
	waitForImage(t, a, tt)
	pv := a.pv
	if pv.video == nil || !pv.video.Loop() {
		t.Fatal("GIF did not open as a looping animation")
	}
	// Sample the actual viewer, allowing the decoder's goroutine to put
	// the next frames in its queue between frames of the UI.
	var red, blue bool
	for range 30 {
		r := pv.view
		pixel := tt.Image().RGBAAt(int(r.X+r.W/2), int(r.Y+r.H/2))
		red = red || pixel.R > 200 && pixel.B < 50
		blue = blue || pixel.B > 200 && pixel.R < 50
		if red && blue {
			break
		}
		time.Sleep(time.Millisecond)
		tt.Advance(25 * time.Millisecond)
	}
	if !red || !blue {
		t.Fatalf("GIF did not change frames: red %v, blue %v, error %v", red, blue, pv.video.Err())
	}
	tt.Key(ui.Cmd, ui.KeyEqual)
	if pv.zoom != 1.5 {
		t.Fatalf("GIF zoom %v", pv.zoom)
	}
	tt.Advance(300 * time.Millisecond)
	save(t, tt, "preview-gif")
	tt.Key(ui.Cmd, ui.KeyC)
	if b := clipboardPicture(t, tt).Bounds(); b.Dx() != 160 || b.Dy() != 120 {
		t.Fatalf("copied GIF's first frame is %v", b)
	}
	tt.Key(0, ui.KeyEscape)
	if a.pv != nil || pv.video.Playing() {
		t.Fatal("closing the preview did not stop GIF playback")
	}
}

func TestGIFMemory(t *testing.T) {
	data := sampleGIF(t)
	if err := checkGIFMemory(data); err != nil {
		t.Fatal(err)
	}
	large := bytes.Clone(data)
	binary.LittleEndian.PutUint16(large[6:], 65535)
	binary.LittleEndian.PutUint16(large[8:], 65535)
	if err := checkGIFMemory(large); !errors.Is(err, remote.ErrTooLarge) {
		t.Fatalf("oversized canvas: %v", err)
	}
	// Repeating the encoded frames expands their memory much more than
	// their file size; reject them before DecodeAll allocates the pixels.
	start := 13
	if data[10]&0x80 != 0 {
		start += 3 << (1 + (data[10] & 7))
	}
	many := append(bytes.Clone(data[:start]), bytes.Repeat(data[start:len(data)-1], 4000)...)
	many = append(many, 0x3b)
	if err := checkGIFMemory(many); !errors.Is(err, remote.ErrTooLarge) {
		t.Fatalf("too many frames: %v", err)
	}
	for _, broken := range [][]byte{data[:10], data[:len(data)-2]} {
		if err := checkGIFMemory(broken); err == nil {
			t.Error("truncated GIF accepted")
		}
	}
}
