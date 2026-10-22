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
	"slices"
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

// Resume is how far the upload of a local file went, kept so that it goes
// on where it stopped: the file's sums, taken once, and the parts the
// netdisk has of it.
type Resume struct {
	Size   int64    `json:"size"`
	Mtime  int64    `json:"mtime"` // of the file the sums are of, in ns
	MD5    string   `json:"md5"`
	Slice  string   `json:"slice"`
	CRC32  uint32   `json:"crc32"`
	Blocks []string `json:"blocks,omitempty"`
	// ID is the upload's at the netdisk, and Parts the MD5 it gave each
	// part sent, "" for those not sent yet.
	ID    string   `json:"id,omitempty"`
	Parts []string `json:"parts,omitempty"`
}

func (r Resume) clone() Resume {
	r.Parts = slices.Clone(r.Parts)
	return r
}

// Upload sends a local file to savePath of the netdisk, by its checksums
// when the netdisk already has its content. from, when not nil, is how
// far an earlier try went, which it goes on from; note is told how far
// this one goes as it does.
func Upload(ctx context.Context, pcs *baidupcs.BaiduPCS, local, savePath string, parallel int, progress Progress, from *Resume, note func(Resume)) error {
	f, err := os.Open(local)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	nd := &pcsDisk{pcs: pcs, target: savePath, parts: &partUpload{pcs: pcs, target: savePath, client: uploadClient(pcs)}}
	return upload(ctx, nd, f, info.Size(), info.ModTime().UnixNano(), parallel, progress, from, note)
}

// disk is what an upload needs of the netdisk.
type disk interface {
	// precreate links the content when the netdisk has it (rapid true),
	// else returns the id to send its parts to, going on with id when
	// it is not "".
	precreate(f io.ReaderAt, r *Resume, id string) (rapid bool, newID string, err error)
	part(ctx context.Context, id string, seq int, off int64, r rio.ReaderLen64) (string, error)
	create(id string, size int64, parts []string) error
}

func upload(ctx context.Context, nd disk, f io.ReaderAt, size, mtime int64, parallel int, progress Progress, from *Resume, note func(Resume)) error {
	if progress == nil {
		progress = func(int64, int64) {}
	}
	if note == nil {
		note = func(Resume) {}
	}
	progress(0, size)
	block := blockSizeFor(size)
	var r Resume
	if from != nil && from.Size == size && from.Mtime == mtime && (size == 0 || len(from.Blocks) > 0) {
		r = from.clone()
	} else {
		sum, err := checksums(ctx, f, size, block)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("计算校验和失败：%w", err)
		}
		r = Resume{Size: size, Mtime: mtime, MD5: hex.EncodeToString(sum.md5), Slice: hex.EncodeToString(sum.slice), CRC32: sum.crc32, Blocks: sum.blocks}
		note(r.clone())
	}
	resumed := r.ID != ""
	for {
		err := send(ctx, nd, f, &r, block, parallel, progress, note)
		if err == nil || !resumed || ctx.Err() != nil || errors.Is(err, ErrExists) || errors.Is(err, errNoSpace) {
			return err
		}
		// The netdisk may have let go of an upload left too long: start
		// it afresh, once.
		resumed = false
		r.ID, r.Parts = "", nil
		note(r.clone())
	}
}

var errNoSpace = errors.New("网盘空间不足")

// send links the file or sends the parts the netdisk lacks, then joins
// them.
func send(ctx context.Context, nd disk, f io.ReaderAt, r *Resume, block int64, parallel int, progress Progress, note func(Resume)) error {
	rapid, id, err := nd.precreate(f, r, r.ID)
	if err != nil {
		return err
	}
	if rapid {
		progress(r.Size, r.Size)
		return nil
	}
	n := int((r.Size + block - 1) / block)
	if r.ID == "" || len(r.Parts) != n {
		r.ID, r.Parts = id, make([]string, n)
		note(r.clone())
	}
	if err := sendParts(ctx, nd, f, r, block, parallel, progress, note); err != nil {
		return err
	}
	if err := nd.create(r.ID, r.Size, r.Parts); err != nil {
		return fmt.Errorf("上传失败：%w", err)
	}
	progress(r.Size, r.Size)
	return nil
}

// sendParts sends the parts not sent yet over parallel connections,
// noting each as the netdisk has it.
func sendParts(ctx context.Context, nd disk, f io.ReaderAt, r *Resume, block int64, parallel int, progress Progress, note func(Resume)) error {
	partLen := func(i int) int64 { return min(block, r.Size-int64(i)*block) }
	var sent atomic.Int64
	for i, p := range r.Parts {
		if p != "" {
			sent.Add(partLen(i))
		}
	}
	progress(sent.Load(), r.Size)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		mu       sync.Mutex
		next     int
		firstErr error
		wg       sync.WaitGroup
	)
	take := func() int {
		mu.Lock()
		defer mu.Unlock()
		for next < len(r.Parts) && r.Parts[next] != "" {
			next++
		}
		if next >= len(r.Parts) || firstErr != nil {
			return -1
		}
		next++
		return next - 1
	}
	for range max(parallel, 1) {
		wg.Go(func() {
			for {
				i := take()
				if i < 0 {
					return
				}
				sum, err := sendPart(ctx, nd, f, r.ID, i, int64(i)*block, partLen(i), &sent, func() { progress(sent.Load(), r.Size) })
				mu.Lock()
				if err != nil {
					if firstErr == nil && ctx.Err() == nil {
						firstErr = err
					}
					mu.Unlock()
					cancel()
					return
				}
				r.Parts[i] = sum
				note(r.clone())
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if firstErr != nil {
		return fmt.Errorf("上传失败：%w", firstErr)
	}
	return context.Cause(ctx)
}

// sendPart sends one part, trying again after a pause when it fails,
// unless the netdisk refused it outright.
func sendPart(ctx context.Context, nd disk, f io.ReaderAt, id string, seq int, off, n int64, sent *atomic.Int64, tell func()) (string, error) {
	var lastErr error
	for attempt := range 5 {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		cr := &countReader{r: io.NewSectionReader(f, off, n), n: n, sent: sent, tell: tell}
		sum, err := nd.part(ctx, id, seq, off, cr)
		if err == nil {
			sent.Add(n - cr.read)
			tell()
			return sum, nil
		}
		sent.Add(-cr.read)
		tell()
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		lastErr = err
		var me *uploader.MultiError
		if errors.As(err, &me) && me.Terminated {
			return "", err
		}
	}
	return "", lastErr
}

// pcsDisk is the netdisk of a BaiduPCS session.
type pcsDisk struct {
	pcs    *baidupcs.BaiduPCS
	target string
	parts  *partUpload
}

func (d *pcsDisk) precreate(f io.ReaderAt, r *Resume, id string) (bool, string, error) {
	uk, perr := d.pcs.CacheUK()
	if perr != nil {
		return false, "", fmt.Errorf("获取用户信息失败：%w", perr)
	}
	now := time.Now().Unix()
	offset, err := dataOffset(r.MD5, uk, now, r.Size, sliceContent)
	if err != nil {
		return false, "", err
	}
	content := make([]byte, sliceContent)
	n, err := f.ReadAt(content, offset)
	if err != nil && err != io.EOF {
		return false, "", err
	}
	content, length := content[:n], int64(n)
	b64 := strings.TrimRight(base64.StdEncoding.EncodeToString(content), "=")
	perr, res := d.pcs.RapidUpload(d.target, baidupcs.OverWritePolicy, id, r.MD5, r.Slice, b64,
		strconv.FormatUint(uint64(r.CRC32), 10), offset, length, r.Size, now, r.Blocks)
	if perr != nil {
		switch perr.GetRemoteErrCode() {
		case 114514, 1919810:
			return false, "", ErrExists
		case 31112:
			return false, "", errNoSpace
		}
		return false, "", fmt.Errorf("上传失败：%w", perr)
	}
	return res.ReturnType == 2, res.UploadID, nil
}

func (d *pcsDisk) part(ctx context.Context, id string, seq int, off int64, r rio.ReaderLen64) (string, error) {
	return d.parts.TmpFile(ctx, id, d.target, seq, off, r)
}

func (d *pcsDisk) create(id string, size int64, parts []string) error {
	sums := make(map[int]string, len(parts))
	for i, p := range parts {
		sums[i] = p
	}
	return d.parts.CreateSuperFile(d.pcs.GetPCSAddr(), baidupcs.OverWritePolicy, id, size, sums)
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
	read int64  // what it counted
	tell func() // told of each read, if not nil
}

func (c *countReader) Read(p []byte) (int, error) {
	k, err := c.r.Read(p)
	c.read += int64(k)
	c.sent.Add(int64(k))
	if c.tell != nil {
		c.tell()
	}
	return k, err
}

func (c *countReader) Len() int64 { return c.n }
