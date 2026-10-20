package baidu

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pengstem/baidunetgo/internal/pcsport"
)

// ErrExists is what Upload returns when the same file is there already.
var ErrExists = pcsport.ErrExists

// Upload sends a local file to remote, a path of the netdisk, overwriting
// what is there, and reports its progress.
func (c *Client) Upload(ctx context.Context, local, remote string, parallel int, progress func(done, total int64)) error {
	if c.fake() {
		return c.fakeUpload(ctx, local, remote, progress)
	}
	if _, err := c.uid(); err != nil {
		return err
	}
	return pcsport.Upload(ctx, c.pcs, local, remote, parallel, progress)
}

// fakeUpload posts the file to a pretend netdisk.
func (c *Client) fakeUpload(ctx context.Context, local, remote string, progress func(done, total int64)) error {
	f, err := os.Open(local)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	var body io.Reader = f
	if progress != nil {
		body = &countingReader{r: f, total: st.Size(), fn: progress}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/upload?"+url.Values{"path": {remote}}.Encode(), body)
	if err != nil {
		return err
	}
	req.ContentLength = st.Size()
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("上传失败：HTTP %d", resp.StatusCode)
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
