package app

import (
	"cmp"
	"context"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// listing is a directory as one page of the browser shows it. Each page of
// the router's history keeps its own, so a page sliding away keeps showing
// its files, and going back shows them at once.
type listing struct {
	dir     string
	files   []baidu.File
	sorted  baidu.Order
	loaded  bool
	loading bool
	err     string
	shown   time.Time // when the files first showed, to stagger them in
	read    time.Time // when the netdisk was last asked for them

	list     ui.ListState
	grid     gridState
	sel      ui.Selection[int64]
	selected int
	used     uint64
	sole     int64 // the file chosen alone in the last frame, else 0
	refocus  bool  // the files take the focus back, from a name edited

	loads       int      // counts the reads that came back
	ghosts      []string // the uploads it shows that it does not list yet
	uploadsSeen [2]int   // the transfers' gen and loads it last followed the uploads at
}

const listingCacheSize = 40

// staleAfter is how long a folder's files are trusted while it shows: the
// netdisk may change from the web or a phone, so they are read again,
// quietly, after it, and as soon as the window comes to the front.
const staleAfter = 2 * time.Minute

// filesRoute returns the router path showing dir.
func filesRoute(dir string) string {
	dir = path.Clean("/" + dir)
	if dir == "/" {
		return "/files"
	}
	parts := strings.Split(strings.TrimPrefix(dir, "/"), "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return "/files/" + strings.Join(parts, "/")
}

// dirOfRoute returns the directory a router path's wildcard shows.
func dirOfRoute(param string) string {
	if param == "" {
		return "/"
	}
	parts := strings.Split(param, "/")
	for i, p := range parts {
		if u, err := url.PathUnescape(p); err == nil {
			parts[i] = u
		}
	}
	return path.Clean("/" + strings.Join(parts, "/"))
}

// listingFor returns the listing of dir, loading it the first time.
func (a *App) listingFor(dir string) *listing {
	if a.listings == nil {
		a.listings = map[string]*listing{}
	}
	a.listingTick++
	l := a.listings[dir]
	if l == nil {
		l = &listing{dir: dir, selected: -1}
		a.listings[dir] = l
		a.load(l)
		a.trimListings()
	}
	l.used = a.listingTick
	if l.loaded && !l.loading && time.Since(l.read) > staleAfter {
		a.load(l)
	}
	return l
}

// refocused reads the folder showing again as the window comes to the
// front, unless it was just read, and lets the others be read again as
// they show.
func (a *App) refocused() {
	cur := a.currentDir()
	for dir, l := range a.listings {
		switch {
		case dir != cur:
			l.read = time.Time{}
		case l.loaded && time.Since(l.read) > 10*time.Second:
			a.load(l)
		}
	}
}

func (a *App) trimListings() {
	if len(a.listings) <= listingCacheSize {
		return
	}
	var oldest *listing
	for _, l := range a.listings {
		if oldest == nil || l.used < oldest.used {
			oldest = l
		}
	}
	delete(a.listings, oldest.dir)
}

// load reads a listing's directory from the netdisk.
func (a *App) load(l *listing) {
	s := a.sess
	if s == nil || l.loading {
		return
	}
	l.loading, l.err, l.read = true, "", time.Now()
	order := a.order
	go func() {
		files, err := s.client.List(context.Background(), l.dir, order)
		a.update(func() {
			l.loading = false
			if err != nil {
				l.err = err.Error()
				if err == baidu.ErrNotLoggedIn {
					a.signOut()
					a.login.err = err.Error()
				}
				return
			}
			l.files, l.loaded = a.keepDying(l.dir, files), true
			l.sorted, l.ghosts = baidu.Order{}, nil
			l.loads++
			if l.shown.IsZero() {
				l.shown = time.Now()
			}
		})
	}()
}

// reload reads the directory again, as after changing it.
func (a *App) reload(dir string) {
	if l := a.listings[dir]; l != nil {
		a.load(l)
	}
}

// sortFiles puts folders first, then sorts by the order asked.
func sortFiles(files []baidu.File, o baidu.Order) {
	slices.SortStableFunc(files, func(x, y baidu.File) int {
		if x.IsDir != y.IsDir {
			if x.IsDir {
				return -1
			}
			return 1
		}
		var r int
		switch o.By {
		case "time":
			r = cmp.Compare(x.Mtime, y.Mtime)
		case "size":
			r = cmp.Compare(x.Size, y.Size)
		default:
			r = naturalCompare(x.Name, y.Name)
		}
		if o.Desc {
			r = -r
		}
		return r
	})
}

// nameCollator sorts names as people read them: Chinese by pinyin, case
// aside, and numbers by their value, so that "2" comes before "10". It is
// used on the UI thread only.
var nameCollator = collate.New(language.SimplifiedChinese, collate.IgnoreCase, collate.Numeric)

func naturalCompare(a, b string) int { return nameCollator.CompareString(a, b) }

// open opens a file: a folder goes into it, a file previews.
func (a *App) open(f baidu.File) {
	if f.FsID < 0 || a.dying[f.FsID] != nil {
		return // coming or going
	}
	if f.IsDir {
		a.router.Push(filesRoute(f.Path))
		return
	}
	a.openPreview(f)
}

// currentDir returns the directory the browser shows, "" off it.
func (a *App) currentDir() string {
	p := a.router.Path()
	if p == "/files" {
		return "/"
	}
	if rest, ok := strings.CutPrefix(p, "/files/"); ok {
		return dirOfRoute(rest)
	}
	return ""
}
