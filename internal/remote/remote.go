// Package remote reads files of the netdisk over HTTP range requests, into
// memory and never to disk: whole, for previews of small files, or as a
// seekable stream, for media played as it arrives.
package remote

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
)

// Source gives links to a file and reads ranges of them; baidu.Client is
// one.
type Source interface {
	DownloadURL(ctx context.Context, path string) (string, error)
	Fetch(ctx context.Context, url string, off, n int64) (*http.Response, error)
}

// Expired reports whether an error means a link must be asked for again.
type Expired func(error) bool

// ErrTooLarge is what ReadAll returns for a file over its limit.
var ErrTooLarge = errors.New("文件太大，无法预览")

// Parallel is how many ranges ReadAll fetches at once.
var Parallel = 4

// ReadAll reads a whole file into memory, calling progress with the bytes
// read so far, and fails with ErrTooLarge past limit bytes. The netdisk
// answers only bounded ranges, so it reads the file in chunks, a few at
// a time.
func ReadAll(ctx context.Context, src Source, expired Expired, path string, size, limit int64, progress func(done int64)) ([]byte, error) {
	if size > limit {
		return nil, ErrTooLarge
	}
	if size == 0 {
		return []byte{}, nil
	}
	f := Open(ctx, src, expired, path, size)
	buf := make([]byte, size)
	n := int((size + chunkSize - 1) / chunkSize)
	var (
		mu    sync.Mutex
		done  int64
		first error
		next  int
		wg    sync.WaitGroup
	)
	for range min(max(Parallel, 1), n) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				mu.Lock()
				i := next
				next++
				stop := first != nil
				mu.Unlock()
				if i >= n || stop {
					return
				}
				c, err := f.fetchChunk(int64(i))
				mu.Lock()
				if err != nil {
					if first == nil {
						first = err
					}
					mu.Unlock()
					return
				}
				copy(buf[int64(i)*chunkSize:], c)
				done += int64(len(c))
				d := done
				mu.Unlock()
				if progress != nil {
					progress(d)
				}
			}
		}()
	}
	wg.Wait()
	if first != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, first
	}
	return buf, nil
}

const chunkSize = 512 << 10

// File is a netdisk file read as it is needed, in chunks kept in memory.
// It is an io.ReadSeeker and an io.ReaderAt, safe from any goroutine.
type File struct {
	ctx     context.Context
	src     Source
	expired Expired
	path    string
	size    int64

	mu     sync.Mutex
	url    string
	pos    int64
	chunks map[int64][]byte
	order  []int64
}

// Open returns a File of the netdisk file of that path and size.
func Open(ctx context.Context, src Source, expired Expired, path string, size int64) *File {
	return &File{ctx: ctx, src: src, expired: expired, path: path, size: size, chunks: map[int64][]byte{}}
}

// Size returns the file's size.
func (f *File) Size() int64 { return f.size }

// Read reads from the current position.
func (f *File) Read(p []byte) (int, error) {
	f.mu.Lock()
	pos := f.pos
	f.mu.Unlock()
	n, err := f.ReadAt(p, pos)
	f.mu.Lock()
	f.pos += int64(n)
	f.mu.Unlock()
	if err == io.EOF && n > 0 {
		err = nil
	}
	return n, err
}

// Seek moves the position, as io.Seeker does.
func (f *File) Seek(off int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch whence {
	case io.SeekCurrent:
		off += f.pos
	case io.SeekEnd:
		off += f.size
	}
	if off < 0 {
		return 0, errors.New("remote: negative position")
	}
	f.pos = off
	return off, nil
}

// ReadAt reads len(p) bytes from off.
func (f *File) ReadAt(p []byte, off int64) (int, error) {
	n := 0
	for n < len(p) {
		if off+int64(n) >= f.size {
			return n, io.EOF
		}
		idx := (off + int64(n)) / chunkSize
		c, err := f.chunk(idx)
		if err != nil {
			return n, err
		}
		within := off + int64(n) - idx*chunkSize
		n += copy(p[n:], c[within:])
	}
	return n, nil
}

// chunk returns chunk idx, fetching it when it is not kept.
func (f *File) chunk(idx int64) ([]byte, error) {
	f.mu.Lock()
	if c, ok := f.chunks[idx]; ok {
		f.mu.Unlock()
		return c, nil
	}
	f.mu.Unlock()
	data, err := f.fetchChunk(idx)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chunks[idx] = data
	f.order = append(f.order, idx)
	if len(f.order) > 32 { // keep 16 MB
		delete(f.chunks, f.order[0])
		f.order = f.order[1:]
	}
	return data, nil
}

// fetchChunk reads chunk idx with a bounded range, asking for a new link
// when the one it has expired.
func (f *File) fetchChunk(idx int64) ([]byte, error) {
	f.mu.Lock()
	u := f.url
	f.mu.Unlock()

	start := idx * chunkSize
	n := min(int64(chunkSize), f.size-start)
	var data []byte
	for attempt := 0; ; attempt++ {
		if u == "" {
			var err error
			if u, err = f.src.DownloadURL(f.ctx, f.path); err != nil {
				return nil, err
			}
		}
		resp, err := f.src.Fetch(f.ctx, u, start, n)
		if err == nil {
			data, err = io.ReadAll(io.LimitReader(resp.Body, n))
			resp.Body.Close()
			if err == nil && int64(len(data)) == n {
				break
			}
			if err == nil {
				err = io.ErrUnexpectedEOF
			}
		}
		if attempt >= 2 || f.ctx.Err() != nil {
			return nil, err
		}
		if f.expired != nil && f.expired(err) {
			u = ""
		}
	}

	f.mu.Lock()
	f.url = u
	f.mu.Unlock()
	return data, nil
}

// Close forgets what was read.
func (f *File) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chunks = map[int64][]byte{}
	f.order = nil
	return nil
}
