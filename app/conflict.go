package app

import (
	"context"
	"path"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// resolution is what to do with an item whose name the folder it goes
// into already has.
type resolution int

const (
	keepBoth resolution = iota // give the one arriving a free name
	replace                    // put the one there in the recycle bin first
	skip                       // leave the one arriving out
)

// clash asks what to do with the items going into a folder that has their
// names.
type clash struct {
	verb  string   // what the items do: 上传, 移动, 复制
	dir   string   // the folder they go into
	names []string // the names it has
	then  func(resolution)
}

// placement is where items go once their names are settled: names[i] is
// the name of the i-th, "" when it is skipped, and replaced is what goes
// to the recycle bin to make room.
type placement struct {
	names    []string
	replaced []baidu.File
}

// place reads dir afresh and settles the names of the items going into
// it, asking what to do when it has some of them already. selves are the
// paths of the items themselves, which a copy into their own folder finds
// there: those always keep both. It calls then on the UI thread, unless
// the user cancels.
func (a *App) place(verb, dir string, names []string, selves []string, then func(placement)) {
	s := a.sess
	if s == nil || len(names) == 0 {
		return
	}
	go func() {
		files, err := s.client.List(context.Background(), dir, baidu.Order{})
		a.update(func() {
			if err != nil {
				a.dlg.busy = false
				a.toast(err.Error(), "error")
				return
			}
			existing := map[string]baidu.File{}
			for _, f := range files {
				existing[strings.ToLower(f.Name)] = f
			}
			self := map[string]bool{}
			for _, p := range selves {
				self[p] = true
			}
			var asked []string
			for _, n := range names {
				if f, ok := existing[strings.ToLower(n)]; ok && !self[f.Path] {
					asked = append(asked, n)
				}
			}
			settle := func(r resolution) {
				taken := map[string]bool{}
				for n := range existing {
					taken[n] = true
				}
				var pl placement
				for _, n := range names {
					f, there := existing[strings.ToLower(n)]
					switch {
					case !there:
					case self[f.Path] || r == keepBoth:
						n = freeName(n, taken)
					case r == skip:
						n = ""
					case r == replace:
						pl.replaced = append(pl.replaced, f)
					}
					if n != "" {
						taken[strings.ToLower(n)] = true
					}
					pl.names = append(pl.names, n)
				}
				then(pl)
			}
			if len(asked) == 0 {
				settle(keepBoth)
				return
			}
			a.dlg = dialog{kind: dlgConflict, open: true, dir: dir, clash: &clash{verb: verb, dir: dir, names: asked, then: settle}}
		})
	}()
}

// freeName returns name, or name with a number before its extension, as
// "照片 (1).jpg", that taken (lowercase) does not hold.
func freeName(name string, taken map[string]bool) string {
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if stem == "" {
		stem, ext = name, ""
	}
	for i := 1; ; i++ {
		cand := stem + " (" + strconv.Itoa(i) + ")" + ext
		if !taken[strings.ToLower(cand)] {
			return cand
		}
	}
}

// conflictDialog asks what to do with the items whose names the folder
// already has.
func (a *App) conflictDialog(c *ui.Context, t *ui.Theme, p *palette) {
	d := &a.dlg
	cl := d.clash
	title := "「" + dirLabel(cl.dir) + "」中已有「" + cl.names[0] + "」"
	if len(cl.names) > 1 {
		title = "「" + dirLabel(cl.dir) + "」中已有 " + itoa(len(cl.names)) + " 个同名项目"
	}
	dialogTitle(c, t, title, "")
	if len(cl.names) > 1 {
		shown := cl.names[:min(len(cl.names), 4)]
		more := strings.Join(shown, "、")
		if len(cl.names) > len(shown) {
			more += " 等"
		}
		ui.Text(c, more).TextColor(t.TextMuted).MaxLines(2)
	}
	ui.Text(c, "要"+cl.verb+"的项目与已有的同名。替换会把已有的移到回收站。").TextColor(t.TextMuted)
	if d.err != "" {
		ui.Text(c, d.err).TextColor(t.Danger).Transition(rowEnter)
	}
	ui.Row(c).Gap(8).Justify(ui.End).Margin(4, 0, 0).Children(func() {
		if pill(c, t, p, "", "取消", false).Disabled(d.busy).Clicked() {
			d.open = false
		}
		ui.Spacer(c)
		choose := func(label string, r resolution, primary bool) {
			b := pill(c, t, p, "", label, primary).Disabled(d.busy)
			if primary {
				b.AutoFocus()
			}
			if b.Clicked() {
				cl.then(r)
			}
		}
		choose("跳过", skip, false)
		choose("替换", replace, false)
		choose("保留两者", keepBoth, true)
	})
}
