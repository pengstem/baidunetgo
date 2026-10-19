package remote

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type source struct {
	srv     *httptest.Server
	links   atomic.Int32
	expires bool
}

func (s *source) DownloadURL(ctx context.Context, path string) (string, error) {
	n := s.links.Add(1)
	return fmt.Sprintf("%s/f?link=%d", s.srv.URL, n), nil
}

var errExpired = errors.New("expired")

func (s *source) Fetch(ctx context.Context, url string, off, n int64) (*http.Response, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	if n > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, off+n-1))
	} else if off > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", off))
	}
	resp, err := http.DefaultClient.Do(req)
	if err == nil && resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		return nil, errExpired
	}
	return resp, err
}

func newSource(data []byte, expireFirst bool) *source {
	s := &source{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if expireFirst && r.URL.Query().Get("link") == "1" {
			http.Error(w, "expired", http.StatusForbidden)
			return
		}
		http.ServeContent(w, r, "f", time.Time{}, bytes.NewReader(data))
	}))
	return s
}

func isExpired(err error) bool { return errors.Is(err, errExpired) }

func sample(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i * 7)
	}
	return b
}

func TestReadAll(t *testing.T) {
	data := sample(300_000)
	src := newSource(data, true)
	defer src.srv.Close()
	var last int64
	got, err := ReadAll(context.Background(), src, isExpired, "/f", int64(len(data)), 1<<20, func(n int64) { last = n })
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) || last != int64(len(data)) {
		t.Errorf("read %d bytes, progress %d", len(got), last)
	}
	if _, err := ReadAll(context.Background(), src, isExpired, "/f", int64(len(data)), 1000, nil); err != ErrTooLarge {
		t.Errorf("limit: %v", err)
	}
}

func TestFileSeeks(t *testing.T) {
	data := sample(3*chunkSize + 1234)
	src := newSource(data, true)
	defer src.srv.Close()
	f := Open(context.Background(), src, isExpired, "/f", int64(len(data)))
	buf := make([]byte, 5000)
	// Across a chunk boundary.
	if n, err := f.ReadAt(buf, chunkSize-2500); err != nil || n != 5000 || !bytes.Equal(buf, data[chunkSize-2500:chunkSize+2500]) {
		t.Fatalf("ReadAt: %d %v", n, err)
	}
	if _, err := f.Seek(-100, io.SeekEnd); err != nil {
		t.Fatal(err)
	}
	rest, err := io.ReadAll(f)
	if err != nil || !bytes.Equal(rest, data[len(data)-100:]) {
		t.Fatalf("tail: %d %v", len(rest), err)
	}
	f.Seek(0, io.SeekStart)
	all, err := io.ReadAll(f)
	if err != nil || !bytes.Equal(all, data) {
		t.Fatalf("all: %d %v", len(all), err)
	}
}
