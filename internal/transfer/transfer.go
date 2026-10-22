// Package transfer moves files between the netdisk and the computer: a
// queue of downloads, each over several connections and resumable, and of
// uploads, run a few at a time and kept across launches.
package transfer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// Netdisk is what transfers need of the netdisk; baidu.Client is one.
type Netdisk interface {
	DownloadURL(ctx context.Context, path string) (string, error)
	Fetch(ctx context.Context, url string, off, n int64) (*http.Response, error)
	List(ctx context.Context, dir string, o baidu.Order) ([]baidu.File, error)
	Upload(ctx context.Context, local, remote string, parallel int, progress func(done, total int64), from *baidu.UploadState, note func(baidu.UploadState)) error
}

// Kind is which way a task goes.
type Kind int

const (
	Download Kind = iota
	Upload
)

// State is where a task is.
type State int

const (
	Queued State = iota
	Running
	Paused
	Done
	Failed
)

// Task is one file going one way.
type Task struct {
	ID       int64     `json:"id"`
	Kind     Kind      `json:"kind"`
	Name     string    `json:"name"`
	Remote   string    `json:"remote"`
	Local    string    `json:"local"`
	Size     int64     `json:"size"`
	Done     int64     `json:"done"`
	State    State     `json:"state"`
	Err      string    `json:"err,omitempty"`
	Created  time.Time `json:"created"`
	Finished time.Time `json:"finished,omitzero"`
	// Resume is how far an upload went, to go on from there.
	Resume *baidu.UploadState `json:"resume,omitempty"`
	// Retries counts the times the task failed and was queued again by
	// itself; RetryAt is when it may start again.
	Retries int       `json:"retries,omitempty"`
	RetryAt time.Time `json:"-"`
	// Speed is bytes a second, while it runs.
	Speed float64 `json:"-"`

	done   *atomic.Int64 // shared with the workers; nil in copies
	cancel context.CancelFunc
}

// Manager runs the tasks.
type Manager struct {
	nd        Netdisk
	expired   func(error) bool
	statePath string

	// Threads is connections per download, Upload per upload, Parallel
	// tasks at once.
	Threads, Upload, Parallel int
	// Block is the bytes a download request asks for; downloads resumed
	// keep the size they began with.
	Block int64
	// OnChange is called, from any goroutine, when tasks change or move.
	OnChange func()
	// OnFinish is called as a task ends, done or failed.
	OnFinish func(t Task)
	// OnSkip is told of folders whose files could not be listed to
	// download, with why.
	OnSkip func(dir string, err error)

	mu      sync.Mutex
	tasks   []*Task
	nextID  int64
	closed  bool
	holding bool // Hold: nothing starts
	stop    chan struct{}
	reading atomic.Int32 // folders being listed to download
}

// Reading reports how many folders are being listed to download.
func (m *Manager) Reading() int { return int(m.reading.Load()) }

// retryDelays are how long a task that failed waits before it is queued
// again by itself, the first time, the second and so on.
var retryDelays = []time.Duration{5 * time.Second, 20 * time.Second, time.Minute, 3 * time.Minute, 5 * time.Minute}

const maxRetries = 5

// retryable tells errors that trying again may get past, as a connection
// that broke, from those it will not: the session expired, the netdisk or
// the disk full, the local file gone.
func retryable(err error) bool {
	for _, e := range []error{baidu.ErrNotLoggedIn, baidu.ErrNoSpace, baidu.ErrExists, os.ErrNotExist, os.ErrPermission, syscall.ENOSPC} {
		if errors.Is(err, e) {
			return false
		}
	}
	return true
}

// New makes a manager, loading the tasks saved at statePath ("" keeps
// none). Tasks that ran when the app quit wait, paused.
func New(nd Netdisk, expired func(error) bool, statePath string) *Manager {
	m := &Manager{nd: nd, expired: expired, statePath: statePath, Threads: 4, Upload: 4, Parallel: 2, Block: defaultBlock, stop: make(chan struct{})}
	if statePath != "" {
		if data, err := os.ReadFile(statePath); err == nil {
			json.Unmarshal(data, &m.tasks)
		}
	}
	for _, t := range m.tasks {
		t.done = new(atomic.Int64)
		t.done.Store(t.Done)
		if t.State == Running || t.State == Queued {
			t.State = Paused
		}
		m.nextID = max(m.nextID, t.ID)
	}
	go m.measure()
	return m
}

// Tasks returns copies of the tasks, newest first.
func (m *Manager) Tasks() []Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Task, len(m.tasks))
	for i, t := range m.tasks {
		out[len(m.tasks)-1-i] = t.snapshot()
	}
	return out
}

func (t *Task) snapshot() Task {
	return Task{ID: t.ID, Kind: t.Kind, Name: t.Name, Remote: t.Remote, Local: t.Local, Size: t.Size,
		Done: t.done.Load(), State: t.State, Err: t.Err, Created: t.Created, Finished: t.Finished, Resume: t.Resume, Retries: t.Retries, RetryAt: t.RetryAt, Speed: t.Speed}
}

func (m *Manager) changed() {
	if m.OnChange != nil {
		m.OnChange()
	}
}

// save writes the tasks; it runs with m.mu held.
func (m *Manager) save() {
	if m.statePath == "" {
		return
	}
	snap := make([]Task, len(m.tasks))
	for i, t := range m.tasks {
		snap[i] = t.snapshot()
	}
	data, err := json.MarshalIndent(snap, "", " ")
	if err != nil {
		return
	}
	tmp := m.statePath + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		os.Rename(tmp, m.statePath)
	}
}

func (m *Manager) add(t *Task) {
	m.nextID++
	t.ID = m.nextID
	t.done = new(atomic.Int64)
	t.Created = time.Now()
	t.State = Queued
	m.tasks = append(m.tasks, t)
}

// AddDownloads queues files and the files of folders, saving them under
// localDir as they are under baseDir in the netdisk.
func (m *Manager) AddDownloads(files []baidu.File, localDir string) {
	go func() {
		taken := map[string]bool{}
		for _, f := range files {
			if !f.IsDir {
				m.queueDownload(f, filepath.Join(localDir, f.Name), taken)
				continue
			}
			root := uniquePath(filepath.Join(localDir, f.Name), taken)
			m.reading.Add(1)
			m.changed()
			m.walk(f.Path, func(sub baidu.File) {
				rel := strings.TrimPrefix(sub.Path, f.Path+"/")
				m.queueDownload(sub, filepath.Join(root, filepath.FromSlash(rel)), nil)
			})
			m.reading.Add(-1)
		}
		m.mu.Lock()
		m.save()
		m.mu.Unlock()
		m.schedule()
	}()
}

func (m *Manager) queueDownload(f baidu.File, local string, taken map[string]bool) {
	if taken != nil {
		local = uniquePath(local, taken)
	}
	m.mu.Lock()
	m.add(&Task{Kind: Download, Name: f.Name, Remote: f.Path, Local: local, Size: f.Size})
	m.mu.Unlock()
	m.schedule()
}

// walkers is how many folders walk lists at once.
const walkers = 4

// walk calls fn, from one goroutine at a time, for every file under dir
// of the netdisk, listing a few folders at once. A folder it cannot list,
// even after trying again, goes to OnSkip.
func (m *Manager) walk(dir string, fn func(baidu.File)) {
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, walkers)
	)
	var list func(dir string)
	list = func(dir string) {
		defer wg.Done()
		sem <- struct{}{}
		var files []baidu.File
		var err error
		for attempt := range 3 {
			if attempt > 0 {
				time.Sleep(time.Duration(attempt) * time.Second)
			}
			if files, err = m.nd.List(context.Background(), dir, baidu.Order{By: "name"}); err == nil || !retryable(err) {
				break
			}
		}
		<-sem
		if err != nil {
			if m.OnSkip != nil {
				m.OnSkip(dir, err)
			}
			return
		}
		for _, f := range files {
			if f.IsDir {
				wg.Add(1)
				go list(f.Path)
			} else {
				mu.Lock()
				fn(f)
				mu.Unlock()
			}
		}
	}
	wg.Add(1)
	list(dir)
	wg.Wait()
}

// uniquePath returns p, or p with a number before its extension when a
// file, or a path taken in this batch, has its name.
func uniquePath(p string, taken map[string]bool) string {
	ext := filepath.Ext(p)
	stem := strings.TrimSuffix(p, ext)
	for i := 0; ; i++ {
		cand := p
		if i > 0 {
			cand = stem + " (" + strconv.Itoa(i) + ")" + ext
		}
		if _, err := os.Stat(cand); errors.Is(err, os.ErrNotExist) && !taken[cand] {
			if _, err := os.Stat(cand + partExt); err == nil {
				continue
			}
			taken[cand] = true
			return cand
		}
	}
}

// AddUploads queues local files, and the files of local folders, to go to
// remoteDir, each under its own name or the one names gives it.
func (m *Manager) AddUploads(locals []string, remoteDir string, names map[string]string) {
	go func() {
		for _, l := range locals {
			st, err := os.Stat(l)
			if err != nil {
				continue
			}
			root := path.Join(remoteDir, filepath.Base(l))
			if name := names[l]; name != "" {
				root = path.Join(remoteDir, name)
			}
			if !st.IsDir() {
				m.queueUpload(l, root, st.Size())
				continue
			}
			filepath.WalkDir(l, func(p string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				info, err := d.Info()
				if err != nil {
					return nil
				}
				rel, _ := filepath.Rel(l, p)
				m.queueUpload(p, path.Join(root, filepath.ToSlash(rel)), info.Size())
				return nil
			})
		}
		m.mu.Lock()
		m.save()
		m.mu.Unlock()
	}()
}

func (m *Manager) queueUpload(local, remote string, size int64) {
	m.mu.Lock()
	m.add(&Task{Kind: Upload, Name: path.Base(remote), Remote: remote, Local: local, Size: size})
	m.mu.Unlock()
	m.schedule()
}

// schedule starts queued tasks while fewer than Parallel run.
func (m *Manager) schedule() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	if m.holding {
		m.mu.Unlock()
		m.changed() // tasks added wait, queued
		return
	}
	running := 0
	for _, t := range m.tasks {
		if t.State == Running {
			running++
		}
	}
	for _, t := range m.tasks {
		if running >= max(m.Parallel, 1) {
			break
		}
		// A task still winding down from a pause starts again as its
		// run ends, not beside it on the same file.
		if t.State != Queued || t.cancel != nil || t.RetryAt.After(time.Now()) {
			continue
		}
		running++
		ctx, cancel := context.WithCancel(context.Background())
		t.State, t.Err, t.cancel = Running, "", cancel
		go m.run(ctx, t)
	}
	m.mu.Unlock()
	m.changed() // outside the lock: listeners read the tasks
}

func (m *Manager) run(ctx context.Context, t *Task) {
	var err error
	switch t.Kind {
	case Download:
		err = m.download(ctx, t)
	case Upload:
		m.mu.Lock()
		from := t.Resume
		m.mu.Unlock()
		err = m.nd.Upload(ctx, t.Local, t.Remote, m.Upload, func(done, total int64) {
			t.done.Store(done)
			if total > 0 {
				m.mu.Lock()
				t.Size = total
				m.mu.Unlock()
			}
		}, from, func(r baidu.UploadState) {
			m.mu.Lock()
			t.Resume = &r
			m.mu.Unlock()
		})
		if errors.Is(err, baidu.ErrExists) {
			err = nil
		}
	}
	m.mu.Lock()
	t.cancel = nil
	switch {
	case ctx.Err() != nil:
		// Paused or removed: whoever cancelled set the state.
	case err != nil && t.Retries < maxRetries && retryable(err):
		// It waits in the queue a while, then tries again from where it
		// stopped.
		t.Retries++
		delay := retryDelays[min(t.Retries, len(retryDelays))-1]
		t.State, t.Err, t.RetryAt = Queued, err.Error(), time.Now().Add(delay)
		time.AfterFunc(delay, m.schedule)
	case err != nil:
		t.State, t.Err = Failed, err.Error()
	default:
		t.State, t.Finished, t.Resume = Done, time.Now(), nil
		t.done.Store(t.Size)
	}
	t.Speed = 0
	final := t.snapshot()
	m.save()
	m.mu.Unlock()
	if m.OnFinish != nil && (final.State == Done || final.State == Failed) {
		m.OnFinish(final)
	}
	m.schedule()
}

func (m *Manager) find(id int64) *Task {
	for _, t := range m.tasks {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// Pause stops a task, keeping what it has done.
func (m *Manager) Pause(id int64) {
	m.mu.Lock()
	if t := m.find(id); t != nil && (t.State == Running || t.State == Queued) {
		t.State = Paused
		if t.cancel != nil {
			t.cancel()
		}
		m.save()
	}
	m.mu.Unlock()
	m.schedule()
}

// Resume queues a paused or failed task again.
func (m *Manager) Resume(id int64) {
	m.mu.Lock()
	if t := m.find(id); t != nil && (t.State == Paused || t.State == Failed) {
		t.State, t.Err, t.Retries, t.RetryAt = Queued, "", 0, time.Time{}
	}
	m.mu.Unlock()
	m.schedule()
}

// Remove forgets a task, stopping it and deleting a download's part.
func (m *Manager) Remove(id int64) {
	m.mu.Lock()
	t := m.find(id)
	if t == nil {
		m.mu.Unlock()
		return
	}
	if t.cancel != nil {
		t.cancel()
	}
	if t.Kind == Download && t.State != Done {
		part := t.Local + partExt
		go func() {
			time.Sleep(300 * time.Millisecond) // let the workers let go of it
			os.Remove(part)
			os.Remove(part + metaExt)
		}()
	}
	m.tasks = slices.DeleteFunc(m.tasks, func(x *Task) bool { return x == t })
	m.save()
	m.mu.Unlock()
	m.schedule()
}

// PauseAll pauses every task waiting or running; ResumeAll queues every
// task paused or failed.
func (m *Manager) PauseAll() {
	for _, t := range m.Tasks() {
		if t.State == Running || t.State == Queued {
			m.Pause(t.ID)
		}
	}
}

func (m *Manager) ResumeAll() {
	for _, t := range m.Tasks() {
		if t.State == Paused || t.State == Failed {
			m.Resume(t.ID)
		}
	}
}

// Hold stops the tasks running and starts none until Unhold, for while
// the computer is offline or asleep. Those it stops wait in the queue, to
// go on where they were; tasks the user paused stay paused.
func (m *Manager) Hold() {
	m.mu.Lock()
	m.holding = true
	for _, t := range m.tasks {
		if t.State == Running {
			t.State, t.Speed = Queued, 0
			t.cancel()
		}
	}
	m.save()
	m.mu.Unlock()
	m.changed()
}

// Unhold starts the queue again after Hold.
func (m *Manager) Unhold() {
	m.mu.Lock()
	held := m.holding
	m.holding = false
	m.mu.Unlock()
	if held {
		m.schedule()
	}
}

// ClearFinished forgets the tasks done.
func (m *Manager) ClearFinished() {
	m.mu.Lock()
	m.tasks = slices.DeleteFunc(m.tasks, func(t *Task) bool { return t.State == Done })
	m.save()
	m.mu.Unlock()
	m.changed()
}

// Close pauses what runs and saves the tasks, for quitting.
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	for _, t := range m.tasks {
		if t.cancel != nil {
			t.cancel()
			t.State = Paused
		}
	}
	m.save()
	m.mu.Unlock()
	close(m.stop)
}

// measure updates the speeds of running tasks twice a second.
func (m *Manager) measure() {
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	last := map[int64]int64{}
	saved := time.Now()
	for {
		select {
		case <-m.stop:
			return
		case <-tick.C:
		}
		m.mu.Lock()
		any := false
		for _, t := range m.tasks {
			if t.State != Running {
				continue
			}
			any = true
			d := t.done.Load()
			prev, ok := last[t.ID]
			last[t.ID] = d
			if !ok {
				continue
			}
			inst := float64(d-prev) * 2
			if t.Speed == 0 {
				t.Speed = inst
			} else {
				t.Speed = t.Speed*0.7 + inst*0.3
			}
		}
		if any && time.Since(saved) > 5*time.Second {
			m.save()
			saved = time.Now()
		}
		m.mu.Unlock()
		if any {
			m.changed()
		}
	}
}
