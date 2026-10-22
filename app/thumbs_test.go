package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// TestThumbSizes fetches list icons small and grid tiles larger, showing
// the small one while the larger comes.
func TestThumbSizes(t *testing.T) {
	a, tt, _ := signedIn(t)
	a.router.Push(filesRoute("/照片"))
	waitFor(t, tt, "IMG_2310.jpg")
	settle(tt)
	f := fileNamed(t, a, "/照片", "IMG_2310.jpg")
	if e := a.thumbs.entries[thumbKeyOf(&f, true)]; e == nil || e.bmp == nil {
		t.Fatal("the list fetched no small thumbnail")
	}
	if a.thumbs.entries[thumbKeyOf(&f, false)] != nil {
		t.Error("the list fetched a large thumbnail")
	}
	if a.thumb(&f, 124) == nil {
		t.Error("the small one does not stand in for the large")
	}
}

// TestThumbDiskCache keeps thumbnails on disk, read back without the
// netdisk, and trims the disk to its limit, oldest first.
func TestThumbDiskCache(t *testing.T) {
	a, _, _ := signedIn(t)
	files, err := a.sess.client.List(context.Background(), "/照片", baidu.Order{})
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(files, func(f baidu.File) bool { return f.Thumb != "" })
	if i < 0 {
		t.Fatal("no thumbnails listed")
	}
	f := files[i]
	th := &thumbs{dir: t.TempDir()}
	job := thumbJob{key: thumbKeyOf(&f, false), url: f.Thumb, c: a.sess.client, wanted: new(atomic.Uint64)}
	if th.fetch(context.Background(), job) == nil {
		t.Fatal("not fetched")
	}
	job.url = "http://127.0.0.1:1/gone"
	if th.fetch(context.Background(), job) == nil {
		t.Fatal("not read from disk")
	}

	dir := t.TempDir()
	for i, age := range []time.Duration{3 * time.Hour, time.Hour, 2 * time.Hour} {
		p := filepath.Join(dir, string(rune('a'+i)))
		os.WriteFile(p, make([]byte, 100), 0o600)
		then := time.Now().Add(-age)
		os.Chtimes(p, then, then)
	}
	trimThumbs(dir, 250)
	left, _ := os.ReadDir(dir)
	if len(left) != 1 || left[0].Name() != "b" {
		t.Errorf("left %v", left)
	}
}

// TestThumbsNewestFirst fetches the thumbnails asked for last first: the
// rows on screen after a fast scroll, not those scrolled past.
func TestThumbsNewestFirst(t *testing.T) {
	th := &thumbs{}
	th.more = sync.NewCond(&th.mu)
	for i := range 3 {
		th.pending = append(th.pending, thumbJob{key: thumbKey{fsid: int64(i)}})
	}
	for _, want := range []int64{2, 1, 0} {
		if job, _ := th.next(); job.key.fsid != want {
			t.Fatalf("took %d, want %d", job.key.fsid, want)
		}
	}
}
