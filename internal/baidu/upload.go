package baidu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pengstem/baidunetgo/internal/pcsport"
)

// ErrExists is what Upload returns when the same file is there already.
var ErrExists = pcsport.ErrExists

// ErrNoSpace is what Upload returns when the netdisk is full.
var ErrNoSpace = pcsport.ErrNoSpace

// UploadState is how far an upload went, to go on with it.
type UploadState = pcsport.Resume

// Upload sends a local file to remote, a path of the netdisk, overwriting
// what is there, and reports its progress. It goes on from where from
// says an earlier try stopped, if not nil, and tells note how far it goes.
func (c *Client) Upload(ctx context.Context, local, remote string, parallel int, progress func(done, total int64), from *UploadState, note func(UploadState)) error {
	if c.fake() {
		return c.fakeUpload(ctx, local, remote, progress, from, note)
	}
	if _, err := c.uid(); err != nil {
		return err
	}
	return pcsport.Upload(ctx, c.pcs, local, remote, parallel, progress, from, note)
}

// fakeUpload sends the file to a pretend netdisk in 4 MB parts, then
// joins them, going on as Upload does from the parts sent before.
func (c *Client) fakeUpload(ctx context.Context, local, remote string, progress func(done, total int64), from *UploadState, note func(UploadState)) error {
	if progress == nil {
		progress = func(int64, int64) {}
	}
	if note == nil {
		note = func(UploadState) {}
	}
	f, err := os.Open(local)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	const part = 4 << 20
	size, mtime := st.Size(), st.ModTime().UnixNano()
	n := int((size + part - 1) / part)
	var r UploadState
	resumed := from != nil && from.Size == size && from.Mtime == mtime && from.ID != "" && len(from.Parts) == n
	if resumed {
		r = *from
		r.Parts = append([]string(nil), from.Parts...)
	}
	for {
		if r.ID == "" {
			r = UploadState{Size: size, Mtime: mtime, ID: strconv.FormatInt(time.Now().UnixNano(), 36), Parts: make([]string, n)}
			note(r)
		}
		done := int64(0)
		for i, p := range r.Parts {
			if p != "" {
				done += min(part, size-int64(i)*part)
			}
		}
		progress(done, size)
		for i := range r.Parts {
			if r.Parts[i] != "" {
				continue
			}
			sec := io.NewSectionReader(f, int64(i)*part, min(part, size-int64(i)*part))
			body := &countingReader{r: sec, done: done, total: size, fn: progress}
			var res struct {
				MD5 string `json:"md5"`
			}
			if err := c.fakePost(ctx, url.Values{"id": {r.ID}, "part": {strconv.Itoa(i)}}, body, sec.Size(), &res); err != nil {
				return err
			}
			done += sec.Size()
			r.Parts[i] = res.MD5
			note(UploadState{Size: r.Size, Mtime: r.Mtime, ID: r.ID, Parts: append([]string(nil), r.Parts...)})
		}
		err := c.fakePost(ctx, url.Values{"id": {r.ID}, "path": {remote}, "parts": {strconv.Itoa(n)}}, http.NoBody, 0, nil)
		if err == nil || !resumed || ctx.Err() != nil {
			return err
		}
		// The netdisk let go of the parts: start afresh, once.
		resumed, r.ID = false, ""
	}
}

// fakePost posts body to the pretend netdisk's /upload with q.
func (c *Client) fakePost(ctx context.Context, q url.Values, body io.Reader, n int64, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/upload?"+q.Encode(), body)
	if err != nil {
		return err
	}
	req.ContentLength = n
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("上传失败：HTTP %d", resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

type countingReader struct {
	r     io.Reader
	done  int64
	total int64
	fn    func(done, total int64)
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	cr.done += int64(n)
	cr.fn(cr.done, cr.total)
	return n, err
}

// ProbeUpload measures uploads: it sends random data over parallel
// connections for up to d to an upload it never finishes, so nothing is
// kept, and returns the bytes sent and the requests that failed.
func (c *Client) ProbeUpload(ctx context.Context, parallel int, d time.Duration) (sent int64, errs int, err error) {
	if c.fake() {
		return c.fakeProbeUpload(ctx, parallel, d)
	}
	if _, err := c.uid(); err != nil {
		return 0, 0, err
	}
	return pcsport.ProbeUpload(ctx, c.pcs, "/.baidunetgo-probe", parallel, d)
}

// fakeProbeUpload posts random parts to the pretend netdisk, which throws
// them away.
func (c *Client) fakeProbeUpload(ctx context.Context, parallel int, d time.Duration) (sent int64, errs int, err error) {
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	part := make([]byte, 1<<20)
	var (
		total atomic.Int64
		mu    sync.Mutex
		wg    sync.WaitGroup
	)
	for range max(parallel, 1) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				body := &countingReader{r: bytes.NewReader(part), total: int64(len(part)), fn: func(int64, int64) {}}
				req, rerr := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/upload?probe=1", body)
				if rerr != nil {
					return
				}
				req.ContentLength = int64(len(part))
				resp, rerr := c.http.Do(req)
				if rerr == nil {
					resp.Body.Close()
					if resp.StatusCode != http.StatusOK {
						rerr = fmt.Errorf("上传失败：HTTP %d", resp.StatusCode)
					}
				}
				if rerr == nil {
					total.Add(body.done)
				} else if ctx.Err() == nil {
					mu.Lock()
					errs++
					err = rerr
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	return total.Load(), errs, err
}
