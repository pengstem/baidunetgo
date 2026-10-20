package app

import (
	"math"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/transfer"
)

// Files come and go through specks (effects.go): a file deleted stays in
// its listing while it dissolves, and an upload shows in the folder it
// goes into at once, as a row gathering from specks as its bytes go up,
// until the listing has the file.

// dying is a file deleted, dissolving before it leaves its listing.
type dying struct {
	dir   string
	index int // where it was, to keep it there through a reload
	file  baidu.File
	at    time.Time
}

// gather is an upload as its folder shows it: the file, or the folder of
// the files, it makes.
type gather struct {
	target float32 // the share of its bytes up
	shown  float32 // the share showing, which follows target slowly
	size   int64
	dir    bool
	done   bool // every upload into it has finished
	failed bool // it failed, or was taken off the queue, and dissolves
	ended  time.Time
}

// gatherCreep and gatherRush bound how fast a row gathers, in shares a
// second: an upload done at once (a rapid upload) still takes a moment,
// and a slow one never seems stuck.
const (
	gatherCreep = 0.04
	gatherRush  = 0.8
)

// recentUpload is how long a finished upload still gathers its row, for
// one done at once or before its folder showed, and is kept gathered, so
// that it does not gather again.
const recentUpload = 20 * time.Second

// dissolve starts files of a listing dissolving, one after another.
func (a *App) dissolve(l *listing, ids []int64) {
	if a.dying == nil {
		a.dying = map[int64]*dying{}
	}
	n := 0
	for i, f := range l.files {
		if slices.Contains(ids, f.FsID) {
			a.dying[f.FsID] = &dying{dir: l.dir, index: i, file: f, at: a.now.Add(time.Duration(min(n, 12)) * 35 * time.Millisecond)}
			n++
		}
	}
}

// spare stops files dissolving, as they come back.
func (a *App) spare(ids []int64) {
	for _, id := range ids {
		delete(a.dying, id)
	}
}

// keepDying puts back into files read again those of dir still
// dissolving, where they were.
func (a *App) keepDying(dir string, files []baidu.File) []baidu.File {
	for _, d := range a.dying {
		if d.dir == dir && !slices.ContainsFunc(files, func(f baidu.File) bool { return f.FsID == d.file.FsID }) {
			files = slices.Insert(files, min(d.index, len(files)), d.file)
		}
	}
	return files
}

// passTime moves the files coming and going on: those dissolved leave
// their listings, uploads gather toward their share, and those done a
// while ago, or failed and dissolved, are forgotten.
func (a *App) passTime(now time.Time) {
	for id, d := range a.dying {
		if now.Sub(d.at) < dissolveFor {
			continue
		}
		delete(a.dying, id)
		if l := a.listings[d.dir]; l != nil {
			l.files = slices.DeleteFunc(l.files, func(f baidu.File) bool { return f.FsID == id })
		}
	}
	dt := float32(min(max(now.Sub(a.passed).Seconds(), 0), 0.25))
	a.passed = now
	for p, g := range a.gathers {
		if gap := g.target - g.shown; gap > 0 {
			step := gap * float32(1-math.Exp(-float64(dt)/0.6))
			g.shown += min(max(step, dt*gatherCreep), dt*gatherRush, gap)
		}
		if g.shown > 0.999 && g.target >= 1 {
			g.shown = 1
		}
		switch {
		case g.failed && now.Sub(g.ended) >= dissolveFor,
			g.done && g.shown >= 1 && now.Sub(g.ended) > recentUpload:
			delete(a.gathers, p)
		}
	}
}

// arrivals follows the uploads into a listing's folder, the files and
// folders they make showing in it until it lists them.
func (a *App) arrivals(l *listing, now time.Time) {
	if !l.loaded {
		return
	}
	if l.uploadsSeen != [2]int{a.transfers.gen, l.loads} {
		l.uploadsSeen = [2]int{a.transfers.gen, l.loads}
		a.followUploads(l, now)
	}
	// The rows of uploads the listing lacks, which change as they end or
	// the listing gets them.
	var want []string
	for p := range a.gathers {
		if path.Dir(p) != l.dir {
			continue
		}
		name := path.Base(p)
		if !slices.ContainsFunc(l.files, func(f baidu.File) bool { return f.FsID >= 0 && f.Name == name }) {
			want = append(want, name)
		}
	}
	slices.Sort(want)
	if slices.Equal(want, l.ghosts) {
		return
	}
	l.ghosts = want
	l.files = slices.DeleteFunc(l.files, func(f baidu.File) bool { return f.FsID < 0 })
	for _, name := range want {
		p := path.Join(l.dir, name)
		g := a.gathers[p]
		l.files = append(l.files, baidu.File{FsID: -int64(seedOf(p)) - 1, Path: p, Name: name, Size: g.size, IsDir: g.dir, Mtime: time.Now().Unix()})
	}
	l.sorted = baidu.Order{}
}

// followUploads brings the uploads into l's folder into a.gathers, by
// the entry of the folder each makes.
func (a *App) followUploads(l *listing, now time.Time) {
	type entry struct {
		size, done                  int64
		dir, active, recent, failed bool
	}
	prefix := strings.TrimSuffix(l.dir, "/") + "/"
	found := map[string]*entry{}
	for _, t := range a.transfers.tasks {
		if t.Kind != transfer.Upload {
			continue
		}
		rel, ok := strings.CutPrefix(t.Remote, prefix)
		if !ok || rel == "" {
			continue
		}
		name, _, nested := strings.Cut(rel, "/")
		e := found[name]
		if e == nil {
			e = &entry{}
			found[name] = e
		}
		e.dir = e.dir || nested
		switch t.State {
		case transfer.Queued, transfer.Running, transfer.Paused:
			e.active = true
			e.size += t.Size
			e.done += t.Done
		case transfer.Done:
			if time.Since(t.Finished) < recentUpload {
				e.recent = true
				e.size += t.Size
				e.done += t.Size
			}
		case transfer.Failed:
			e.failed = true
		}
	}
	if a.gathers == nil {
		a.gathers = map[string]*gather{}
	}
	for name, e := range found {
		p := path.Join(l.dir, name)
		g := a.gathers[p]
		switch {
		case e.active || e.recent:
			if g == nil {
				g = &gather{}
				a.gathers[p] = g
			}
			g.size, g.dir, g.failed = e.size, e.dir, false
			g.target = 1
			if e.size > 0 {
				g.target = float32(e.done) / float32(e.size)
			}
			if !e.active && !g.done {
				g.done, g.ended, g.target = true, now, 1
			}
		case e.failed && g != nil && !g.done && !g.failed:
			g.failed, g.ended = true, now
		}
	}
	// Uploads taken off the queue dissolve too.
	for p, g := range a.gathers {
		if path.Dir(p) == l.dir && found[path.Base(p)] == nil && !g.done && !g.failed {
			g.failed, g.ended = true, now
		}
	}
}

// uploadLabel says how an upload shows in its row, in place of the time.
func uploadLabel(g *gather) string {
	switch {
	case g.failed:
		return "上传失败"
	case g.done:
		return "已上传"
	case g.target <= 0:
		return "等待上传"
	}
	return "上传中 " + itoa(int(g.target*100)) + "%"
}
