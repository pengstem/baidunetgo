package app

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/gif"
	"io"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/remote"
)

// GIFs keep all their indexed frames plus the player's RGBA buffers.
// Bound those before decoding: a small compressed file can fill memory.
const gifMemoryLimit = 128 << 20

func decodePreviewImage(data []byte) (ui.ImageSource, *ui.VideoPlayer, error) {
	if bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")) {
		if err := checkGIFMemory(data); err != nil {
			return nil, nil, err
		}
		d, err := ui.NewGIFDecoder(data)
		if err != nil {
			return nil, nil, err
		}
		p, err := ui.NewVideoPlayer(d)
		return nil, p, err
	}
	img, err := ui.DecodeImage(data)
	return img, nil, err
}

// checkGIFMemory walks block headers without decompressing any pixels.
// Leave validating the image data to MyGo's decoder.
func checkGIFMemory(data []byte) error {
	cfg, err := gif.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return err
	}
	// The canvas, disposal backup and player's current, first and spare
	// frames can coexist; reserve eight full RGBA pictures for them.
	memory := int64(cfg.Width) * int64(cfg.Height) * 32
	if memory > gifMemoryLimit {
		return remote.ErrTooLarge
	}
	pos := 13 // header and logical screen descriptor
	if data[10]&0x80 != 0 {
		pos += 3 << (1 + (data[10] & 7)) // global color table
	}
	for pos < len(data) {
		block := data[pos]
		pos++
		switch block {
		case 0x3b: // trailer
			return nil
		case 0x21: // extension's label, followed by sub-blocks
			pos++
		case 0x2c: // image descriptor and optional local color table
			if len(data)-pos < 9 {
				return io.ErrUnexpectedEOF
			}
			w := int64(binary.LittleEndian.Uint16(data[pos+4:]))
			h := int64(binary.LittleEndian.Uint16(data[pos+6:]))
			memory += w*h + 2048 // indexed pixels, palette and frame metadata
			if memory > gifMemoryLimit {
				return remote.ErrTooLarge
			}
			flags := data[pos+8]
			pos += 9
			if flags&0x80 != 0 {
				pos += 3 << (1 + (flags & 7))
			}
			pos++ // LZW code size
		default:
			return image.ErrFormat
		}
		// Skip extension data or compressed image sub-blocks.
		for {
			if pos >= len(data) {
				return io.ErrUnexpectedEOF
			}
			n := int(data[pos])
			pos++
			if n == 0 {
				break
			}
			pos += n
		}
	}
	return io.ErrUnexpectedEOF
}
