package pcsport

import (
	"bytes"
	"context"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsutil/checksum"
)

// TestChecksums checks the one-pass sums against BaiduPCS-Go's two passes.
func TestChecksums(t *testing.T) {
	const slice = int64(baidupcs.SliceMD5Size)
	const block = 4 << 20
	for _, size := range []int64{0, 1, slice - 1, slice, slice + 1, chunk, block - 1, block, block + 1, 2*block + chunk/3} {
		data := make([]byte, size)
		rand.NewChaCha8([32]byte{byte(size)}).Read(data)
		path := filepath.Join(t.TempDir(), "f")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		lfc := checksum.NewLocalFileChecksum(path, int(slice))
		if err := lfc.OpenPath(); err != nil {
			t.Fatal(err)
		}
		if err := lfc.Sum(checksum.CHECKSUM_MD5 | checksum.CHECKSUM_SLICE_MD5 | checksum.CHECKSUM_CRC32); err != nil {
			t.Fatal(err)
		}
		if err := lfc.CalculateChunkedSum(block); err != nil {
			t.Fatal(err)
		}
		lfc.Close()

		s, err := checksums(context.Background(), bytes.NewReader(data), size, block)
		if err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		if !bytes.Equal(s.md5, lfc.MD5) || !bytes.Equal(s.slice, lfc.SliceMD5) || s.crc32 != lfc.CRC32 || !slices.Equal(s.blocks, lfc.BlocksList) {
			t.Errorf("size %d: got md5 %x slice %x crc %d blocks %v, want %x %x %d %v",
				size, s.md5, s.slice, s.crc32, s.blocks, lfc.MD5, lfc.SliceMD5, lfc.CRC32, lfc.BlocksList)
		}
	}
}

func TestChecksumsCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	data := make([]byte, 3*chunk)
	if _, err := checksums(ctx, bytes.NewReader(data), int64(len(data)), 4<<20); err == nil {
		t.Error("no error after cancel")
	}
}

func BenchmarkChecksums(b *testing.B) {
	data := make([]byte, 256<<20)
	rand.NewChaCha8([32]byte{}).Read(data)
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		checksums(context.Background(), bytes.NewReader(data), int64(len(data)), 16<<20)
	}
}
