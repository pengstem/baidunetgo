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

	list     ui.ListState
	grid     ui.GridState
	sel      ui.Selection[int64]
	selected int
	used     uint64
}

const listingCacheSize = 40

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
	return l
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
	l.loading, l.err = true, ""
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
			l.files, l.loaded = files, true
			l.sorted = baidu.Order{}
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
