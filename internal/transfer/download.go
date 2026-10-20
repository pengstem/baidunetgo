package transfer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	partExt      = ".bngdl"
	metaExt      = ".json"
	defaultBlock = 4 << 20
)

// partMeta is what a download has done, beside its part file, to resume.
type partMeta struct {
	Remote string `json:"remote"`
	Size   int64  `json:"size"`
	Block  int64  `json:"block,omitempty"` // bytes a block, 4 MB when 0
	Blocks []bool `json:"blocks"`
}

// download fetches a file in blocks over several connections into a part
// file, which becomes the file once whole.
func (m *Manager) download(ctx context.Context, t *Task) error {
	if err := os.MkdirAll(filepath.Dir(t.Local), 0o755); err != nil {
		return err
	}
	part, metaPath := t.Local+partExt, t.Local+partExt+metaExt
	if t.Size == 0 {
		f, err := os.Create(t.Local)
		if err == nil {
			f.Close()
		}
		return err
	}
	blockSize := m.Block
	if blockSize <= 0 {
		blockSize = defaultBlock
	}
	n := int((t.Size + blockSize - 1) / blockSize)
	meta := partMeta{Remote: t.Remote, Size: t.Size, Block: blockSize, Blocks: make([]bool, n)}
	if data, err := os.ReadFile(metaPath); err == nil {
		var saved partMeta
		if json.Unmarshal(data, &saved) == nil && saved.Size == t.Size && saved.Remote == t.Remote {
			if saved.Block <= 0 {
				saved.Block = defaultBlock
			}
			if len(saved.Blocks) == int((t.Size+saved.Block-1)/saved.Block) {
				meta, blockSize, n = saved, saved.Block, len(saved.Blocks)
			}
		}
	}
	f, err := os.OpenFile(part, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	if st, _ := f.Stat(); st.Size() != t.Size {
		if err := f.Truncate(t.Size); err != nil {
			f.Close()
			return err
		}
	}

	var (
		mu       sync.Mutex
		next     int
		link     string
		linkGen  int
		firstErr error
		lastSave = time.Now()
	)
	doneBytes := int64(0)
	for i, ok := range meta.Blocks {
		if ok {
			doneBytes += blockLen(i, blockSize, t.Size)
		}
	}
	t.done.Store(doneBytes)

	// getLink returns the link, asking for a new one when the one a
	// worker used (gen) expired.
	getLink := func(stale int) (string, int, error) {
		mu.Lock()
		defer mu.Unlock()
		if link != "" && stale != linkGen {
			return link, linkGen, nil
		}
		u, err := m.nd.DownloadURL(ctx, t.Remote)
		if err != nil {
			return "", 0, err
		}
		link = u
		linkGen++
		return link, linkGen, nil
	}
	saveMeta := func() {
		data, _ := json.Marshal(meta)
		os.WriteFile(metaPath, data, 0o644)
	}
	take := func() int {
		mu.Lock()
		defer mu.Unlock()
		for next < n && meta.Blocks[next] {
			next++
		}
		if next >= n || firstErr != nil {
			return -1
		}
		next++
		return next - 1
	}
	if _, _, err := getLink(0); err != nil {
		f.Close()
		return err
	}

	var wg sync.WaitGroup
	for range max(m.Threads, 1) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 64<<10)
			for {
				i := take()
				if i < 0 || ctx.Err() != nil {
					return
				}
				err := m.fetchBlock(ctx, t, f, i, blockSize, buf, getLink)
				mu.Lock()
				if err != nil {
					if firstErr == nil && ctx.Err() == nil {
						firstErr = err
					}
					mu.Unlock()
					return
				}
				meta.Blocks[i] = true
				if time.Since(lastSave) > time.Second {
					saveMeta()
					lastSave = time.Now()
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	saveMeta()
	f.Close()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if firstErr != nil {
		return firstErr
	}
	for _, ok := range meta.Blocks {
		if !ok {
			return errors.New("下载未完成")
		}
	}
	if err := os.Rename(part, t.Local); err != nil {
		return err
	}
	os.Remove(metaPath)
	return nil
}

func blockLen(i int, block, size int64) int64 {
	start := int64(i) * block
	return min(block, size-start)
}

// fetchBlock reads block i into f, trying again with a new link when it
// expired and after a pause when the connection failed.
func (m *Manager) fetchBlock(ctx context.Context, t *Task, f *os.File, i int, block int64, buf []byte, getLink func(int) (string, int, error)) error {
	start := int64(i) * block
	length := blockLen(i, block, t.Size)
	gen := 0
	var lastErr error
	for attempt := range 5 {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * 800 * time.Millisecond):
			}
		}
		u, g, err := getLink(gen)
		if err != nil {
			return err
		}
		gen = g
		resp, err := m.nd.Fetch(ctx, u, start, length)
		if err != nil {
			lastErr = err
			if m.expired != nil && m.expired(err) {
				gen = g // ask for a new link
			} else {
				gen = -1 // keep the link
			}
			continue
		}
		got := int64(0)
		for got < length {
			k, rerr := resp.Body.Read(buf[:min(int64(len(buf)), length-got)])
			if k > 0 {
				if _, werr := f.WriteAt(buf[:k], start+got); werr != nil {
					resp.Body.Close()
					return werr
				}
				got += int64(k)
				t.done.Add(int64(k))
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				lastErr = rerr
				break
			}
		}
		resp.Body.Close()
		if got == length {
			return nil
		}
		t.done.Add(-got) // the block starts over
		gen = -1
		if lastErr == nil {
			lastErr = io.ErrUnexpectedEOF
		}
	}
	return lastErr
}
