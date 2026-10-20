package app

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

const (
	thumbCacheSize = 600
	thumbWorkers   = 6
)

type thumbState int

const (
	thumbQueued thumbState = iota
	thumbReady
	thumbFailed
)

// thumbKey names a thumbnail by the file and its version, not by its URL:
// the netdisk signs thumbnail URLs afresh with every listing, so a folder
// read again (after a file in it is deleted, say) would fetch them all
// again.
type thumbKey struct {
	fsid, mtime, size int64
}

func thumbKeyOf(f *baidu.File) thumbKey { return thumbKey{f.FsID, f.Mtime, f.Size} }

type thumbEntry struct {
	state thumbState
	bmp   *ui.Bitmap
	used  uint64
}

// thumbs fetches thumbnails into memory as rows show them, a few at a
// time, keeping the most recently shown.
type thumbs struct {
	entries map[thumbKey]*thumbEntry
	tick    uint64
	queue   chan thumbJob
	cancel  context.CancelFunc
}

type thumbJob struct {
	key thumbKey
	url string
	c   *baidu.Client
}

// startThumbs starts the workers fetching thumbnails for a session.
func (a *App) startThumbs() {
	ctx, cancel := context.WithCancel(context.Background())
	a.thumbs = &thumbs{entries: map[thumbKey]*thumbEntry{}, queue: make(chan thumbJob, 4096), cancel: cancel}
	th := a.thumbs
	for range thumbWorkers {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case job := <-th.queue:
					bmp := fetchThumb(ctx, job)
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
			}
		}()
	}
}

func (a *App) stopThumbs() {
	if a.thumbs != nil {
		a.thumbs.cancel()
		a.thumbs = nil
	}
}

func fetchThumb(ctx context.Context, job thumbJob) *ui.Bitmap {
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
	return bmp
}

// thumb returns the thumbnail of a file, nil until it has arrived, asking
// for it the first time.
func (a *App) thumb(f *baidu.File) *ui.Bitmap {
	th := a.thumbs
	if th == nil || f.Thumb == "" || a.sess == nil {
		return nil
	}
	th.tick++
	key := thumbKeyOf(f)
	if e, ok := th.entries[key]; ok {
		e.used = th.tick
		return e.bmp
	}
	th.entries[key] = &thumbEntry{used: th.tick}
	select {
	case th.queue <- thumbJob{key: key, url: f.Thumb, c: a.sess.client}:
	default:
		delete(th.entries, key) // try again in a later frame
	}
	if len(th.entries) > thumbCacheSize {
		th.evict()
	}
	return nil
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
