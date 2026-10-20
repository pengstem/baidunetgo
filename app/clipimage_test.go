package app

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// withOrientation puts an EXIF segment saying orientation o into a JPEG.
func withOrientation(jpg []byte, o uint16) []byte {
	tiff := []byte("MM\x00\x2a\x00\x00\x00\x08")
	tiff = binary.BigEndian.AppendUint16(tiff, 1) // one entry
	tiff = binary.BigEndian.AppendUint16(tiff, 0x0112)
	tiff = binary.BigEndian.AppendUint16(tiff, 3)
	tiff = binary.BigEndian.AppendUint32(tiff, 1)
	tiff = binary.BigEndian.AppendUint16(tiff, o)
	tiff = append(tiff, 0, 0, 0, 0, 0, 0)
	seg := append([]byte("Exif\x00\x00"), tiff...)
	app1 := []byte{0xff, 0xe1}
	app1 = binary.BigEndian.AppendUint16(app1, uint16(len(seg)+2))
	out := append([]byte{}, jpg[:2]...)
	out = append(out, app1...)
	out = append(out, seg...)
	return append(out, jpg[2:]...)
}

func TestClipPNG(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for y := range 20 {
		for x := range 40 {
			if x < 20 {
				src.Set(x, y, color.RGBA{255, 0, 0, 255})
			} else {
				src.Set(x, y, color.RGBA{0, 0, 255, 255})
			}
		}
	}
	var buf bytes.Buffer
	jpeg.Encode(&buf, src, &jpeg.Options{Quality: 95})

	out, err := clipPNG(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 40 || b.Dy() != 20 {
		t.Errorf("plain JPEG came out %v", b)
	}

	// Orientation 6: turned a quarter clockwise to show, so the left
	// (red) half ends up on top.
	out, err = clipPNG(withOrientation(buf.Bytes(), 6))
	if err != nil {
		t.Fatal(err)
	}
	img, _ = png.Decode(bytes.NewReader(out))
	if b := img.Bounds(); b.Dx() != 20 || b.Dy() != 40 {
		t.Fatalf("turned JPEG came out %v", b)
	}
	if r, _, b, _ := img.At(10, 5).RGBA(); r < b {
		t.Error("the top is not red")
	}

	var pngBuf bytes.Buffer
	png.Encode(&pngBuf, src)
	if out, _ := clipPNG(pngBuf.Bytes()); !bytes.Equal(out, pngBuf.Bytes()) {
		t.Error("a PNG was not copied as it is")
	}
	if _, err := clipPNG([]byte("not a picture")); err == nil {
		t.Error("no error for garbage")
	}
}
