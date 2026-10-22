package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

const (
	thumbCacheSize = 600
	thumbWorkers   = 6
	thumbQueueSize = 4096
	// smallThumb is the largest a thumbnail shows that is fetched small
	// (about 140×90); larger ones, as the grid's, are fetched at about
	// 360×270, never the netdisk's 850×580, which took 2 MB of memory each.
	smallThumb = 64
	// thumbDiskLimit bounds the thumbnails kept on disk across launches.
	thumbDiskLimit = 256 << 20
)

type thumbState int

const (
	thumbQueued thumbState = iota
	thumbReady
	thumbFailed
)

// thumbKey names a thumbnail by the file, its version and its size, not
// by its URL: the netdisk signs thumbnail URLs afresh with every listing,
// so a folder read again (after a file in it is deleted, say) would fetch
// them all again.
type thumbKey struct {
	fsid, mtime, size int64
	small             bool
}

func thumbKeyOf(f *baidu.File, small bool) thumbKey {
	return thumbKey{f.FsID, f.Mtime, f.Size, small}
}

type thumbEntry struct {
	state thumbState
	bmp   *ui.Bitmap
	used  uint64
	// wanted is the last frame built that showed it, while it waits.
	wanted *atomic.Uint64
}

// thumbs fetches thumbnails into memory as rows show them, a few at a
// time, the ones asked for last first, keeping the most recently shown in
// memory and more on disk.
type thumbs struct {
	entries map[thumbKey]*thumbEntry
	tick    uint64
	frame   atomic.Uint64 // frames built
	dir     string        // where thumbnails are kept on disk, "" for nowhere
	written atomic.Int32  // thumbnails written to disk since it was trimmed
	cancel  context.CancelFunc

	mu      sync.Mutex
	more    *sync.Cond
	pending []thumbJob // waiting, the newest last
	closed  bool
}

type thumbJob struct {
	key    thumbKey
	url    string
	c      *baidu.Client
	wanted *atomic.Uint64
}

// thumbDir is where thumbnails are kept on disk, "" when nowhere.
func (a *App) thumbDir() string {
	if a.fake {
		return ""
	}
	dir, err := mygo.App.Path(mygo.PathCache)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "thumbs")
}

// startThumbs starts the workers fetching thumbnails for a session.
func (a *App) startThumbs() {
	ctx, cancel := context.WithCancel(context.Background())
	th := &thumbs{entries: map[thumbKey]*thumbEntry{}, cancel: cancel, dir: a.thumbDir()}
	th.more = sync.NewCond(&th.mu)
	a.thumbs = th
	if th.dir != "" {
		os.MkdirAll(th.dir, 0o700)
		go trimThumbs(th.dir, thumbDiskLimit)
	}
	for range thumbWorkers {
		go func() {
			for {
				job, ok := th.next()
				if !ok {
					return
				}
				// Rows scrolled past before their turn are not fetched;
				// they ask again if they show again.
				if job.wanted.Load()+1 < th.frame.Load() {
					a.update(func() {
						if e := th.entries[job.key]; e != nil && e.state == thumbQueued {
							delete(th.entries, job.key)
						}
					})
					continue
				}
				bmp := th.fetch(ctx, job)
				a.update(func() {
					if e := th.entries[job.key]; e != nil {
						if bmp != nil {
							e.state, e.bmp = thumbReady, bmp
						} else {
							e.state = thumbFailed
						}
					}
				})
			}
		}()
	}
}

func (a *App) stopThumbs() {
	if th := a.thumbs; th != nil {
		th.cancel()
		th.mu.Lock()
		th.closed = true
		th.mu.Unlock()
		th.more.Broadcast()
		a.thumbs = nil
	}
}

// forgetThumbs deletes the thumbnails kept on disk, as the account signs
// out.
func (a *App) forgetThumbs() {
	if dir := a.thumbDir(); dir != "" {
		os.RemoveAll(dir)
	}
}

// next waits for a job, taking the newest; it reports false once the
// thumbnails stop.
func (th *thumbs) next() (thumbJob, bool) {
	th.mu.Lock()
	defer th.mu.Unlock()
	for len(th.pending) == 0 && !th.closed {
		th.more.Wait()
	}
	if th.closed {
		return thumbJob{}, false
	}
	job := th.pending[len(th.pending)-1]
	th.pending = th.pending[:len(th.pending)-1]
	return job, true
}

// fetch reads a thumbnail from disk, else from the netdisk, keeping it on
// disk.
func (th *thumbs) fetch(ctx context.Context, job thumbJob) *ui.Bitmap {
	file := th.file(job.key)
	if file != "" {
		if data, err := os.ReadFile(file); err == nil {
			if bmp, err := ui.DecodeBitmap(data); err == nil {
				now := time.Now()
				os.Chtimes(file, now, now) // recently used, kept longest
				return bmp
			}
		}
	}
	resp, err := job.c.Fetch(ctx, job.url, 0, 0)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	data, err := readAll(resp.Body, 8<<20)
	if err != nil {
		return nil
	}
	bmp, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil
	}
	if file != "" {
		tmp := file + ".tmp"
		if os.WriteFile(tmp, data, 0o600) == nil {
			os.Rename(tmp, file)
		}
		if th.written.Add(1)%200 == 0 {
			go trimThumbs(th.dir, thumbDiskLimit)
		}
	}
	return bmp
}

// file is where a thumbnail is kept on disk, "" when nowhere.
func (th *thumbs) file(k thumbKey) string {
	if th.dir == "" {
		return ""
	}
	size := "m"
	if k.small {
		size = "s"
	}
	return filepath.Join(th.dir, fmt.Sprintf("%d-%d-%d-%s", k.fsid, k.mtime, k.size, size))
}

// trimThumbs deletes the thumbnails used longest ago while those in dir
// take more than limit bytes, down to three quarters of it.
func trimThumbs(dir string, limit int64) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type kept struct {
		name string
		size int64
		used time.Time
	}
	var all []kept
	total := int64(0)
	for _, e := range ents {
		if info, err := e.Info(); err == nil && info.Mode().IsRegular() {
			all = append(all, kept{e.Name(), info.Size(), info.ModTime()})
			total += info.Size()
		}
	}
	if total <= limit {
		return
	}
	slices.SortFunc(all, func(x, y kept) int { return x.used.Compare(y.used) })
	for _, k := range all {
		if total <= limit*3/4 {
			break
		}
		if os.Remove(filepath.Join(dir, k.name)) == nil {
			total -= k.size
		}
	}
}

// thumb returns the thumbnail of a file to show at size points, nil until
// it has arrived, asking for it the first time.
func (a *App) thumb(f *baidu.File, size float32) *ui.Bitmap {
	th := a.thumbs
	if th == nil || f.Thumb == "" || a.sess == nil {
		return nil
	}
	th.tick++
	small := size <= smallThumb && f.ThumbSmall != ""
	key := thumbKeyOf(f, small)
	frame := th.frame.Load()
	// A small one at hand shows while the larger comes.
	stand := func() *ui.Bitmap {
		if e := th.entries[thumbKeyOf(f, true)]; !small && e != nil {
			e.used = th.tick
			return e.bmp
		}
		return nil
	}
	if e, ok := th.entries[key]; ok {
		e.used = th.tick
		if e.wanted != nil {
			e.wanted.Store(frame)
		}
		if e.bmp == nil {
			return stand()
		}
		return e.bmp
	}
	wanted := new(atomic.Uint64)
	wanted.Store(frame)
	th.entries[key] = &thumbEntry{used: th.tick, wanted: wanted}
	url := f.Thumb
	if small {
		url = f.ThumbSmall
	}
	th.mu.Lock()
	th.pending = append(th.pending, thumbJob{key: key, url: url, c: a.sess.client, wanted: wanted})
	if len(th.pending) > thumbQueueSize {
		// The oldest waiting are dropped, to be asked for again.
		for _, j := range th.pending[:len(th.pending)-thumbQueueSize] {
			delete(th.entries, j.key)
		}
		th.pending = slices.Delete(th.pending, 0, len(th.pending)-thumbQueueSize)
	}
	th.mu.Unlock()
	th.more.Signal()
	if len(th.entries) > thumbCacheSize {
		th.evict()
	}
	return stand()
}

// evict forgets the thumbnails shown longest ago.
func (th *thumbs) evict() {
	var oldest []thumbKey
	limit := th.tick - thumbCacheSize/2
	for k, e := range th.entries {
		if e.used < limit && e.state != thumbQueued {
			oldest = append(oldest, k)
		}
	}
	for _, k := range oldest {
		delete(th.entries, k)
	}
}
