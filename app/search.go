package app

import (
	"context"
	"net/url"
	"path"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// searchState is the results of the last search.
type searchState struct {
	query   string
	in      string // the folder searched, "/" for the whole netdisk
	results []baidu.File
	more    bool // there were more results than it read
	loading bool
	err     string
	list    ui.ListState
	scope   int // what the scope switch shows: the whole netdisk, or the folder
}

// search finds files across the netdisk. Searched from a folder, it can
// look in that folder only, a switch away.
func (a *App) search(q string) {
	if q == "" {
		return
	}
	v := url.Values{"q": {q}}
	if dir := a.currentDir(); dir != "" && dir != "/" {
		v.Set("from", dir)
	} else if a.router.Path() == "/search" {
		for _, k := range []string{"from", "in"} {
			if x := a.router.Query(k); x != "" {
				v.Set(k, x)
			}
		}
	}
	a.found.query = "" // searched again, even for the same words
	a.router.Push("/search?" + v.Encode())
}

func (a *App) searchPage(c *ui.Context, t *ui.Theme, p *palette, q string) {
	s := &a.found
	from, in := a.router.Query("from"), a.router.Query("in")
	if in == "" {
		in = "/"
	}
	if (s.query != q || s.in != in) && a.sess != nil {
		s.query, s.in, s.results, s.more, s.loading, s.err = q, in, nil, false, true, ""
		client := a.sess.client
		go func() {
			res, more, err := client.Search(context.Background(), in, q)
			a.update(func() {
				if s.query != q || s.in != in {
					return
				}
				s.loading = false
				if err != nil {
					s.err = err.Error()
					return
				}
				sortFiles(res, baidu.Order{By: "name"})
				s.results, s.more = res, more
			})
		}()
	}
	if from != "" {
		s.scope = 0
		if in != "/" {
			s.scope = 1
		}
		was := s.scope
		ui.Row(c).Padding(8, 10, 0).Children(func() {
			segmented(c, t, p, &s.scope, "整个网盘", "「"+dirLabel(from)+"」").Width(260)
		})
		if s.scope != was {
			v := url.Values{"q": {q}, "from": {from}}
			if s.scope == 1 {
				v.Set("in", from)
			}
			a.router.Replace("/search?" + v.Encode())
		}
	}
	ui.Column(c).Grow(1).Children(func() {
		switch {
		case s.loading:
			skeletonRows(c, t, p)
		case s.err != "":
			emptyState(c, t, p, "triangle-alert", "搜索失败", s.err, nil)
		case len(s.results) == 0:
			emptyState(c, t, p, "search", "没有找到「"+q+"」", "换个关键词试试", nil)
		default:
			now := c.Now()
			list := ui.List(c, &s.list, len(s.results), func(i int) {
				f := s.results[i]
				row := stripe(ui.Row(c.Key(f.FsID)).Height(44).PaddingX(10).Gap(10).AlignItems(ui.Center), p, i)
				row.Children(func() {
					a.fileIcon(c, t, p, &f, 28)
					ui.Column(c).Grow(1).Shrink(1).Gap(1).Children(func() {
						wholeOnHover(c, ui.RichText(c, highlight(f.Name, q, t)...).SingleLine(), f.Name, 0)
						wholeOnHover(c, ui.Text(c, path.Dir(f.Path)).FontSize(11).TextColor(t.TextMuted).SingleLine(), path.Dir(f.Path), 11)
					})
					if !f.IsDir {
						figure(c, humanSize(f.Size)).Width(90).TextAlign(ui.End).TextColor(t.TextMuted)
					}
					figure(c, humanTime(f.Mtime, now)).Width(140).TextAlign(ui.End).TextColor(t.TextMuted)
				})
				if row.DoubleClicked() {
					a.open(f)
				}
				row.ContextMenu(func(m *ui.Menu) {
					if m.Item("打开").Chosen() {
						a.open(f)
					}
					if m.Item("在所在文件夹中显示").Chosen() {
						a.router.Push(filesRoute(path.Dir(f.Path)))
					}
					if m.Item("显示详情").Chosen() {
						a.showInfo(path.Dir(f.Path), []baidu.File{f})
					}
					m.Separator()
					if m.Item("复制路径").Chosen() {
						a.copy(f.Path, "已复制路径")
					}
					if kindOf(&f) == kindImage && m.Item("复制图片").Chosen() {
						a.copyImage(f, nil)
					}
				})
			})
			list.Grow(1).Padding(4, 10, 20)
		}
	})
}

// highlight marks where q is in name.
func highlight(name, q string, t *ui.Theme) []ui.Span {
	i := indexFold(name, q)
	if i < 0 || q == "" {
		return []ui.Span{{Text: name}}
	}
	return []ui.Span{
		{Text: name[:i]},
		{Text: name[i : i+len(q)], Weight: 650, Color: t.Accent},
		{Text: name[i+len(q):]},
	}
}

// searchWidth and searchFolded are how wide the search field is closed,
// in a wide title bar and folded into a button.
const searchWidth, searchFolded = 170, 28

// searchField searches the netdisk for what is typed, growing as it takes
// the focus. It is built of a bare text input, not ui.SearchField, so that
// Cmd-F can put the cursor in it.

func (a *App) searchField(c *ui.Context, t *ui.Theme, p *palette) {
	f := ui.Row(c.Key("search")).Height(28).Radius(14).PaddingStart(9).PaddingEnd(5).Gap(5).AlignItems(ui.Center).Background(p.Control)
	var in ui.Element
	f.Children(func() {
		ui.Icon(c, icon("search")).FontSize(13).TextColor(t.TextMuted)
		in = ui.TextInputBase(c, &a.query).Label("搜索网盘").Placeholder("搜索").Grow(1).MinWidth(0)
		if a.query != "" {
			x := ui.ButtonBase(c).Label("清除").Size(16, 16).Radius(8).Center().Background(t.TextMuted.Alpha(0.5)).TextColor(t.Background)
			x.Children(func() { ui.Icon(c, icon("x")).FontSize(10) })
			if x.Clicked() {
				a.query = ""
				in.Focus()
			}
		}
	})
	// In a narrow title bar it folds into a round button, as Finder's
	// does, which opens it, leaving the room to the path.
	w := float32(searchWidth)
	a.searchOpen = in.Focused() || a.query != ""
	if a.searchOpen {
		w = 240
	} else if a.compact {
		w = searchFolded
		f.Tooltip("搜索")
		if f.Hovered() {
			f.Background(t.SurfaceHover)
		}
		if f.Clicked() {
			in.Focus()
		}
	}
	f.Width(f.AnimateTo("w", w, motionPanel))
	if in.Focused() && in.Shortcut(0, ui.KeyEscape) {
		a.query = ""
	}
	// Cmd-F puts the cursor in it.
	if a.finding {
		in.Focus()
		a.finding = false
	}
	if c.Shortcut(ui.Cmd, ui.KeyF) {
		a.finding = true
		c.Invalidate()
	}
	if in.Submitted() {
		a.search(a.query)
	}
}
