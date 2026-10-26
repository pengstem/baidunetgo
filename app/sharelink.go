package app

import (
	"context"
	"errors"
	"path"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
)

// linkState is a share link someone gave, opened to show what it shares
// before any of it is saved: the dlgSaveLink dialog.
type linkState struct {
	text    string // what was pasted or typed
	pwd     string // the password typed, when the text's is missing or wrong
	key     string // the link and password opened, or being opened
	clip    bool   // it came from the clipboard by itself
	opening bool
	err     error
	shared  *baidu.Shared
	avatar  *ui.Bitmap
	dir     string                  // the share's folder showing, "" for its top
	back    bool                    // the folder showing was gone up to
	dirs    map[string][]baidu.File // the share's folders read, by path
	reading map[string]bool
	dirErr  map[string]string
	chosen  map[int64]bool
	dest    picker
	moving  bool // the destination's folders show
}

const (
	shareRowHeight = 40
	shareListMax   = 6.5 * shareRowHeight
)

// openLink shows what a share link in text shares, from the clipboard by
// itself when clip.
func (a *App) openLink(text string, clip bool) {
	a.ask(dlgSaveLink, "/", nil)
	dest := a.settings.ShareDest
	if dest == "" {
		dest = "/"
	}
	a.link = linkState{text: strings.TrimSpace(text), clip: clip, dest: picker{dir: dest}}
}

// checkClipboard opens a share link the clipboard holds, once for each
// link, as the window comes to the front.
func (a *App) checkClipboard(c *ui.Context) {
	text := c.ReadClipboard()
	if len(text) > 4096 {
		return
	}
	link, pwd, ok := baidu.ParseShareLink(text)
	if !ok || a.seenShare(link, pwd) {
		return
	}
	a.openLink(text, true)
}

// seenShare reports whether the clipboard showed a link already, and
// remembers that it has, across launches; links the app copied itself
// count as seen.
func (a *App) seenShare(link, pwd string) bool {
	key := link + "#" + pwd
	if a.settings.SeenShare == key {
		return true
	}
	a.settings.SeenShare = key
	if !a.fake {
		a.settings.save()
	}
	return false
}

// linkPassword is the password the link is opened with: the one typed,
// else the text's.
func (l *linkState) password(parsed string) string {
	if len(l.pwd) == 4 {
		return l.pwd
	}
	if l.pwd != "" {
		return ""
	}
	return parsed
}

// openShare opens the link the text names, when it changed.
func (a *App) openShare(link, pwd string) {
	l := &a.link
	key := link + "#" + pwd
	if key == l.key || a.sess == nil {
		return
	}
	l.key, l.opening, l.err, l.shared, l.avatar = key, true, nil, nil, nil
	l.dir, l.dirs, l.reading, l.dirErr, l.chosen = "", map[string][]baidu.File{}, map[string]bool{}, map[string]string{}, map[int64]bool{}
	c := a.sess.client
	go func() {
		s, err := c.OpenShare(context.Background(), link, pwd)
		a.update(func() {
			if l.key != key {
				return
			}
			l.opening, l.shared, l.err = false, s, err
			if s != nil {
				l.dirs[""] = s.Files
			}
		})
		if s == nil || s.Avatar == "" {
			return
		}
		data, err := fetchSmall(c, s.Avatar)
		if err != nil {
			return
		}
		bmp, err := ui.DecodeBitmap(data)
		a.update(func() {
			if l.key == key && err == nil {
				l.avatar = bmp
			}
		})
	}()
}

// fetchSmall reads a small file of the web, as a photo.
func fetchSmall(c *baidu.Client, u string) ([]byte, error) {
	resp, err := c.Fetch(context.Background(), u, 0, 0)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return readAll(resp.Body, 4<<20)
}

// sharedFiles returns what a folder of the share holds, reading it the
// first time.
func (a *App) sharedFiles(dir string) ([]baidu.File, bool) {
	l := &a.link
	if fs, ok := l.dirs[dir]; ok {
		return fs, true
	}
	if !l.reading[dir] && l.dirErr[dir] == "" && a.sess != nil {
		l.reading[dir] = true
		c, s, key := a.sess.client, l.shared, l.key
		go func() {
			fs, err := c.SharedDir(context.Background(), s, dir)
			a.update(func() {
				if l.key != key {
					return
				}
				l.reading[dir] = false
				if err != nil {
					l.dirErr[dir] = err.Error()
					return
				}
				l.dirs[dir] = fs
			})
		}()
	}
	return nil, false
}

// saveLinkDialog shows what a share link shares, lets the user go through
// its folders and choose some of it, and saves that, or all of it, into a
// folder of the netdisk.
func (a *App) saveLinkDialog(c *ui.Context, t *ui.Theme, p *palette) {
	l := &a.link
	link, parsed, ok := baidu.ParseShareLink(l.text)
	pwd := l.password(parsed)
	if ok {
		a.openShare(link, pwd)
	}
	if s := l.shared; s != nil {
		a.sharerHeader(c, t, p, s)
	} else {
		title, detail := "打开分享链接", "粘贴链接，提取码会自动识别"
		if l.clip {
			title, detail = "剪贴板里有分享链接", "正在看看里面有什么"
		}
		if errors.Is(l.err, baidu.ErrSharePassword) {
			detail = "填上提取码，就能看到里面的文件"
		}
		dialogTitle(c, t, title, detail)
		if !l.clip {
			ui.TextArea(c, &l.text).Lines(2, 4).AutoFocus().Placeholder("https://pan.baidu.com/s/1… 提取码: abcd").Label("分享链接")
		}
		if ok {
			linkChip(c, t, link, parsed)
		}
	}
	if errors.Is(l.err, baidu.ErrSharePassword) || l.pwd != "" && l.shared == nil {
		a.passwordRow(c, t, p, parsed)
	}
	switch {
	case !ok:
		if strings.TrimSpace(l.text) != "" {
			ui.Text(c, "没有找到 pan.baidu.com 的分享链接").TextColor(t.Warning).FontSize(t.Rem(0.88))
		}
	case l.shared != nil:
		a.sharedBrowser(c, t, p)
		a.destRow(c, t, p)
	case l.opening:
		shareSkeleton(c, p)
	case l.err != nil && !errors.Is(l.err, baidu.ErrSharePassword):
		shareProblem(c, t, p, l.err.Error(), func() { l.key = "" })
	}
	label := "全部保存"
	if n := len(l.chosen); n > 0 {
		label = "保存所选 " + itoa(n) + " 项"
	}
	if a.dialogButtons(c, t, p, label, false, l.shared == nil) {
		a.saveShared()
	}
}

// sharerHeader says who shared the link, when, and for how long.
func (a *App) sharerHeader(c *ui.Context, t *ui.Theme, p *palette, s *baidu.Shared) {
	l := &a.link
	owner := s.Owner
	if owner == "" {
		owner = "百度网盘用户"
	}
	ui.Row(c).Key("sharer").Gap(12).AlignItems(ui.Center).Transition(popIn).Children(func() {
		ui.Box(c).Size(38, 38).Radius(19).Center().Shrink(0).Clip().Background(t.Accent.Alpha(0.14)).Children(func() {
			if l.avatar != nil {
				ui.Image(c, l.avatar).Fill().Fit(ui.Cover).Transition(ui.ElementTransition{Enter: &ui.Motion{Opacity: 0}, Animation: &motionFade})
				return
			}
			ui.Text(c, string([]rune(owner)[:1])).FontSize(16).FontWeight(600).TextColor(t.Accent)
		})
		ui.Column(c).Grow(1).Shrink(1).Gap(3).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, owner+" 的分享").FontSize(15).FontWeight(600).SingleLine().Shrink(1)
				if s.Mine {
					chip(c, "我的分享", t.Accent)
				}
			})
			var parts []string
			if s.Created > 0 {
				parts = append(parts, time.Unix(s.Created, 0).Format("2006年1月2日")+"分享")
			}
			parts = append(parts, expiryLabel(s.Expires, c.Now()))
			figure(c, strings.Join(parts, " · ")).FontSize(t.Rem(0.86)).TextColor(t.TextMuted)
		})
		if l.clip {
			ui.Row(c).Gap(4).AlignItems(ui.Center).Shrink(0).Children(func() {
				ui.Icon(c, icon("copy")).FontSize(11).TextColor(p.Faint)
				ui.Text(c, "来自剪贴板").FontSize(11).TextColor(p.Faint)
			})
		}
	})
}

func expiryLabel(expires int64, now time.Time) string {
	if expires == 0 {
		return "永久有效"
	}
	left := time.Unix(expires, 0).Sub(now)
	if left < 24*time.Hour {
		return "今天过期"
	}
	return itoa(int(left.Hours()/24)) + " 天后过期"
}

// linkChip is the link found in the text, and its password.
func linkChip(c *ui.Context, t *ui.Theme, link, pwd string) {
	ui.Row(c).Key("chip").Gap(8).AlignItems(ui.Center).Padding(8, 12).Radius(10).Background(t.Accent.Alpha(0.1)).Transition(rowEnter).Children(func() {
		ui.Icon(c, icon("link")).FontSize(15).TextColor(t.Accent)
		ui.Text(c, strings.TrimPrefix(link, "https://")).SingleLine().Shrink(1)
		ui.Spacer(c)
		if pwd != "" {
			ui.Text(c, "提取码 "+pwd).FontWeight(600).TextColor(t.Accent).SingleLine()
		}
	})
}

// passwordRow asks for the share's password, opening it as the fourth
// character is typed.
func (a *App) passwordRow(c *ui.Context, t *ui.Theme, p *palette, parsed string) {
	l := &a.link
	msg, color := "这个分享需要提取码", t.TextMuted
	if errors.Is(l.err, baidu.ErrSharePassword) && (parsed != "" || len(l.pwd) == 4) {
		msg, color = "提取码不对，再试一次", t.Warning
	}
	ui.Row(c).Key("pwd").Gap(10).AlignItems(ui.Center).Padding(10, 12).Radius(10).Background(p.Group).Transition(rowEnter).Children(func() {
		ui.Text(c, msg).TextColor(color).Grow(1).Shrink(1)
		ui.TextInput(c, &l.pwd).AutoFocus().Width(96).Height(32).Font("monospace").FontSize(16).LetterSpacing(3).
			Placeholder("四位").Label("提取码")
		if len(l.pwd) > 4 {
			l.pwd = l.pwd[:4]
		}
		if l.opening {
			ui.Spinner(c).Label("正在验证")
		}
	})
}

// sharedBrowser lists the folder of the share showing, with a way back up
// and a mark on each entry to choose it.
func (a *App) sharedBrowser(c *ui.Context, t *ui.Theme, p *palette) {
	l := &a.link
	s := l.shared
	top := "/" // the sharer's folder the share's paths start in
	if len(s.Files) > 0 {
		top = path.Dir(s.Files[0].Path)
	}
	files, ok := a.sharedFiles(l.dir)
	ui.Row(c).Gap(6).AlignItems(ui.Center).Height(28).Children(func() {
		if l.dir != "" {
			if iconButton(c, t, p, "arrow-left", "上一级").Clicked() {
				l.back = true
				if up := path.Dir(l.dir); up == top || up == "/" {
					l.dir = ""
				} else {
					l.dir = up
				}
			}
		}
		crumbs := []string{"全部文件"}
		if l.dir != "" {
			crumbs = append(crumbs, strings.Split(strings.Trim(strings.TrimPrefix(l.dir, top), "/"), "/")...)
		}
		ui.Row(c).Grow(1).Shrink(1).Gap(4).AlignItems(ui.Center).Children(func() {
			for i, name := range crumbs {
				last := i == len(crumbs)-1
				if i > 0 {
					ui.Icon(c, icon("chevron-right")).FontSize(11).TextColor(p.Faint)
				}
				txt := ui.Text(c, name).SingleLine()
				if last {
					txt.FontWeight(600).Shrink(1)
				} else {
					txt.TextColor(t.TextMuted).Shrink(4)
				}
			}
		})
		if ok && len(files) > 0 {
			all := true
			for _, f := range files {
				all = all && l.chosen[f.FsID]
			}
			label := "全选"
			if all {
				label = "取消全选"
			}
			b := ui.ButtonBase(c).Label(label).PaddingX(6).Height(24).Radius(6).TextColor(t.Accent)
			if b.Hovered() {
				b.Background(t.Accent.Alpha(0.1))
			}
			b.Children(func() { ui.Text(c, label).FontSize(t.Rem(0.88)).FontWeight(500) })
			if b.Clicked() {
				for _, f := range files {
					if all {
						delete(l.chosen, f.FsID)
					} else {
						l.chosen[f.FsID] = true
					}
				}
			}
		}
	})
	enter := &ui.Motion{X: 10, Opacity: 0}
	if l.back {
		enter = &ui.Motion{X: -10, Opacity: 0}
	}
	box := ui.Column(c.Key("dir:" + l.dir)).Radius(10).Background(p.Group).Clip().
		Transition(ui.ElementTransition{Animation: &motionPanel, Enter: enter})
	box.Children(func() {
		switch {
		case l.dirErr[l.dir] != "":
			dir := l.dir
			shareProblem(c, t, p, l.dirErr[dir], func() { delete(l.dirErr, dir) })
		case !ok:
			shareSkeleton(c, p)
		case len(files) == 0:
			ui.Column(c).Height(2 * shareRowHeight).Center().Children(func() {
				ui.Text(c, "空文件夹").TextColor(t.TextMuted)
			})
		default:
			ui.Scroll(c).MaxHeight(shareListMax).Padding(4).Children(func() {
				for i, f := range files {
					a.sharedRow(c, t, p, f, i)
				}
			})
		}
	})
}

// sharedRow is an entry of the share: a folder opens as it is clicked, a
// file is chosen; the mark at its start chooses either.
func (a *App) sharedRow(c *ui.Context, t *ui.Theme, p *palette, f baidu.File, i int) {
	l := &a.link
	chosen := l.chosen[f.FsID]
	row := ui.ButtonBase(c.Key(f.FsID)).Label(f.Name).Height(shareRowHeight).PaddingX(8).Gap(10).AlignItems(ui.Center).Radius(7)
	if row.Hovered() {
		row.Background(p.Hover)
	}
	if in := entranceOf(row, i); in < 1 {
		row.Opacity(in).Translate(0, (1-in)*8)
	}
	row.Children(func() {
		mark := ui.ButtonBase(c).Label("选择「"+f.Name+"」").Size(20, 20).Radius(10).Center().Shrink(0)
		if chosen {
			mark.Background(t.Accent).Children(func() { ui.Icon(c, icon("check")).FontSize(12).TextColor(t.AccentText) })
		} else {
			ring := p.Faint
			if mark.Hovered() {
				ring = t.Accent
			}
			mark.Border(1.5, ring)
		}
		if mark.Clicked() {
			l.toggle(f.FsID)
		}
		a.fileIcon(c, t, p, &f, 26)
		ui.Column(c).Grow(1).Shrink(1).Gap(1).Children(func() {
			wholeOnHover(c, ui.Text(c, f.Name).SingleLine(), f.Name, 0)
			detail := humanSize(f.Size)
			if f.IsDir {
				detail = "文件夹"
			}
			figure(c, detail).FontSize(11).TextColor(t.TextMuted)
		})
		if f.IsDir {
			ui.Icon(c, icon("chevron-right")).FontSize(13).TextColor(p.Faint)
		}
	})
	if row.Clicked() {
		if f.IsDir {
			l.dir, l.back = f.Path, false
		} else {
			l.toggle(f.FsID)
		}
	}
}

// entranceOf brings the share's rows in one after another.
func entranceOf(e ui.Element, i int) float32 {
	return e.Keyframes("in", []ui.Keyframe{{At: 0, Value: 0}, {At: entranceFor, Value: 1, Ease: easeStandard}},
		ui.KeyframeOptions{Delay: time.Duration(min(i, 12)) * 18 * time.Millisecond})
}

func (l *linkState) toggle(id int64) {
	if l.chosen[id] {
		delete(l.chosen, id)
	} else {
		l.chosen[id] = true
	}
}

// destRow says where the share goes, and opens the folders to choose
// another.
func (a *App) destRow(c *ui.Context, t *ui.Theme, p *palette) {
	l := &a.link
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "保存到").TextColor(t.TextMuted)
		folderGlyph(c, p, 16)
		ui.Text(c, dirLabel(l.dest.dir)).FontWeight(600).SingleLine().Shrink(1).ContentTransition(ui.ContentTransition{})
		ui.Spacer(c)
		label, ic := "更改", "chevron-down"
		if l.moving {
			label, ic = "收起", "chevron-up"
		}
		b := ui.ButtonBase(c).Label(label).Gap(3).PaddingX(8).Height(24).Radius(6).AlignItems(ui.Center).TextColor(t.Accent)
		if b.Hovered() {
			b.Background(t.Accent.Alpha(0.1))
		}
		b.Children(func() {
			ui.Text(c, label).FontSize(t.Rem(0.88)).FontWeight(500)
			ui.Icon(c, icon(ic)).FontSize(11)
		})
		if b.Clicked() {
			l.moving = !l.moving
		}
	})
	if l.moving {
		ui.Column(c).Key("dest").Gap(8).Transition(rowEnter).Children(func() {
			a.folderChooser(c, t, p, &l.dest, 150, nil)
		})
	}
}

// shareSkeleton holds the list's place while the share opens.
func shareSkeleton(c *ui.Context, p *palette) {
	ui.Column(c).Key("skeleton").Padding(4).Radius(10).Background(p.Group).Children(func() {
		for i := range 3 {
			row := ui.Row(c).Height(shareRowHeight).PaddingX(8).Gap(10).AlignItems(ui.Center)
			pulse := row.Loop("pulse", 1400*time.Millisecond, ui.Bounce(ui.EaseInOut))
			row.Opacity(0.5 + 0.5*pulse - float32(i)*0.1)
			row.Children(func() {
				ui.Box(c).Size(20, 20).Radius(10).Background(p.Control)
				ui.Box(c).Size(26, 26).Radius(5).Background(p.Control)
				ui.Column(c).Gap(5).Children(func() {
					ui.Box(c).Size(float32(150+i*47%90), 9).Radius(4).Background(p.Control)
					ui.Box(c).Size(48, 7).Radius(3).Background(p.Control)
				})
			})
		}
	})
}

// shareProblem says why the share or a folder of it could not be read.
func shareProblem(c *ui.Context, t *ui.Theme, p *palette, msg string, retry func()) {
	ui.Column(c).Key("problem").Padding(18).Gap(8).AlignItems(ui.Center).Radius(10).Background(p.Group).Transition(popIn).Children(func() {
		ui.Icon(c, icon("circle-x")).FontSize(22).TextColor(p.Faint)
		ui.Text(c, msg).TextColor(t.TextMuted).TextAlign(ui.Center)
		if pill(c, t, p, "refresh-cw", "重试", false).Clicked() {
			retry()
		}
	})
}

// saveShared saves what was chosen of the share, or all of it, and offers
// to go to where it went.
func (a *App) saveShared() {
	l := &a.link
	s, dest := l.shared, l.dest.dir
	var ids []int64
	for id := range l.chosen {
		ids = append(ids, id)
	}
	a.settings.ShareDest = dest
	if !a.fake {
		a.settings.save()
	}
	a.run(func(ctx context.Context, c *baidu.Client) error {
		_, err := c.SaveShared(ctx, s, ids, dest)
		return err
	}, []string{dest}, "", func() {
		a.pendingToasts = append(a.pendingToasts, ui.Toast{
			Title: "已保存到「" + dirLabel(dest) + "」", Type: "success", Action: "前往",
			OnAction: func() { a.router.Push(filesRoute(dest)) },
		})
	})
}
