// Package tune measures how fast the netdisk serves this account, and
// chooses the transfer settings that make it fastest without asking for
// more connections than help: the netdisk limits accounts by their
// membership, and refuses or slows those that open too many.
package tune

import (
	"context"
	"errors"
	"io"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// Sample is the speed of one number of connections.
type Sample struct {
	Threads int     `json:"threads"`
	Speed   float64 `json:"speed"` // bytes a second
	Bytes   int64   `json:"bytes"`
	Errors  int     `json:"errors"`
	Err     string  `json:"err,omitempty"`
}

// Result is what a probe measured.
type Result struct {
	File      string        `json:"file"`
	Size      int64         `json:"size"`
	Link      time.Duration `json:"link"`       // asking for the download link
	FirstByte time.Duration `json:"first_byte"` // from the request to the first byte
	Samples   []Sample      `json:"samples"`
	At        time.Time     `json:"at"`
}

// Best returns the fewest connections within 90% of the fastest speed, 0
// when nothing worked.
func (r *Result) Best() int {
	fastest := 0.0
	for _, s := range r.Samples {
		fastest = max(fastest, s.Speed)
	}
	if fastest == 0 {
		return 0
	}
	ok := make([]Sample, 0, len(r.Samples))
	for _, s := range r.Samples {
		if s.Speed >= 0.9*fastest && s.Errors == 0 {
			ok = append(ok, s)
		}
	}
	if len(ok) == 0 {
		return 1
	}
	sort.Slice(ok, func(i, j int) bool { return ok[i].Threads < ok[j].Threads })
	return ok[0].Threads
}

// Speed returns the speed measured with n connections, 0 if not measured.
func (r *Result) Speed(n int) float64 {
	for _, s := range r.Samples {
		if s.Threads == n {
			return s.Speed
		}
	}
	return 0
}

// Config is transfer settings.
type Config struct {
	Threads  int `json:"threads"`  // connections per download
	Parallel int `json:"parallel"` // files at once
}

// ForVIP returns settings suited to a membership level (0 none, 1 VIP, 2
// SVIP) before anything is measured: the netdisk throttles accounts
// without membership to one slow connection a file, and answers too many
// connections with refusals.
func ForVIP(vip int) Config {
	switch vip {
	case 2:
		return Config{Threads: 16, Parallel: 3}
	case 1:
		return Config{Threads: 4, Parallel: 2}
	}
	return Config{Threads: 2, Parallel: 2}
}

// Candidates returns the numbers of connections worth measuring for a
// membership level.
func Candidates(vip int) []int {
	switch vip {
	case 2:
		return []int{4, 8, 16, 24}
	case 1:
		return []int{1, 2, 4, 8}
	}
	return []int{1, 2, 4}
}

// Recommend returns settings from a measurement, falling back to those of
// the membership level.
func Recommend(vip int, r *Result) Config {
	cfg := ForVIP(vip)
	if r == nil {
		return cfg
	}
	if best := r.Best(); best > 0 {
		cfg.Threads = best
	}
	// Where one file's connections cannot use what the netdisk gives the
	// account, more files at once can.
	if vip < 2 && r.Speed(cfg.Threads) > 0 && r.Speed(cfg.Threads) < 2<<20 {
		cfg.Parallel = 3
	}
	return cfg
}

// Probe downloads parts of f into nowhere with each number of connections
// in threads, for up to per each and budget bytes in all, and measures
// how fast they come. It keeps nothing.
func Probe(ctx context.Context, c *baidu.Client, f baidu.File, threads []int, per time.Duration, budget int64) (*Result, error) {
	r := &Result{File: f.Path, Size: f.Size, At: time.Now()}
	start := time.Now()
	link, err := c.DownloadURL(ctx, f.Path)
	if err != nil {
		return nil, err
	}
	r.Link = time.Since(start)

	start = time.Now()
	resp, err := c.Fetch(ctx, link, 0, 64<<10)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 1)
	resp.Body.Read(buf)
	r.FirstByte = time.Since(start)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	share := budget / int64(max(len(threads), 1))
	for _, n := range threads {
		if ctx.Err() != nil {
			break
		}
		s := measure(ctx, c, link, f.Size, n, per, share)
		if s.Errors > 0 && baidu.Expired(errors.New(s.Err)) {
			if l, err := c.DownloadURL(ctx, f.Path); err == nil {
				link = l
			}
		}
		r.Samples = append(r.Samples, s)
		// Past a refusal, more connections only fare worse.
		if s.Speed == 0 && n > 1 {
			break
		}
	}
	return r, nil
}

const block = 2 << 20

// measure reads blocks of the file over n connections for up to d or
// limit bytes.
func measure(ctx context.Context, c *baidu.Client, link string, size int64, n int, d time.Duration, limit int64) Sample {
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	var (
		got    atomic.Int64
		next   atomic.Int64
		errs   atomic.Int32
		mu     sync.Mutex
		lastEr string
		wg     sync.WaitGroup
	)
	start := time.Now()
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 64<<10)
			for ctx.Err() == nil && got.Load() < limit {
				off := (next.Add(block) - block) % max(size-block, 1)
				resp, err := c.Fetch(ctx, link, off, min(block, size-off))
				if err != nil {
					if ctx.Err() == nil {
						errs.Add(1)
						mu.Lock()
						lastEr = err.Error()
						mu.Unlock()
						time.Sleep(300 * time.Millisecond)
					}
					continue
				}
				for {
					k, err := resp.Body.Read(buf)
					got.Add(int64(k))
					if err != nil {
						break
					}
				}
				resp.Body.Close()
			}
		}()
	}
	wg.Wait()
	el := time.Since(start).Seconds()
	s := Sample{Threads: n, Bytes: got.Load(), Errors: int(errs.Load()), Err: lastEr}
	if el > 0 {
		s.Speed = float64(s.Bytes) / el
	}
	return s
}

// FindFile looks for a file large enough to measure with, at least min
// bytes, going through a few folders of the netdisk.
func FindFile(ctx context.Context, c *baidu.Client, min int64) (baidu.File, error) {
	queue := []string{"/"}
	var best baidu.File
	for visited := 0; len(queue) > 0 && visited < 40; visited++ {
		dir := queue[0]
		queue = queue[1:]
		files, err := c.List(ctx, dir, baidu.Order{By: "size", Desc: true})
		if err != nil {
			if visited == 0 {
				return baidu.File{}, err
			}
			continue
		}
		for _, f := range files {
			if f.IsDir {
				queue = append(queue, f.Path)
			} else if f.Size > best.Size {
				best = f
			}
		}
		if best.Size >= min*4 {
			break
		}
	}
	if best.Size < min {
		return baidu.File{}, errors.New("网盘里没有足够大的文件可以测速")
	}
	return best, nil
}
