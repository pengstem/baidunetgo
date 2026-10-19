// Package stream serves netdisk files to players over HTTP on the loopback
// interface. The netdisk's download hosts answer only bounded range
// requests (bytes=a-b), with its user agent and cookies; players ask for
// open-ended ranges. The server turns each request into bounded ones,
// fetched a few at a time ahead of the player, and asks for a new link
// when one expires. Nothing is written to disk.
package stream

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pengstem/baidunetgo/internal/remote"
)

const chunk = 2 << 20

// Server serves registered files.
type Server struct {
	src     remote.Source
	expired remote.Expired
	ln      net.Listener
	srv     *http.Server

	mu    sync.Mutex
	files map[string]*file
	// Ahead is how many chunks are fetched at once ahead of the player.
	Ahead int
}

type file struct {
	path string
	size int64
	name string

	mu   sync.Mutex
	link string
}

// Start listens on a random port of 127.0.0.1.
func Start(src remote.Source, expired remote.Expired) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{src: src, expired: expired, ln: ln, files: map[string]*file{}, Ahead: 4}
	s.srv = &http.Server{Handler: s, ReadHeaderTimeout: 10 * time.Second}
	go s.srv.Serve(ln)
	return s, nil
}

// Close stops serving.
func (s *Server) Close() error { return s.srv.Close() }

// URL registers a netdisk file and returns the address a player reads it
// at. The address holds a random token: other programs cannot guess it.
func (s *Server) URL(p string, size int64) string {
	b := make([]byte, 16)
	rand.Read(b)
	tok := hex.EncodeToString(b)
	s.mu.Lock()
	s.files[tok] = &file{path: p, size: size, name: path.Base(p)}
	s.mu.Unlock()
	return "http://" + s.ln.Addr().String() + "/" + tok + "/" + url.PathEscape(path.Base(p))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tok, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	s.mu.Lock()
	f := s.files[tok]
	s.mu.Unlock()
	if f == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		http.NotFound(w, r)
		return
	}
	start, end, partial, ok := parseRange(r.Header.Get("Range"), f.size)
	if !ok {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", f.size))
		http.Error(w, "range not satisfiable", http.StatusRequestedRangeNotSatisfiable)
		return
	}
	h := w.Header()
	h.Set("Accept-Ranges", "bytes")
	if ct := mime.TypeByExtension(path.Ext(f.name)); ct != "" {
		h.Set("Content-Type", ct)
	} else {
		h.Set("Content-Type", "application/octet-stream")
	}
	h.Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	if partial {
		h.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, f.size))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	if r.Method == http.MethodHead || f.size == 0 {
		return
	}
	s.copy(r.Context(), w, f, start, end)
}

// parseRange reads a single range of a Range header: bytes=a-b, a-, or
// -n. No header means the whole file.
func parseRange(h string, size int64) (start, end int64, partial, ok bool) {
	if h == "" {
		return 0, size - 1, false, true
	}
	spec, found := strings.CutPrefix(h, "bytes=")
	if !found || strings.Contains(spec, ",") {
		return 0, 0, false, false
	}
	a, b, _ := strings.Cut(spec, "-")
	switch {
	case a == "":
		n, err := strconv.ParseInt(b, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false, false
		}
		start, end = max(size-n, 0), size-1
	default:
		var err error
		if start, err = strconv.ParseInt(a, 10, 64); err != nil {
			return 0, 0, false, false
		}
		end = size - 1
		if b != "" {
			if end, err = strconv.ParseInt(b, 10, 64); err != nil {
				return 0, 0, false, false
			}
			end = min(end, size-1)
		}
	}
	if start < 0 || start >= size || end < start {
		return 0, 0, false, false
	}
	return start, end, true, true
}

// copy writes bytes [start, end] of f to w, fetching chunks ahead in
// parallel and writing them in order.
func (s *Server) copy(ctx context.Context, w io.Writer, f *file, start, end int64) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		data []byte
		err  error
	}
	ahead := max(s.Ahead, 1)
	var slots []chan result
	next := start
	launch := func() {
		if next > end {
			return
		}
		off, n := next, min(chunk, end-next+1)
		next += n
		ch := make(chan result, 1)
		slots = append(slots, ch)
		go func() {
			data, err := s.fetch(ctx, f, off, n)
			ch <- result{data, err}
		}()
	}
	for range ahead {
		launch()
	}
	for len(slots) > 0 {
		res := <-slots[0]
		slots = slots[1:]
		if res.err != nil {
			return
		}
		if _, err := w.Write(res.data); err != nil {
			return
		}
		launch()
	}
}

// fetch reads n bytes at off with a bounded range, asking for a new link
// when the one it has expired.
func (s *Server) fetch(ctx context.Context, f *file, off, n int64) ([]byte, error) {
	var lastErr error
	for attempt := range 4 {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 400 * time.Millisecond):
			}
		}
		f.mu.Lock()
		link := f.link
		f.mu.Unlock()
		if link == "" {
			l, err := s.src.DownloadURL(ctx, f.path)
			if err != nil {
				lastErr = err
				continue
			}
			f.mu.Lock()
			f.link, link = l, l
			f.mu.Unlock()
		}
		resp, err := s.src.Fetch(ctx, link, off, n)
		if err != nil {
			lastErr = err
			if s.expired != nil && s.expired(err) {
				f.mu.Lock()
				if f.link == link {
					f.link = ""
				}
				f.mu.Unlock()
			}
			continue
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, n))
		resp.Body.Close()
		if err == nil && int64(len(data)) == n {
			return data, nil
		}
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		lastErr = err
	}
	return nil, lastErr
}
