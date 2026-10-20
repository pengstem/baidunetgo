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
	results []baidu.File
	loading bool
	err     string
	list    ui.ListState
}

// search finds files across the netdisk.
func (a *App) search(q string) {
	if q == "" {
		return
	}
	a.router.Push("/search?" + url.Values{"q": {q}}.Encode())
}

func (a *App) searchPage(c *ui.Context, t *ui.Theme, p *palette, q string) {
	s := &a.found
	if s.query != q && a.sess != nil {
		s.query, s.results, s.loading, s.err = q, nil, true, ""
		client := a.sess.client
		go func() {
			res, err := client.Search(context.Background(), "/", q)
			a.update(func() {
				if s.query != q {
					return
				}
				s.loading = false
				if err != nil {
					s.err = err.Error()
					return
				}
				sortFiles(res, baidu.Order{By: "name"})
				s.results = res
			})
		}()
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
			ui.Text(c, "找到 "+itoa(len(s.results))+" 个结果").TextColor(t.TextMuted).Padding(4, 24, 8)
			list := ui.List(c, &s.list, len(s.results), func(i int) {
				f := s.results[i]
				row := ui.Row(c.Key(f.FsID)).Height(rowHeight + 4).PaddingX(12).Gap(12).AlignItems(ui.Center).Radius(5)
				if row.Hovered() {
					row.Background(p.Hover)
				}
				row.Children(func() {
					a.fileIcon(c, t, p, &f, 34)
					ui.Column(c).Grow(1).Shrink(1).Gap(2).Children(func() {
						ui.RichText(c, highlight(f.Name, q, t)...).SingleLine()
						ui.Text(c, path.Dir(f.Path)).FontSize(t.Rem(0.84)).TextColor(t.TextMuted).SingleLine()
					})
					if !f.IsDir {
						ui.Text(c, humanSize(f.Size)).Width(90).TextAlign(ui.End).TextColor(t.TextMuted)
					}
					ui.Text(c, humanTime(f.Mtime, now)).Width(140).TextAlign(ui.End).TextColor(t.TextMuted)
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
				})
			})
			list.Grow(1).Padding(0, 12, 24)
		}
	})
}

// highlight marks where q is in name.
func highlight(name, q string, t *ui.Theme) []ui.Span {
	i := indexFold(name, q)
	if i < 0 || q == "" {
		return []ui.Span{{Text: name, Weight: 500}}
	}
	return []ui.Span{
		{Text: name[:i], Weight: 500},
		{Text: name[i : i+len(q)], Weight: 650, Color: t.Accent},
		{Text: name[i+len(q):], Weight: 500},
	}
}

// searchField searches the netdisk for what is typed, growing as it takes
// the focus.
func (a *App) searchField(c *ui.Context, t *ui.Theme, p *palette) {
	f := ui.SearchField(c.Key("search"), &a.query).Label("搜索网盘").Placeholder("搜索网盘").Height(32).Radius(9)
	w := float32(180)
	if f.FocusWithin() || a.query != "" {
		w = 260
	}
	f.Width(f.AnimateTo("w", w, motionPanel))
	if f.Submitted() {
		a.search(a.query)
	}
}
