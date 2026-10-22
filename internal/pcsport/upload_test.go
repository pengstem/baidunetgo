package pcsport

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io"
	"math/rand"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/qjfoidnh/BaiduPCS-Go/requester/rio"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/uploader"
)

// fakeDisk takes parts in memory. It stops taking them after stopAfter
// parts, cancelling the upload, and refuses the ids in gone.
type fakeDisk struct {
	mu        sync.Mutex
	ids       int
	sent      []int // parts sent, in order
	stopAfter int
	cancel    context.CancelFunc
	gone      map[string]bool
	created   []string
	data      []byte
}

func (d *fakeDisk) precreate(f io.ReaderAt, r *Resume, id string) (bool, string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ids++
	return false, "u" + string(rune('0'+d.ids)), nil
}

func (d *fakeDisk) part(ctx context.Context, id string, seq int, off int64, r rio.ReaderLen64) (string, error) {
	data, _ := io.ReadAll(r)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.gone[id] {
		return "", &uploader.MultiError{Terminated: true, Err: errors.New("no such upload")}
	}
	if d.stopAfter > 0 && len(d.sent) >= d.stopAfter {
		d.cancel()
		return "", ctx.Err()
	}
	d.sent = append(d.sent, seq)
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:]), nil
}

func (d *fakeDisk) create(id string, size int64, parts []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.gone[id] {
		return errors.New("no such upload")
	}
	d.created = parts
	return nil
}

// countingReaderAt counts the bytes read of it.
type countingReaderAt struct {
	r    *bytes.Reader
	read atomic.Int64
}

func (c *countingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	n, err := c.r.ReadAt(p, off)
	c.read.Add(int64(n))
	return n, err
}

func TestUploadResumes(t *testing.T) {
	data := make([]byte, 4*chunk*4+12345) // five 4 MB parts
	rand.New(rand.NewSource(1)).Read(data)
	f := &countingReaderAt{r: bytes.NewReader(data)}
	var want []string
	for off := 0; off < len(data); off += 4 << 20 {
		sum := md5.Sum(data[off:min(off+4<<20, len(data))])
		want = append(want, hex.EncodeToString(sum[:]))
	}

	// The first try stops after two parts.
	ctx, cancel := context.WithCancel(context.Background())
	d := &fakeDisk{stopAfter: 2, cancel: cancel}
	var saved Resume
	err := upload(ctx, d, f, int64(len(data)), 7, 1, nil, nil, func(r Resume) { saved = r })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("first try: %v", err)
	}
	if saved.ID != "u1" || len(saved.Parts) != 5 || saved.Parts[0] != want[0] || saved.Parts[1] != want[1] || saved.Parts[2] != "" {
		t.Fatalf("saved %+v", saved)
	}

	// The second goes on: no checksums read again, only the parts left.
	f.read.Store(0)
	d.stopAfter, d.sent = 0, nil
	var last Resume
	if err := upload(context.Background(), d, f, int64(len(data)), 7, 2, nil, &saved, func(r Resume) { last = r }); err != nil {
		t.Fatal(err)
	}
	slices.Sort(d.sent)
	if !slices.Equal(d.sent, []int{2, 3, 4}) {
		t.Errorf("sent again %v", d.sent)
	}
	if !slices.Equal(d.created, want) {
		t.Errorf("joined %v", d.created)
	}
	if got := f.read.Load(); got > int64(len(data)-2*(4<<20))+sliceContent {
		t.Errorf("read %d bytes, the sums again", got)
	}
	if last.ID != "u1" {
		t.Errorf("went on with %q", last.ID)
	}

	// A file changed since starts afresh.
	d.sent = nil
	if err := upload(context.Background(), d, f, int64(len(data)), 8, 2, nil, &saved, nil); err != nil {
		t.Fatal(err)
	}
	if len(d.sent) != 5 {
		t.Errorf("changed file sent %v", d.sent)
	}
}

// TestUploadExpired starts afresh when the netdisk has let go of the
// upload it went on with.
func TestUploadExpired(t *testing.T) {
	data := make([]byte, 9<<20)
	rand.New(rand.NewSource(2)).Read(data)
	f := bytes.NewReader(data)
	d := &fakeDisk{gone: map[string]bool{"old": true}}
	var r Resume
	if err := upload(context.Background(), d, f, int64(len(data)), 1, 1, nil, nil, func(x Resume) { r = x }); err != nil {
		t.Fatal(err)
	}
	from := r
	from.ID = "old"
	from.Parts = []string{r.Parts[0], "", ""}
	d.sent, d.created = nil, nil
	var last Resume
	if err := upload(context.Background(), d, f, int64(len(data)), 1, 1, nil, &from, func(x Resume) { last = x }); err != nil {
		t.Fatal(err)
	}
	if len(d.sent) != 3 || last.ID == "old" || len(d.created) != 3 {
		t.Errorf("sent %v, id %q, joined %d", d.sent, last.ID, len(d.created))
	}
}
