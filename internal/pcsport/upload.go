// Package pcsport holds the upload logic of BaiduPCS-Go
// (https://github.com/qjfoidnh/BaiduPCS-Go, Apache License 2.0), which
// lives in its internal packages and cannot be imported: rapid upload by
// checksums, then the file in parts, then the parts joined. It is adapted
// from internal/pcsfunctions/pcsupload to run without the CLI's global
// configuration and to report its progress.
package pcsport

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/pcserror"
	"github.com/qjfoidnh/BaiduPCS-Go/requester"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/multipartreader"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/rio"
	"github.com/qjfoidnh/BaiduPCS-Go/requester/uploader"
)

// Progress reports bytes sent of the total.
type Progress func(done, total int64)

// ErrExists is what Upload returns when the netdisk already holds the same
// file there.
var ErrExists = errors.New("网盘中已有同样的文件")

const sliceContent = 4 << 10

// Upload sends a local file to savePath of the netdisk, by its checksums
// when the netdisk already has its content.
func Upload(ctx context.Context, pcs *baidupcs.BaiduPCS, local, savePath string, parallel int, progress Progress) error {
	f, err := os.Open(local)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	size := info.Size()
	if progress != nil {
		progress(0, size)
	}

	// Rapid upload: the netdisk links content it already has.
	blockSize := blockSizeFor(size)
	sum, err := checksums(ctx, f, size, blockSize)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("计算校验和失败：%w", err)
	}
	uk, perr := pcs.CacheUK()
	if perr != nil {
		return fmt.Errorf("获取用户信息失败：%w", perr)
	}
	md5hex := hex.EncodeToString(sum.md5)
	now := time.Now().Unix()
	offset, err := dataOffset(md5hex, uk, now, size, sliceContent)
	if err != nil {
		return err
	}
	content := make([]byte, sliceContent)
	n, err := f.ReadAt(content, offset)
	if err != nil && err != io.EOF {
		return err
	}
	content, length := content[:n], int64(n)
	b64 := strings.TrimRight(base64.StdEncoding.EncodeToString(content), "=")
	perr, res := pcs.RapidUpload(savePath, baidupcs.OverWritePolicy, "", md5hex, hex.EncodeToString(sum.slice), b64,
		strconv.FormatUint(uint64(sum.crc32), 10), offset, length, size, now, sum.blocks)
	if perr != nil {
		switch perr.GetRemoteErrCode() {
		case 114514, 1919810:
			return ErrExists
		case 31112:
			return errors.New("网盘空间不足")
		}
		return fmt.Errorf("上传失败：%w", perr)
	}
	if res.ReturnType == 2 {
		if progress != nil {
			progress(size, size)
		}
		return nil
	}

	// The parts, then joined.
	mu := uploader.NewMultiUploader(&partUpload{pcs: pcs, target: savePath, client: uploadClient(pcs)},
		rio.NewFileReaderAtLen64(f),
		&uploader.MultiUploaderConfig{Parallel: max(parallel, 1), BlockSize: blockSize, Policy: baidupcs.OverWritePolicy}, savePath)
	mu.SetInstanceState(&uploader.InstanceState{Uploadid: res.UploadID})
	var upErr error
	mu.OnError(func(err error) { upErr = err })
	if progress != nil {
		mu.OnUploadStatusEvent(func(st uploader.Status, _ <-chan struct{}) {
			progress(st.Uploaded(), st.TotalSize())
		})
	}
	stop := context.AfterFunc(ctx, mu.Cancel)
	defer stop()
	mu.Execute()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if upErr != nil {
		return fmt.Errorf("上传失败：%w", upErr)
	}
	if progress != nil {
		progress(size, size)
	}
	return nil
}

// partUpload sends parts to the PCS host of the session, whose
// certificate is checked.
type partUpload struct {
	pcs    *baidupcs.BaiduPCS
	target string
	client *requester.HTTPClient
}

// uploadClient sends parts over the session's transport, with a longer
// timeout for a whole part.
func uploadClient(pcs *baidupcs.BaiduPCS) *requester.HTTPClient {
	c := requester.NewHTTPClient()
	c.SetCookiejar(pcs.GetClient().Jar)
	c.SetTimeout(200 * time.Second)
	c.SetKeepAlive(true) // makes the library's transport, which we replace
	c.Client.Transport = pcs.GetClient().Client.Transport
	return c
}

func (pu *partUpload) Precreate() (string, pcserror.Error) {
	return pu.pcs.GetPCSAddr(), nil
}

func (pu *partUpload) TmpFile(ctx context.Context, uploadID, target string, seq int, off int64, r rio.ReaderLen64) (string, error) {
	var hardErr *uploader.MultiError
	sum, perr := pu.pcs.UploadTmpFile(uploadID, target, seq, off, func(u string, jar http.CookieJar) (*http.Response, error) {
		mr := multipartreader.NewMultipartReader()
		mr.AddFormFile("uploadedfile", "", r)
		mr.CloseMultipart()
		type result struct {
			resp *http.Response
			err  error
		}
		done := make(chan result, 1)
		go func() {
			resp, err := pu.client.Req(http.MethodPost, u, mr, nil)
			if resp != nil {
				switch resp.StatusCode {
				case 400, 401, 403, 413:
					hardErr = &uploader.MultiError{Terminated: true}
				}
			}
			done <- result{resp, err}
		}()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case r := <-done:
			return r.resp, r.err
		}
	})
	if hardErr != nil {
		hardErr.Err = perr
		return sum, hardErr
	}
	if perr != nil {
		return sum, perr
	}
	return sum, nil
}

func (pu *partUpload) CreateSuperFile(host, policy, uploadID string, size int64, sums map[int]string) error {
	pu.pcs.SetPCSAddr(host)
	if perr := pu.pcs.UploadCreateSuperFile(uploadID, policy, size, pu.target, sums); perr != nil {
		return perr
	}
	return nil
}

func blockSizeFor(size int64) int64 {
	switch {
	case size >= baidupcs.MaxUploadThreshold:
		return baidupcs.MaxUploadBlockSize
	case size >= baidupcs.MiddleUploadThreshold:
		return baidupcs.MiddleUploadBlockSize
	}
	return baidupcs.MinUploadBlockSize
}

// dataOffset is where the netdisk asks for a slice of the content to
// prove a rapid upload has it.
func dataOffset(contentMD5 string, uk, t, size, sub int64) (int64, error) {
	h := md5.New()
	io.WriteString(h, fmt.Sprintf("%d%s%d", uk, contentMD5, t))
	mixed := hex.EncodeToString(h.Sum(nil))
	raw, err := strconv.ParseInt(mixed[:8], 16, 64)
	if err != nil {
		return 0, err
	}
	if size-sub+1 <= 1 {
		return 0, nil
	}
	return raw % (size - sub + 1), nil
}

// Dir returns the directory of a netdisk path, for making it first.
func Dir(p string) string { return path.Dir(p) }

// ProbeUpload sends random parts over parallel connections for up to d,
// to an upload it never joins, so the netdisk keeps nothing, and returns
// how many bytes went and the requests that failed.
func ProbeUpload(ctx context.Context, pcs *baidupcs.BaiduPCS, target string, parallel int, d time.Duration) (sent int64, errs int, lastErr error) {
	perr, res := pcs.FakeRapidUpload(target, baidupcs.OverWritePolicy, 2*baidupcs.MinUploadBlockSize)
	if perr != nil {
		return 0, 0, fmt.Errorf("上传测速失败：%w", perr)
	}
	if res.UploadID == "" {
		return 0, 0, errors.New("上传测速失败：没有拿到上传编号")
	}
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	pu := &partUpload{pcs: pcs, target: target, client: uploadClient(pcs)}
	part := make([]byte, baidupcs.MinUploadBlockSize)
	rand.Read(part)
	var (
		total atomic.Int64
		mu    sync.Mutex
		wg    sync.WaitGroup
	)
	for w := range max(parallel, 1) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for seq := w; ctx.Err() == nil; seq += parallel {
				r := &countReader{r: bytes.NewReader(part), n: int64(len(part)), sent: &total}
				if _, err := pu.TmpFile(ctx, res.UploadID, target, seq%2, int64(seq%2)*int64(len(part)), r); err != nil && ctx.Err() == nil {
					mu.Lock()
					errs++
					lastErr = err
					mu.Unlock()
					time.Sleep(300 * time.Millisecond)
				}
			}
		}()
	}
	wg.Wait()
	return total.Load(), errs, lastErr
}

// countReader is a part that counts its bytes as the request reads them.
type countReader struct {
	r    io.Reader
	n    int64
	sent *atomic.Int64
}

func (c *countReader) Read(p []byte) (int, error) {
	k, err := c.r.Read(p)
	c.sent.Add(int64(k))
	return k, err
}

func (c *countReader) Len() int64 { return c.n }
