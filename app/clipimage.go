package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/draw"
	"image/png"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/remote"
)

// copyImage puts a picture on the clipboard, from data when the preview
// holds it already, else read from the netdisk first. Converting it to
// PNG, which is what clipboards take, runs on a goroutine.
func (a *App) copyImage(f baidu.File, data []byte) {
	s := a.sess
	if s == nil {
		return
	}
	if data == nil {
		if f.Size > imageLimit {
			a.toast("图片太大，不在内存中复制。可以下载后复制。", "error")
			return
		}
		if f.Size > 2<<20 {
			a.toast("正在读取「"+f.Name+"」…", "")
		}
	}
	go func() {
		var err error
		if data == nil {
			data, err = remote.ReadAll(context.Background(), s.client, baidu.Expired, f.Path, f.Size, imageLimit, nil)
		}
		var out []byte
		if err == nil {
			out, err = clipPNG(data)
		}
		a.update(func() {
			if err != nil {
				a.toast("复制失败："+copyError(err), "error")
				return
			}
			a.copyPicture(out, "已复制图片")
		})
	}()
}

// writePNG puts a PNG picture on the clipboard.
func writePNG(c *ui.Context, png []byte) {
	c.Services().WriteClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, png))))
}

func copyError(err error) string {
	if errors.Is(err, image.ErrFormat) {
		return "这种图片格式无法复制"
	}
	return previewError(err)
}

// clipPNG returns a picture as PNG, upright as its camera's EXIF
// orientation says, as the preview shows it.
func clipPNG(data []byte) ([]byte, error) {
	if bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		return data, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if o := exifOrientation(data); o > 1 {
		img = orient(img, o)
	}
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// exifOrientation returns the orientation in a JPEG's EXIF data, 1 to 8,
// or 0 without one.
func exifOrientation(jpg []byte) int {
	if len(jpg) < 4 || jpg[0] != 0xff || jpg[1] != 0xd8 {
		return 0
	}
	for i := 2; i+4 <= len(jpg); {
		if jpg[i] != 0xff {
			return 0
		}
		marker, size := jpg[i+1], int(binary.BigEndian.Uint16(jpg[i+2:]))
		if marker == 0xda || size < 2 || i+2+size > len(jpg) { // the image data starts
			return 0
		}
		seg := jpg[i+4 : i+2+size]
		if marker == 0xe1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			return tiffOrientation(seg[6:])
		}
		i += 2 + size
	}
	return 0
}

// tiffOrientation reads the Orientation tag of TIFF data's first IFD.
func tiffOrientation(t []byte) int {
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}
	ifd := int(bo.Uint32(t[4:]))
	if ifd < 8 || ifd+2 > len(t) {
		return 0
	}
	n := int(bo.Uint16(t[ifd:]))
	for e := ifd + 2; e+12 <= len(t) && n > 0; e, n = e+12, n-1 {
		if bo.Uint16(t[e:]) == 0x0112 && bo.Uint16(t[e+2:]) == 3 { // Orientation, a SHORT
			if o := int(bo.Uint16(t[e+8:])); o >= 1 && o <= 8 {
				return o
			}
			return 0
		}
	}
	return 0
}

// orient turns and flips img as EXIF orientation o says it is to show.
func orient(img image.Image, o int) image.Image {
	b := img.Bounds()
	src := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(src, src.Bounds(), img, b.Min, draw.Src)
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	out := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range dh {
		for x := range dw {
			var sx, sy int
			switch o {
			case 2:
				sx, sy = w-1-x, y
			case 3:
				sx, sy = w-1-x, h-1-y
			case 4:
				sx, sy = x, h-1-y
			case 5:
				sx, sy = y, x
			case 6:
				sx, sy = y, h-1-x
			case 7:
				sx, sy = w-1-y, h-1-x
			case 8:
				sx, sy = w-1-y, x
			default:
				sx, sy = x, y
			}
			copy(out.Pix[4*(y*dw+x):4*(y*dw+x)+4], src.Pix[4*(sy*w+sx):])
		}
	}
	return out
}
