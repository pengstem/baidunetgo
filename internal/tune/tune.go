// Package tune measures how fast the netdisk serves this account, and
// chooses the transfer settings that make it fastest without asking for
// more than helps: the netdisk limits accounts by their membership, and
// refuses or slows those that open too many connections.
//
// A probe measures, one after the other, each setting with the best of
// those before it: which download host, how many connections a file, how
// large a request, how many files at once, and how many connections an
// upload. How far players and previews read ahead follows from the
// speeds of the connection counts.
package tune

import (
	"context"
	"errors"
	"io"
	"math"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// Version is that of what a probe measures; results of older versions
// are measured again.
const Version = 2

// Sample is the speed with one value of a setting.
type Sample struct {
	// Value is the setting: connections, files, or bytes a request.
	Value  int64   `json:"value"`
	Host   string  `json:"host,omitempty"` // for hosts
	Speed  float64 `json:"speed"`          // bytes a second, all files together
	Bytes  int64   `json:"bytes"`
	Errors int     `json:"errors"`
	Err    string  `json:"err,omitempty"`
}

// Result is what a probe measured.
type Result struct {
	Version   int           `json:"version"`
	File      string        `json:"file"`
	Size      int64         `json:"size"`
	Link      time.Duration `json:"link"`       // asking for the download links
	FirstByte time.Duration `json:"first_byte"` // from the request to the first byte
	Proto     string        `json:"proto"`      // HTTP version of the download host

	Hosts     []Sample `json:"hosts,omitempty"`   // each download host
	Threads   []Sample `json:"threads,omitempty"` // connections a file
	Blocks    []Sample `json:"blocks,omitempty"`  // bytes a request
	Files     []Sample `json:"files,omitempty"`   // files at once
	Upload    []Sample `json:"upload,omitempty"`  // connections an upload
	UploadErr string   `json:"upload_err,omitempty"`

	At time.Time `json:"at"`
}

// Speed returns the speed measured with n connections a file, 0 if not
// measured.
func (r *Result) Speed(n int) float64 {
	return speedOf(r.Threads, int64(n))
}

// Total returns the speed of cfg's settings together, the most
// downloads reached in the probe.
func (r *Result) Total(cfg Config) float64 {
	if s := speedOf(r.Files, int64(cfg.Parallel)); s > 0 {
		return s
	}
	if s := speedOf(r.Blocks, cfg.Block); s > 0 {
		return s
	}
	return r.Speed(cfg.Threads)
}

func speedOf(samples []Sample, v int64) float64 {
	for _, s := range samples {
		if s.Value == v {
			return s.Speed
		}
	}
	return 0
}

// fewest returns the smallest value within frac of the fastest speed,
// of those never refused, 0 when nothing worked.
func fewest(samples []Sample, frac float64) int64 {
	fastest, any := 0.0, 0.0
	for _, s := range samples {
		any = max(any, s.Speed)
		if s.Errors == 0 {
			fastest = max(fastest, s.Speed)
		}
	}
	if any == 0 {
		return 0
	}
	var ok []int64
	for _, s := range samples {
		if s.Speed >= frac*fastest && s.Errors == 0 {
			ok = append(ok, s.Value)
		}
	}
	if len(ok) == 0 {
		// Everything fast was refused at times: the least of all.
		for _, s := range samples {
			if s.Speed > 0 {
				ok = append(ok, s.Value)
			}
		}
	}
	return slices.Min(ok)
}

// Config is transfer settings.
type Config struct {
	Threads  int   `json:"threads"`  // connections a download
	Parallel int   `json:"parallel"` // files at once
	Upload   int   `json:"upload"`   // connections an upload
	Block    int64 `json:"block"`    // bytes a download request asks for
	Ahead    int   `json:"ahead"`    // connections previews and players read with
	// Hosts are download hosts, fastest first.
	Hosts []string `json:"hosts,omitempty"`
}

// DefaultBlock is the bytes a download request asks for before anything
// is measured.
const DefaultBlock = 4 << 20

// ForVIP returns settings suited to a membership level (0 none, 1 VIP, 2
// SVIP) before anything is measured: the netdisk throttles accounts
// without membership to one slow connection a file, and answers too many
// connections with refusals.
func ForVIP(vip int) Config {
	switch vip {
	case 2:
		return Config{Threads: 16, Parallel: 3, Upload: 8, Block: DefaultBlock, Ahead: 4}
	case 1:
		return Config{Threads: 4, Parallel: 2, Upload: 4, Block: DefaultBlock, Ahead: 4}
	}
	return Config{Threads: 2, Parallel: 2, Upload: 4, Block: DefaultBlock, Ahead: 2}
}

// Candidates returns the numbers of connections a file worth measuring
// for a membership level.
func Candidates(vip int) []int {
	switch vip {
	case 2:
		return []int{8, 16, 32, 48}
	case 1:
		return []int{1, 2, 4, 8}
	}
	return []int{1, 2, 4}
}

// uploadCandidates returns the numbers of connections an upload worth
// measuring.
func uploadCandidates(vip int) []int {
	if vip == 2 {
		return []int{4, 8, 16, 32}
	}
	return []int{2, 4, 8, 16}
}

// blockCandidates are the request sizes measured; download hosts refuse
// ranges of 8 MB and more.
var blockCandidates = []int64{1 << 20, 2 << 20, 4 << 20}

// streamSpeed is what a player needs to stay ahead of most videos.
const streamSpeed = 8 << 20

// Recommend returns settings from a measurement, falling back to those of
// the membership level for what it did not measure.
func Recommend(vip int, r *Result) Config {
	cfg := ForVIP(vip)
	if r == nil {
		return cfg
	}
	if n := fewest(r.Threads, 0.9); n > 0 {
		cfg.Threads = int(n)
	}
	if b := pickBlock(r.Blocks); b > 0 {
		cfg.Block = b
	}
	if p := pickFiles(r.Files); p > 0 {
		cfg.Parallel = p
	} else if vip < 2 && r.Speed(cfg.Threads) > 0 && r.Speed(cfg.Threads) < 2<<20 {
		// Where one file's connections cannot use what the netdisk gives
		// the account, more files at once can.
		cfg.Parallel = 3
	}
	if n := fewest(r.Upload, 0.9); n > 0 {
		cfg.Upload = int(n)
	}
	cfg.Hosts = rankHosts(r.Hosts)
	cfg.Ahead = pickAhead(r.Threads, cfg.Threads)
	return cfg
}

// pickBlock returns the request size nearest the default among those
// within 5% of the fastest: the default resumes finely and asks little.
func pickBlock(samples []Sample) int64 {
	fastest := 0.0
	for _, s := range samples {
		fastest = max(fastest, s.Speed)
	}
	if fastest == 0 {
		return 0
	}
	best, dist := int64(0), math.Inf(1)
	for _, s := range samples {
		if s.Errors > 0 || s.Speed < 0.95*fastest {
			continue
		}
		if d := math.Abs(math.Log2(float64(s.Value) / DefaultBlock)); d < dist {
			best, dist = s.Value, d
		}
	}
	return best
}

// pickFiles returns the fewest files at once within 90% of the fastest,
// but two when two lose little: one large file should not hold up the
// rest of the queue.
func pickFiles(samples []Sample) int {
	p := int(fewest(samples, 0.9))
	if p == 1 {
		for _, s := range samples {
			if s.Value == 2 && s.Errors == 0 && s.Speed >= 0.8*speedOf(samples, 1) {
				return 2
			}
		}
	}
	return p
}

// rankHosts returns the hosts that served, fastest first.
func rankHosts(samples []Sample) []string {
	ok := slices.Clone(samples)
	ok = slices.DeleteFunc(ok, func(s Sample) bool { return s.Speed == 0 })
	sort.SliceStable(ok, func(i, j int) bool { return ok[i].Speed > ok[j].Speed })
	var hosts []string
	for _, s := range ok {
		hosts = append(hosts, s.Host)
	}
	return hosts
}

// pickAhead returns the fewest connections that read as fast as a player
// needs, else those of downloads, between 2 and 8.
func pickAhead(samples []Sample, threads int) int {
	sorted := slices.Clone(samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Value < sorted[j].Value })
	for _, s := range sorted {
		if s.Errors == 0 && s.Speed >= streamSpeed {
			return min(max(int(s.Value), 2), 8)
		}
	}
	return min(max(threads, 2), 8)
}

// Options are what a probe measures, and for how long.
type Options struct {
	VIP int
	// Threads are the connection counts measured, Candidates(VIP) when
	// nil.
	Threads []int
	// Per is how long each measurement lasts at most.
	Per time.Duration
	// Budget is the bytes downloaded in all, shared by the measurements.
	Budget int64
	// Upload measures uploads too.
	Upload bool
	// Stage, when set, is told what is measured as it starts.
	Stage func(name string)
}

// Probe measures downloads of files[0] into nowhere, and of files[1] at
// the same time when there is one, and uploads to nowhere. It keeps
// nothing.
func Probe(ctx context.Context, c *baidu.Client, files []baidu.File, o Options) (*Result, error) {
	if len(files) == 0 {
		return nil, errors.New("没有用来测速的文件")
	}
	f := files[0]
	if o.Threads == nil {
		o.Threads = Candidates(o.VIP)
	}
	stage := func(name string) {
		if o.Stage != nil {
			o.Stage(name)
		}
	}
	def := ForVIP(o.VIP)
	r := &Result{Version: Version, File: f.Path, Size: f.Size, At: time.Now()}

	stage("获取下载链接")
	start := time.Now()
	links, err := c.DownloadURLs(ctx, f.Path)
	if err != nil {
		return nil, err
	}
	r.Link = time.Since(start)
	links = links[:min(len(links), 4)]

	start = time.Now()
	resp, err := c.Fetch(ctx, links[0], 0, 64<<10)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 1)
	resp.Body.Read(buf)
	r.FirstByte = time.Since(start)
	r.Proto = resp.Proto
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	count := 2*(len(o.Threads)+len(blockCandidates)) + 3
	if len(links) > 1 {
		count += 2 * len(links)
	}
	share := o.Budget / int64(count)
	link := links[0]
	one := func(l string, n int, block int64, d time.Duration) Sample {
		return measure(ctx, c, []string{l}, []int64{f.Size}, n, block, d, share)
	}

	// The host, with the connections of the membership.
	if len(links) > 1 {
		stage("下载线路")
		hostThreads := min(def.Threads, 8)
		r.Hosts = sweep(ctx, len(links), false, func(i int) Sample {
			s := one(links[i], hostThreads, DefaultBlock, o.Per*3/5)
			s.Host = baidu.Host(links[i])
			return s
		})
		if hosts := rankHosts(r.Hosts); len(hosts) > 0 {
			for _, l := range links {
				if baidu.Host(l) == hosts[0] {
					link = l
				}
			}
		}
	}

	// Connections a file.
	stage("每个文件的连接数")
	r.Threads = sweep(ctx, len(o.Threads), true, func(i int) Sample {
		s := one(link, o.Threads[i], DefaultBlock, o.Per)
		link = refresh(ctx, c, f.Path, link, s)
		return s
	})
	threads := def.Threads
	if n := fewest(r.Threads, 0.9); n > 0 {
		threads = int(n)
	}

	// Bytes a request.
	stage("请求块大小")
	r.Blocks = sweep(ctx, len(blockCandidates), false, func(i int) Sample {
		s := one(link, threads, blockCandidates[i], o.Per*4/5)
		s.Value = blockCandidates[i]
		link = refresh(ctx, c, f.Path, link, s)
		return s
	})
	block := pickBlock(r.Blocks)
	if block == 0 {
		block = DefaultBlock
	}
	if ctx.Err() != nil {
		return r, nil
	}

	// Files at once: the one file alone, then with others.
	stage("同时传输的文件数")
	if one := speedOf(r.Blocks, block); one > 0 {
		r.Files = append(r.Files, Sample{Value: 1, Speed: one})
		other := f
		if len(files) > 1 {
			other = files[1]
		}
		otherLink, err := pickLink(ctx, c, other.Path, baidu.Host(link))
		if err == nil {
			for _, p := range []int{2, 3} {
				if ctx.Err() != nil {
					return r, nil
				}
				ls, sizes := []string{link, otherLink}, []int64{f.Size, other.Size}
				if p == 3 {
					// A third: the first file again, on a link of its own.
					l3, err := pickLink(ctx, c, f.Path, baidu.Host(link))
					if err != nil {
						break
					}
					ls, sizes = append(ls, l3), append(sizes, f.Size)
				}
				s := measure(ctx, c, ls, sizes, threads, block, o.Per, share)
				// Slower than fewer files: measured again before it counts,
				// as a host slowing down for a while looks the same.
				if prev := speedOf(r.Files, int64(p-1)); s.Errors == 0 && s.Speed < 0.9*prev && ctx.Err() == nil {
					if again := measure(ctx, c, ls, sizes, threads, block, o.Per, share); again.Speed > s.Speed {
						s = again
					}
				}
				s.Value = int64(p)
				r.Files = append(r.Files, s)
				// A third file is worth trying only where a second helped.
				if s.Errors > 0 || s.Speed < 1.1*speedOf(r.Files, int64(p-1)) {
					break
				}
			}
		}
	}

	// Connections an upload.
	if o.Upload {
		stage("上传连接数")
		for _, n := range uploadCandidates(o.VIP) {
			if ctx.Err() != nil {
				return r, nil
			}
			start := time.Now()
			sent, errs, err := c.ProbeUpload(ctx, n, o.Per)
			if err != nil && sent == 0 && errs == 0 {
				r.UploadErr = err.Error()
				break
			}
			s := Sample{Value: int64(n), Bytes: sent, Errors: errs}
			if err != nil {
				s.Err = err.Error()
			}
			if el := time.Since(start).Seconds(); el > 0 {
				s.Speed = float64(sent) / el
			}
			r.Upload = append(r.Upload, s)
			if s.Speed == 0 {
				break
			}
			// More connections that add little will not add more.
			if prev := speedOf(r.Upload, int64(n/2)); prev > 0 && s.Speed < 1.1*prev {
				break
			}
		}
	}
	return r, nil
}

// sweep measures n values going up, then again going down, and keeps the
// faster sample of each, with the refusals of both: the hosts slow down
// for seconds at a time, and of two samples apart, the faster is the one
// not caught in that. Going up stops past one that nothing came through
// when stopAtZero, as more connections only fare worse.
func sweep(ctx context.Context, n int, stopAtZero bool, measure func(i int) Sample) []Sample {
	var out []Sample
	for i := range n {
		if ctx.Err() != nil {
			return out
		}
		s := measure(i)
		out = append(out, s)
		if stopAtZero && s.Speed == 0 && i > 0 {
			break
		}
	}
	for i := len(out) - 1; i >= 0; i-- {
		first := &out[i]
		if ctx.Err() != nil || first.Speed == 0 {
			continue
		}
		s := measure(i)
		s.Errors += first.Errors
		if s.Err == "" {
			s.Err = first.Err
		}
		if s.Speed < first.Speed {
			s.Speed, s.Bytes = first.Speed, first.Bytes
		}
		*first = s
	}
	return out
}

// refresh asks for a new link when a measurement found the one it used
// expired.
func refresh(ctx context.Context, c *baidu.Client, path, link string, s Sample) string {
	if s.Errors > 0 && baidu.Expired(errors.New(s.Err)) {
		if l, err := pickLink(ctx, c, path, baidu.Host(link)); err == nil {
			return l
		}
	}
	return link
}

// pickLink returns a link to the file on host, when the netdisk offers
// one there, else the first.
func pickLink(ctx context.Context, c *baidu.Client, path, host string) (string, error) {
	links, err := c.DownloadURLs(ctx, path)
	if err != nil {
		return "", err
	}
	for _, l := range links {
		if baidu.Host(l) == host {
			return l, nil
		}
	}
	return links[0], nil
}

// measure reads blocks of each file over n connections a file for up to
// d or limit bytes, and returns their speed together.
func measure(ctx context.Context, c *baidu.Client, links []string, sizes []int64, n int, block int64, d time.Duration, limit int64) Sample {
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	var (
		got    atomic.Int64
		errs   atomic.Int32
		mu     sync.Mutex
		lastEr string
		wg     sync.WaitGroup
	)
	start := time.Now()
	for i, link := range links {
		size := sizes[i]
		var next atomic.Int64
		for range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				buf := make([]byte, 64<<10)
				// A host that keeps refusing is not asked on and on.
				for ctx.Err() == nil && got.Load() < limit && errs.Load() < int32(4*n*len(links)) {
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
	}
	wg.Wait()
	el := time.Since(start).Seconds()
	s := Sample{Value: int64(n), Bytes: got.Load(), Errors: int(errs.Load()), Err: lastEr}
	if el > 0 {
		s.Speed = float64(s.Bytes) / el
	}
	return s
}

// FindFile looks for a file large enough to measure with, at least `least`
// bytes, going through a few folders of the netdisk.
func FindFile(ctx context.Context, c *baidu.Client, least int64) (baidu.File, error) {
	files, err := FindFiles(ctx, c, least, 1)
	if err != nil {
		return baidu.File{}, err
	}
	return files[0], nil
}

// FindFiles looks for up to n files of at least `least` bytes, the largest
// first, going through a few folders of the netdisk.
func FindFiles(ctx context.Context, c *baidu.Client, least int64, n int) ([]baidu.File, error) {
	queue := []string{"/"}
	var found []baidu.File
	for visited := 0; len(queue) > 0 && visited < 40; visited++ {
		dir := queue[0]
		queue = queue[1:]
		files, err := c.List(ctx, dir, baidu.Order{By: "size", Desc: true})
		if err != nil {
			if visited == 0 {
				return nil, err
			}
			continue
		}
		for _, f := range files {
			if f.IsDir {
				queue = append(queue, f.Path)
			} else if f.Size >= least {
				found = append(found, f)
			}
		}
		sort.SliceStable(found, func(i, j int) bool { return found[i].Size > found[j].Size })
		found = found[:min(len(found), n)]
		if len(found) == n && found[n-1].Size >= least*4 {
			break
		}
	}
	if len(found) == 0 {
		return nil, errors.New("网盘里没有足够大的文件可以测速")
	}
	return found, nil
}
