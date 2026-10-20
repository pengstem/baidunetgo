package pcsport

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"hash"
	"hash/crc32"
	"io"
	"sync"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
)

// sums are what a rapid upload names a file by.
type sums struct {
	md5    []byte   // of the whole file
	slice  []byte   // of its first SliceMD5Size bytes
	crc32  uint32   // of the whole file
	blocks []string // hex MD5 of each block of the upload
}

// chunk is how much checksums reads at a time; block sizes are multiples
// of it.
const chunk = 1 << 20

// checksums reads r's size bytes once for every sum the upload needs. The
// whole-file MD5 runs on one goroutine, the CRC32 and block MD5s on
// another, while the next chunk is read, so it takes about as long as one
// MD5 of the file. An empty file has no sums, as in BaiduPCS-Go.
func checksums(ctx context.Context, r io.ReaderAt, size, block int64) (sums, error) {
	var s sums
	if size <= 0 {
		return s, nil
	}
	whole, crc := md5.New(), crc32.NewIEEE()
	var blk hash.Hash
	var inBlock int64
	sliceEnd := min(int64(baidupcs.SliceMD5Size), size)
	sumBlock := func() {
		s.blocks = append(s.blocks, hex.EncodeToString(blk.Sum(nil)))
		blk, inBlock = nil, 0
	}

	bufs := [2][]byte{make([]byte, chunk), make([]byte, chunk)}
	read := func(buf []byte, off int64) ([]byte, error) {
		n, err := r.ReadAt(buf[:min(int64(len(buf)), size-off)], off)
		if err == io.EOF && off+int64(n) == size {
			err = nil
		}
		return buf[:n], err
	}
	cur, err := read(bufs[0], 0)
	for off := int64(0); err == nil && len(cur) > 0; {
		if err = ctx.Err(); err != nil {
			break
		}
		var wg sync.WaitGroup
		at := off
		wg.Go(func() {
			p := cur
			if at < sliceEnd {
				k := sliceEnd - at
				if k <= int64(len(p)) {
					whole.Write(p[:k])
					s.slice = whole.Sum(nil)
					p = p[k:]
				}
			}
			whole.Write(p)
		})
		wg.Go(func() {
			crc.Write(cur)
			for p := cur; len(p) > 0; {
				if blk == nil {
					blk = md5.New()
				}
				k := min(block-inBlock, int64(len(p)))
				blk.Write(p[:k])
				inBlock += k
				p = p[k:]
				if inBlock == block {
					sumBlock()
				}
			}
		})
		off += int64(len(cur))
		var next []byte
		if off < size {
			next, err = read(bufs[off/chunk%2], off)
		}
		wg.Wait()
		cur = next
	}
	if err != nil {
		return sums{}, err
	}
	if blk != nil {
		sumBlock()
	}
	s.md5, s.crc32 = whole.Sum(nil), crc.Sum32()
	return s, nil
}
