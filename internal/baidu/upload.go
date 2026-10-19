package baidu

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

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
