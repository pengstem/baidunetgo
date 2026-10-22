package transfer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/fakepan"
)

// counting wraps a client, counting its range requests and failing the
// first few as expired links.
type counting struct {
	*baidu.Client
	fetches atomic.Int32
	expire  atomic.Int32
	links   atomic.Int32
}

func (c *counting) DownloadURL(ctx context.Context, p string) (string, error) {
	c.links.Add(1)
	return c.Client.DownloadURL(ctx, p)
}

func (c *counting) Fetch(ctx context.Context, u string, off, n int64) (*http.Response, error) {
	c.fetches.Add(1)
	if c.expire.Add(-1) >= 0 {
		return nil, &baidu.HTTPError{Status: http.StatusForbidden}
	}
	return c.Client.Fetch(ctx, u, off, n)
}

func setup(t *testing.T) (*fakepan.Server, *counting, []byte) {
	srv := fakepan.New()
	t.Cleanup(srv.Close)
	data := make([]byte, 3*defaultBlock+12345)
	rand.New(rand.NewSource(1)).Read(data)
	srv.Add(fakepan.Node{Path: "/大文件.bin", Data: data})
	c := &counting{Client: baidu.OpenAt(baidu.Credentials{BDUSS: "x", UID: 1}, srv.URL)}
	return srv, c, data
}

func wait(t *testing.T, m *Manager, want State) Task {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		ts := m.Tasks()
		all := len(ts) > 0
		for _, x := range ts {
			if x.State != want {
				all = false
			}
		}
		if all {
			return ts[0]
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("tasks never %v: %+v", want, m.Tasks())
	return Task{}
}

func TestDownloadRetriesExpiredLinks(t *testing.T) {
	_, c, data := setup(t)
	c.expire.Store(2)
	dir := t.TempDir()
	m := New(c, baidu.Expired, filepath.Join(dir, "tasks.json"))
	defer m.Close()
	var changes atomic.Int32
	m.OnChange = func() { m.Tasks(); changes.Add(1) } // listeners read the tasks
	m.AddDownloads([]baidu.File{{Path: "/大文件.bin", Name: "大文件.bin", Size: int64(len(data))}}, dir)
	task := wait(t, m, Done)
	got, err := os.ReadFile(task.Local)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("content differs (%d bytes, %v)", len(got), err)
	}
	if c.links.Load() < 2 {
		t.Errorf("expired link not renewed: %d links", c.links.Load())
	}
	if _, err := os.Stat(task.Local + partExt); !os.IsNotExist(err) {
		t.Error("part file left")
	}
}

func TestDownloadResumes(t *testing.T) {
	_, c, data := setup(t)
	dir := t.TempDir()
	local := filepath.Join(dir, "大文件.bin")
	// Two blocks were done before the app quit.
	part := make([]byte, len(data))
	copy(part[:2*defaultBlock], data[:2*defaultBlock])
	os.WriteFile(local+partExt, part, 0o644)
	meta, _ := json.Marshal(partMeta{Remote: "/大文件.bin", Size: int64(len(data)), Blocks: []bool{true, true, false, false}})
	os.WriteFile(local+partExt+metaExt, meta, 0o644)
	m := New(c, baidu.Expired, "")
	defer m.Close()
	m.Block = 1 << 20 // a new size since: the download keeps its own
	m.mu.Lock()
	m.add(&Task{Kind: Download, Name: "大文件.bin", Remote: "/大文件.bin", Local: local, Size: int64(len(data))})
	m.mu.Unlock()
	m.schedule()
	wait(t, m, Done)
	got, _ := os.ReadFile(local)
	if !bytes.Equal(got, data) {
		t.Fatal("resumed content differs")
	}
	if n := c.fetches.Load(); n != 2 {
		t.Errorf("fetched %d blocks, want the 2 left", n)
	}
}

func TestDownloadBlockSize(t *testing.T) {
	_, c, data := setup(t)
	dir := t.TempDir()
	m := New(c, baidu.Expired, "")
	defer m.Close()
	m.Block = 1 << 20
	m.AddDownloads([]baidu.File{{Path: "/大文件.bin", Name: "大文件.bin", Size: int64(len(data))}}, dir)
	task := wait(t, m, Done)
	got, _ := os.ReadFile(task.Local)
	if !bytes.Equal(got, data) {
		t.Fatal("content differs")
	}
	if want := int32((len(data) + 1<<20 - 1) / (1 << 20)); c.fetches.Load() != want {
		t.Errorf("fetched %d blocks, want %d of 1 MB", c.fetches.Load(), want)
	}
}

func TestFolderDownloadAndUpload(t *testing.T) {
	srv, c, _ := setup(t)
	dir := t.TempDir()
	m := New(c, baidu.Expired, filepath.Join(dir, "tasks.json"))
	defer m.Close()
	m.AddDownloads([]baidu.File{{Path: "/文档", Name: "文档", IsDir: true}}, dir)
	time.Sleep(200 * time.Millisecond)
	wait(t, m, Done)
	if _, err := os.Stat(filepath.Join(dir, "文档", "旅行计划.md")); err != nil {
		t.Fatalf("folder not downloaded: %v", err)
	}
	m.ClearFinished()

	up := filepath.Join(dir, "up")
	os.MkdirAll(filepath.Join(up, "sub"), 0o755)
	os.WriteFile(filepath.Join(up, "a.txt"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(up, "sub", "b.txt"), []byte("world"), 0o644)
	m.AddUploads([]string{up}, "/备份", nil)
	time.Sleep(200 * time.Millisecond)
	wait(t, m, Done)
	if string(srv.Data("/备份/up/sub/b.txt")) != "world" {
		t.Error("folder not uploaded")
	}

	// The tasks are kept for the next launch.
	m2 := New(c, baidu.Expired, filepath.Join(dir, "tasks.json"))
	defer m2.Close()
	if len(m2.Tasks()) != 2 {
		t.Errorf("saved tasks %d", len(m2.Tasks()))
	}
}

// gated holds every range request until open is closed, or the request
// is cancelled.
type gated struct {
	*counting
	open chan struct{}
}

func (g *gated) Fetch(ctx context.Context, u string, off, n int64) (*http.Response, error) {
	select {
	case <-g.open:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return g.counting.Fetch(ctx, u, off, n)
}

func TestHoldWaitsThenGoesOn(t *testing.T) {
	srv, c, data := setup(t)
	srv.Add(fakepan.Node{Path: "/小文件.bin", Data: data[:1000]})
	g := &gated{counting: c, open: make(chan struct{})}
	dir := t.TempDir()
	m := New(g, baidu.Expired, "")
	defer m.Close()
	m.AddDownloads([]baidu.File{{Path: "/大文件.bin", Name: "大文件.bin", Size: int64(len(data))}}, dir)
	wait(t, m, Running)
	m.Hold()
	m.AddDownloads([]baidu.File{{Path: "/小文件.bin", Name: "小文件.bin", Size: 1000}}, dir)
	close(g.open) // the network is back, but the queue holds
	time.Sleep(200 * time.Millisecond)
	for _, x := range m.Tasks() {
		if x.State != Queued {
			t.Fatalf("%s is %v while held, want queued", x.Name, x.State)
		}
	}
	if n := c.fetches.Load(); n != 0 {
		t.Fatalf("%d blocks fetched while held", n)
	}
	m.Unhold()
	wait(t, m, Done)
	got, _ := os.ReadFile(filepath.Join(dir, "大文件.bin"))
	if !bytes.Equal(got, data) {
		t.Fatal("content differs after the hold")
	}
}

// pausing stops an upload as its first part is in, and notes what each
// upload goes on from.
type pausing struct {
	*counting
	m     *Manager
	froms []*baidu.UploadState
	once  sync.Once
}

func (p *pausing) Upload(ctx context.Context, local, remote string, parallel int, progress func(done, total int64), from *baidu.UploadState, note func(baidu.UploadState)) error {
	p.froms = append(p.froms, from)
	return p.counting.Upload(ctx, local, remote, parallel, progress, from, func(s baidu.UploadState) {
		note(s)
		if slices.ContainsFunc(s.Parts, func(x string) bool { return x != "" }) {
			p.once.Do(func() {
				go p.m.PauseAll()
				<-ctx.Done()
			})
		}
	})
}

// TestUploadResumes goes on with an upload paused, and kept across a
// launch, from the parts it sent.
func TestUploadResumes(t *testing.T) {
	srv, c, _ := setup(t)
	dir := t.TempDir()
	data := make([]byte, 13<<20+5)
	rand.New(rand.NewSource(3)).Read(data)
	local := filepath.Join(dir, "大.bin")
	os.WriteFile(local, data, 0o644)
	state := filepath.Join(dir, "tasks.json")
	p := &pausing{counting: c}
	m := New(p, baidu.Expired, state)
	p.m = m
	m.AddUploads([]string{local}, "/", nil)
	time.Sleep(100 * time.Millisecond)
	paused := wait(t, m, Paused)
	if paused.Resume == nil || paused.Done == 0 {
		t.Fatalf("nothing kept to go on from: %+v", paused)
	}
	m.Close()

	m2 := New(p, baidu.Expired, state)
	defer m2.Close()
	m2.ResumeAll()
	wait(t, m2, Done)
	if !bytes.Equal(srv.Data("/大.bin"), data) {
		t.Fatal("content differs")
	}
	last := p.froms[len(p.froms)-1]
	if last == nil || last.Parts[0] == "" {
		t.Errorf("the second launch started afresh: %+v", last)
	}
	if m2.Tasks()[0].Resume != nil {
		t.Error("a finished upload keeps its parts")
	}
}

// TestUploadDropped starts afresh when the netdisk has let go of the
// parts sent.
func TestUploadDropped(t *testing.T) {
	srv, c, _ := setup(t)
	dir := t.TempDir()
	data := make([]byte, 9<<20)
	rand.New(rand.NewSource(4)).Read(data)
	local := filepath.Join(dir, "大.bin")
	os.WriteFile(local, data, 0o644)
	p := &pausing{counting: c}
	m := New(p, baidu.Expired, "")
	defer m.Close()
	p.m = m
	m.AddUploads([]string{local}, "/", nil)
	time.Sleep(100 * time.Millisecond)
	wait(t, m, Paused)
	srv.DropUploads()
	m.ResumeAll()
	wait(t, m, Done)
	if !bytes.Equal(srv.Data("/大.bin"), data) {
		t.Fatal("content differs")
	}
}

// flaky fails the uploads it is given errors for, one error a try.
type flaky struct {
	*counting
	mu   sync.Mutex
	errs []error
	list map[string]error // folders that cannot be listed
}

func (f *flaky) Upload(ctx context.Context, local, remote string, parallel int, progress func(done, total int64), from *baidu.UploadState, note func(baidu.UploadState)) error {
	f.mu.Lock()
	var err error
	if len(f.errs) > 0 {
		err, f.errs = f.errs[0], f.errs[1:]
	}
	f.mu.Unlock()
	if err != nil {
		return err
	}
	return f.counting.Upload(ctx, local, remote, parallel, progress, from, note)
}

func (f *flaky) List(ctx context.Context, dir string, o baidu.Order) ([]baidu.File, error) {
	if err := f.list[dir]; err != nil {
		return nil, err
	}
	return f.counting.List(ctx, dir, o)
}

func TestRetriesByItself(t *testing.T) {
	defer func(d []time.Duration) { retryDelays = d }(retryDelays)
	retryDelays = []time.Duration{50 * time.Millisecond, 50 * time.Millisecond}
	srv, c, _ := setup(t)
	dir := t.TempDir()
	local := filepath.Join(dir, "a.txt")
	os.WriteFile(local, []byte("hello"), 0o644)
	f := &flaky{counting: c, errs: []error{errors.New("connection reset by peer")}}
	m := New(f, baidu.Expired, "")
	defer m.Close()
	m.AddUploads([]string{local}, "/", nil)
	task := wait(t, m, Done)
	if string(srv.Data("/a.txt")) != "hello" || task.Retries != 1 {
		t.Fatalf("retries %d, data %q", task.Retries, srv.Data("/a.txt"))
	}

	// What trying again cannot mend fails at once.
	f.errs = []error{baidu.ErrNoSpace}
	m.AddUploads([]string{local}, "/b", nil)
	time.Sleep(100 * time.Millisecond)
	for _, x := range m.Tasks() {
		if x.Remote == "/b/a.txt" && (x.State != Failed || x.Retries != 0) {
			t.Errorf("a full netdisk retried: %+v", x)
		}
	}
}

func TestFolderListFails(t *testing.T) {
	_, c, _ := setup(t)
	f := &flaky{counting: c, list: map[string]error{"/照片/京都": baidu.ErrNotLoggedIn}}
	m := New(f, baidu.Expired, "")
	defer m.Close()
	var skipped []string
	var mu sync.Mutex
	m.OnSkip = func(dir string, err error) { mu.Lock(); skipped = append(skipped, dir); mu.Unlock() }
	m.Hold() // queue only
	m.AddDownloads([]baidu.File{{Path: "/照片", Name: "照片", IsDir: true}}, t.TempDir())
	deadline := time.Now().Add(5 * time.Second)
	for (len(m.Tasks()) == 0 || m.Reading() > 0) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(skipped, []string{"/照片/京都"}) || len(m.Tasks()) == 0 {
		t.Errorf("skipped %v, %d tasks", skipped, len(m.Tasks()))
	}
	for _, x := range m.Tasks() {
		if strings.HasPrefix(x.Remote, "/照片/京都/") {
			t.Errorf("queued from the folder that failed: %s", x.Remote)
		}
	}
}
