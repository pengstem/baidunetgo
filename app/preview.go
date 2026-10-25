package app

import (
	"context"
	"errors"
	"math"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/audio"
	_ "github.com/egoist/mygo/audio/mp3"
	_ "github.com/egoist/mygo/audio/vorbis"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/pdf"
	"github.com/pengstem/baidunetgo/internal/player"
	"github.com/pengstem/baidunetgo/internal/remote"
)

// Preview limits: files larger than these are not loaded into memory.
var (
	imageLimit int64 = 50 << 20
	textLimit  int64 = 8 << 20
	pdfLimit   int64 = 200 << 20
)

// thumbMatch ties a file's thumbnail to its picture in the preview, which
// grows from it and shrinks back to it.
type thumbMatch struct{ id int64 }

// previewState is the file being previewed.
type previewState struct {
	file   baidu.File
	kind   kind
	group  []baidu.File // the pictures beside it, for the arrows
	cancel context.CancelFunc

	loading  bool
	progress float32
	err      string

	thumb   *ui.Bitmap
	bmp     *ui.Bitmap
	data    []byte // the picture's file, to copy
	zoom    float32
	pan     ui.Point
	dismiss ui.Point
	// follow is set while the fingers pinch or pan the picture, which
	// then follows them at once rather than easing after them.
	follow bool
	view   ui.Rect // the viewer's box in the last frame

	buf      *editor.Buffer
	ed       editor.State
	markdown string
	source   bool // show a Markdown file's source

	doc       *pdf.Doc
	pages     []pdfPage
	pdfBase   float32 // how wide the pages fit the window
	pdfScroll ui.ScrollState

	player *audio.Player
	stream *remote.File
}

func (pv *previewState) close() {
	if pv.cancel != nil {
		pv.cancel()
	}
	if pv.player != nil {
		pv.player.Close()
	}
	if pv.doc != nil {
		doc := pv.doc
		go doc.Close()
	}
}

// openPreview shows a file's content: in the overlay for what the app
// shows itself, in mpv for video and audio it does not play.
func (a *App) openPreview(f baidu.File) {
	k := kindOf(&f)
	switch {
	case k == kindVideo:
		a.playInMPV(f)
		return
	case k == kindAudio && !inAppAudio(f.Name):
		a.playInMPV(f)
		return
	}
	if a.pv != nil {
		a.pv.close()
	}
	pv := &previewState{file: f, kind: k, zoom: 1}
	if k == kindImage {
		if l := a.listings[path.Dir(f.Path)]; l != nil {
			for _, g := range l.files {
				if kindOf(&g) == kindImage {
					pv.group = append(pv.group, g)
				}
			}
		}
		if f.Thumb != "" && a.thumbs != nil {
			for _, small := range []bool{false, true} {
				if e := a.thumbs.entries[thumbKeyOf(&f, small)]; e != nil && e.bmp != nil && pv.thumb == nil {
					pv.thumb = e.bmp
				}
			}
		}
	}
	a.pv = pv
	a.loadPreview(pv)
}

func inAppAudio(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".mp3", ".ogg", ".oga", ".wav":
		return true
	}
	return false
}

// closePreview closes the overlay.
func (a *App) closePreview() {
	if a.pv != nil {
		a.pv.close()
		a.pv = nil
	}
}

// loadPreview reads the file into memory and decodes it, on goroutines.
func (a *App) loadPreview(pv *previewState) {
	s := a.sess
	if s == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	pv.cancel = cancel
	var limit int64
	switch pv.kind {
	case kindImage:
		limit = imageLimit
	case kindText, kindCode, kindMarkdown:
		limit = textLimit
	case kindPDF:
		limit = pdfLimit
		pdf.Warm()
	case kindAudio:
		a.startAudio(ctx, pv)
		return
	default:
		return // shows what it is, with nothing to load
	}
	pv.loading = true
	f := pv.file
	go func() {
		last := time.Now()
		data, err := remote.ReadAll(ctx, s.client, baidu.Expired, f.Path, f.Size, limit, func(done int64) {
			if time.Since(last) < 50*time.Millisecond || f.Size == 0 {
				return
			}
			last = time.Now()
			frac := float32(float64(done) / float64(f.Size))
			a.update(func() { pv.progress = frac })
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			a.update(func() { pv.loading, pv.err = false, previewError(err) })
			return
		}
		switch pv.kind {
		case kindImage:
			bmp, err := ui.DecodeBitmap(data)
			a.update(func() {
				pv.loading, pv.progress = false, 1
				if err != nil {
					pv.err = "这种图片格式无法预览"
					return
				}
				pv.bmp, pv.data = bmp, data
			})
		case kindPDF:
			doc, err := pdf.Open(ctx, data)
			if ctx.Err() != nil {
				if doc != nil {
					doc.Close()
				}
				return
			}
			a.update(func() {
				pv.loading = false
				if err != nil {
					pv.err = err.Error()
					return
				}
				pv.doc = doc
				pv.pages = make([]pdfPage, len(doc.Pages))
			})
		default:
			text := decodeText(data)
			a.update(func() {
				pv.loading = false
				if pv.kind == kindMarkdown {
					pv.markdown = text
				}
				pv.buf = editor.NewBuffer(text)
				pv.buf.SetLanguage(editor.LanguageFor(pv.file.Name))
				pv.ed = editor.State{Buffer: pv.buf, ReadOnly: true, SoftWrap: pv.kind != kindCode, NoScrollPastEnd: true}
			})
		}
	}()
}

func previewError(err error) string {
	if errors.Is(err, remote.ErrTooLarge) {
		return "文件太大，不在内存中预览。可以下载后查看。"
	}
	return err.Error()
}

// decodeText reads text as UTF-8, else as GB18030, as Chinese Windows
// writes it.
func decodeText(data []byte) string {
	data = []byte(strings.TrimPrefix(string(data), "\ufeff"))
	if utf8.Valid(data) {
		return string(data)
	}
	if out, err := simplifiedchinese.GB18030.NewDecoder().Bytes(data); err == nil {
		return string(out)
	}
	return strings.ToValidUTF8(string(data), "�")
}

// startAudio plays an audio file as it streams into memory.
func (a *App) startAudio(ctx context.Context, pv *previewState) {
	s := a.sess
	pv.loading = true
	f := pv.file
	go func() {
		stream := remote.Open(ctx, s.client, baidu.Expired, f.Path, f.Size)
		p, err := audio.OpenStream(stream)
		if ctx.Err() != nil {
			if p != nil {
				p.Close()
			}
			return
		}
		a.update(func() {
			pv.loading = false
			if err != nil {
				pv.err = "无法播放：" + err.Error()
				return
			}
			pv.player, pv.stream = p, stream
			p.Play()
		})
	}()
}

// playInMPV streams a file in mpv.
func (a *App) playInMPV(f baidu.File) {
	s := a.sess
	if s == nil {
		return
	}
	mpv, err := player.Find(a.settings.MPV)
	if err != nil {
		a.toast(err.Error(), "error")
		return
	}
	if a.stream == nil {
		a.toast("本地串流服务没有启动", "error")
		return
	}
	// mpv reads through the app's loopback server, which turns its
	// open-ended ranges into the bounded ones the netdisk serves.
	if err := player.Play(mpv, a.stream.URL(f.Path, f.Size), f.Name, nil); err != nil {
		a.toast("播放失败："+err.Error(), "error")
		return
	}
	a.toast("正在用 mpv 播放「"+f.Name+"」", "")
}

// step shows the picture before (-1) or after (+1) in its folder.
func (a *App) step(d int) {
	pv := a.pv
	if pv == nil || len(pv.group) < 2 {
		return
	}
	i := (pv.groupIndex() + d + len(pv.group)) % len(pv.group)
	a.openPreview(pv.group[i])
}

// previewOverlay shows the file being previewed over the window.
func (a *App) previewOverlay(c *ui.Context, t *ui.Theme, p *palette) {
	ui.Overlay(c, func() {
		pv := a.pv
		if pv == nil {
			return
		}
		// The panel itself takes the focus, not its first button, which
		// Space would press.
		root := ui.Box(c).Key("preview").Absolute().Top(0).Left(0).Right(0).Bottom(0).Modal().
			Focusable().FocusRing(false).AutoFocus().
			Transition(ui.ElementTransition{Animation: &motionHover, Enter: &ui.Motion{Opacity: 0}, Exit: &ui.Motion{Opacity: 0}})
		root.Children(func() {
			back := ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0)
			back.Children(func() {
				ui.Box(c).Fill().Background(previewScrim(t)).PassThrough()
			})
			drag := pv.dismiss.Y
			if drag < 0 {
				drag = -drag
			}
			back.Opacity(1 - min(drag/420, 0.7))
			if back.Clicked() {
				a.closePreview()
				return
			}
			ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Children(func() {
				a.previewBar(c, t, p, pv)
				ui.Box(c).Grow(1).Center().Padding(0, 24, 24).Children(func() {
					a.previewBody(c, t, p, pv)
				})
			})
			// Space closes it too, as Quick Look, unless it plays.
			if root.OverlayShortcut(0, ui.KeyEscape) || (pv.kind != kindAudio && root.OverlayShortcut(0, ui.KeySpace)) {
				a.closePreview()
			}
			if pv.kind == kindImage {
				// Back and forward too: a touchpad's swipes between pages
				// and a mouse's side buttons, which the router under the
				// overlay never gets.
				if root.OverlayShortcut(0, ui.KeyLeft) || root.OverlayShortcut(0, ui.KeyBack) {
					a.step(-1)
				}
				if root.OverlayShortcut(0, ui.KeyRight) || root.OverlayShortcut(0, ui.KeyForward) {
					a.step(1)
				}
				if pv.bmp != nil && root.OverlayShortcut(ui.Cmd, ui.KeyC) {
					a.copyImage(pv.file, pv.data)
				}
			}
			if pv.zoomable() {
				if root.OverlayShortcut(ui.Cmd, ui.KeyEqual) {
					pv.zoomStep(1.5)
				}
				if root.OverlayShortcut(ui.Cmd, ui.KeyMinus) {
					pv.zoomStep(1 / 1.5)
				}
				if root.OverlayShortcut(ui.Cmd, ui.Key0) {
					pv.zoomStep(0)
				}
			}
		})
	})
}

func previewScrim(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(8, 8, 7, 0.92)
	}
	return ui.RGBA(20, 20, 18, 0.86)
}

// previewBar is the file's name and actions above it.
func (a *App) previewBar(c *ui.Context, t *ui.Theme, p *palette, pv *previewState) {
	bar := c.TitleBar()
	white := ui.Hex("#ecece8")
	ui.Row(c).Height(barHeight(c)).PaddingStart(max(bar.Left, 0) + 20).PaddingEnd(bar.Right + 8).
		Gap(12).AlignItems(ui.Center).TextColor(white).DragWindow().
		Transition(ui.ElementTransition{Animation: &motionPanel, Enter: &ui.Motion{Y: -12, Opacity: 0}}).Children(func() {
		lk := pv.kind.look(p)
		ui.Icon(c, icon(lk.icon)).FontSize(18).TextColor(ui.RGBA(236, 236, 232, 0.6))
		ui.Column(c).Gap(1).Shrink(1).Children(func() {
			ui.Text(c, pv.file.Name).FontWeight(500).SingleLine()
			meta := lk.label + " · " + humanSize(pv.file.Size)
			if pv.doc != nil {
				meta += " · 第 " + itoa(pv.pdfShown()+1) + " / " + itoa(len(pv.doc.Pages)) + " 页"
			}
			if pv.bmp != nil {
				w, h := pv.bmp.Size()
				meta += " · " + itoa(w) + "×" + itoa(h)
			}
			if n := len(pv.group); n > 1 {
				meta += " · " + itoa(pv.groupIndex()+1) + " / " + itoa(n)
			}
			ui.Text(c, meta).FontSize(t.Rem(0.82)).TextColor(white.Alpha(0.6)).SingleLine()
		})
		ui.Spacer(c)
		if pv.zoomable() {
			if glassButton(c, "zoom-out", "缩小").Clicked() {
				pv.zoomStep(1 / 1.5)
			}
			ui.Text(c, itoa(int(pv.zoom*100+0.5))+"%").Width(48).TextAlign(ui.Center).FontFeatures("tnum").
				TextColor(white.Alpha(0.75)).ContentTransition(ui.ContentTransition{Numeric: true})
			if glassButton(c, "zoom-in", "放大").Clicked() {
				pv.zoomStep(1.5)
			}
		}
		if pv.kind == kindImage && pv.bmp != nil {
			if glassButton(c, "copy", "复制图片").Clicked() {
				a.copyImage(pv.file, pv.data)
			}
		}
		if pv.buf != nil {
			if glassButton(c, "copy", "复制全文").Clicked() {
				a.copy(pv.buf.String(), "已复制全文")
			}
		}
		if pv.kind == kindMarkdown && pv.buf != nil {
			label := "源码"
			if pv.source {
				label = "预览"
			}
			if glassButton(c, "file-code", label).Clicked() {
				pv.source = !pv.source
			}
		}
		if pv.kind == kindAudio || pv.kind == kindVideo {
			if glassButton(c, "external-link", "在 mpv 中播放").Clicked() {
				if pv.player != nil {
					pv.player.Pause()
				}
				a.playInMPV(pv.file)
			}
		}
		if glassButton(c, "download", "下载").Clicked() {
			a.download([]baidu.File{pv.file})
		}
		if glassButton(c, "x", "关闭").Clicked() {
			a.closePreview()
		}
	})
}

// zoomable reports whether the preview zooms: a picture, or a PDF.
func (pv *previewState) zoomable() bool {
	return pv.kind == kindImage && pv.bmp != nil || pv.doc != nil
}

// zoomBy zooms the picture by f, within 1 to 8 times; 0 fits it to the
// window again.
func (pv *previewState) zoomBy(f float32) {
	pv.zoom = clamp(pv.zoom*f, 1, 8)
	if pv.zoom == 1 {
		pv.pan = ui.Point{}
	}
}

// zoomAt zooms the picture by f, within 1 to 8 times, keeping the point
// under (x, y) in the viewer where it is.
func (pv *previewState) zoomAt(f, x, y float32) {
	old := pv.zoom
	pv.zoomBy(f)
	if pv.zoom == 1 || pv.zoom == old {
		return
	}
	// The picture shows centered in the viewer, moved by the pan.
	r := pv.zoom / old
	dx, dy := x-pv.view.W/2-pv.pan.X, y-pv.view.H/2-pv.pan.Y
	pv.pan.X += dx * (1 - r)
	pv.pan.Y += dy * (1 - r)
}

// glassButton is a round icon button over the dark preview backdrop.
func glassButton(c *ui.Context, ic, label string) ui.Element {
	b := ui.ButtonBase(c).Label(label).Tooltip(label).Size(32, 32).Radius(16).Center().
		TextColor(ui.Hex("#ecece8"))
	bg := ui.RGBA(255, 255, 255, 0.08)
	if b.Pressed() {
		bg = ui.RGBA(255, 255, 255, 0.22)
	} else if b.Hovered() {
		bg = ui.RGBA(255, 255, 255, 0.15)
	}
	b.Background(b.AnimateColor("bg", bg, motionHover))
	b.Children(func() { ui.Icon(c, icon(ic)).FontSize(16) })
	return b
}

func (a *App) previewBody(c *ui.Context, t *ui.Theme, p *palette, pv *previewState) {
	switch {
	case pv.err != "":
		a.previewMessage(c, t, p, pv, "triangle-alert", pv.err)
	case pv.kind == kindImage:
		a.imageViewer(c, t, p, pv)
	case pv.loading:
		a.previewLoading(c, t, p, pv)
	case pv.kind == kindPDF && pv.doc != nil:
		a.pdfViewer(c, t, p, pv)
	case pv.kind == kindAudio && pv.player != nil:
		a.audioPlayer(c, t, p, pv)
	case pv.buf != nil:
		a.textViewer(c, t, p, pv)
	default:
		a.previewMessage(c, t, p, pv, pv.kind.look(p).icon, "这种文件不能在应用内预览")
	}
}

// previewLoading shows the bytes arriving.
func (a *App) previewLoading(c *ui.Context, t *ui.Theme, p *palette, pv *previewState) {
	ui.Column(c).Width(280).Gap(12).AlignItems(ui.Center).TextColor(ui.Hex("#f4f5f8")).Transition(popIn).Children(func() {
		lk := pv.kind.look(p)
		ic := ui.Icon(c, icon(lk.icon)).FontSize(40).TextColor(lk.color)
		ic.Opacity(0.6 + 0.4*ic.Loop("pulse", 1200*time.Millisecond, ui.Bounce(ui.EaseInOut)))
		track := ui.Box(c).FillWidth().Height(4).Radius(2).Background(ui.RGBA(255, 255, 255, 0.12)).Clip()
		track.Children(func() {
			bar := ui.Box(c).Height(4).Radius(2).Background(t.Accent)
			bar.WidthPercent(bar.AnimateTo("p", pv.progress*100, motionPanel))
		})
		ui.Text(c, "正在载入到内存 · "+humanSize(int64(float64(pv.progress)*float64(pv.file.Size)))+" / "+humanSize(pv.file.Size)).
			FontSize(t.Rem(0.86)).TextColor(ui.RGBA(244, 245, 248, 0.6)).FontFeatures("tnum")
	})
}

func (a *App) previewMessage(c *ui.Context, t *ui.Theme, p *palette, pv *previewState, ic, msg string) {
	ui.Column(c).Width(360).Padding(24).Gap(12).Radius(8).AlignItems(ui.Center).Background(p.Card).
		Border(1, p.Hairline).Shadow(0, 16, 40, -12, ui.RGBA(0, 0, 0, 0.45)).Transition(popIn).Children(func() {
		ui.Icon(c, icon(ic)).FontSize(28).TextColor(p.Faint)
		ui.Text(c, pv.file.Name).FontWeight(500).TextAlign(ui.Center).MaxLines(2)
		ui.Text(c, msg).TextColor(t.TextMuted).TextAlign(ui.Center)
		ui.Row(c).Gap(8).Margin(6, 0, 0).Children(func() {
			if pill(c, t, p, "download", "下载到本地", true).Clicked() {
				a.download([]baidu.File{pv.file})
			}
		})
	})
}

// imageViewer shows a picture, grown from its thumbnail, zoomed with a
// pinch, the wheel and the buttons, panned with two fingers or dragged,
// and dragged down to close.
func (a *App) imageViewer(c *ui.Context, t *ui.Theme, p *palette, pv *previewState) {
	src := pv.bmp
	if src == nil {
		src = pv.thumb
	}
	ww, wh := c.Size()
	availW, availH := ww-48, wh-titleBarMin-8-24-40
	box := ui.Box(c).Fill().Center()
	pv.view = box.Bounds()
	box.HandleInput(func(ev ui.InputEvent) bool {
		if pv.bmp == nil {
			return false
		}
		switch {
		case ev.Kind == ui.InputZoom:
			pv.zoomAt(ev.Scale, ev.X, ev.Y)
			pv.follow = true
		case ev.Kind == ui.InputScroll && !ev.Precise:
			// A mouse wheel's notches zoom.
			pv.zoomAt(float32(math.Pow(1.0015, float64(-ev.DY))), ev.X, ev.Y)
		case ev.Kind == ui.InputScroll && pv.zoom > 1:
			// A touchpad's two fingers move the picture, as they would
			// the page under it.
			pv.pan.X -= ev.DX
			pv.pan.Y -= ev.DY
			pv.follow = true
		default:
			return false
		}
		return true
	})
	box.Children(func() {
		if src == nil {
			a.previewLoading(c, t, p, pv)
			return
		}
		iw, ih := src.Size()
		if pv.bmp == nil && pv.thumb != nil {
			// Size the thumbnail as the picture will be: the netdisk's are
			// of its proportions.
			iw, ih = iw*8, ih*8
		}
		scale := min(availW/float32(iw), availH/float32(ih), 1)
		if pv.bmp == nil {
			scale = min(availW/float32(iw), availH/float32(ih))
		}
		w, h := float32(iw)*scale, float32(ih)*scale
		z := box.AnimateTo("zoom", pv.zoom, motionPanel)
		move := motionPanel
		if pv.follow {
			z = box.Jump("zoom", pv.zoom)
			move = ui.Animation{Duration: time.Nanosecond}
			pv.follow = false
		}
		img := ui.Image(c, src).Size(w*z, h*z).Fit(ui.Contain).Radius(3).
			Shadow(0, 30, 80, -20, ui.RGBA(0, 0, 0, 0.6)).
			ContentTransition(ui.ContentTransition{}).
			Transition(ui.ElementTransition{Match: thumbMatch{pv.file.FsID}, Animation: &motionPanel})
		if pv.zoom > 1 {
			img.Draggable(&pv.pan, ui.DraggableOptions{
				Bounds:    &ui.Rect{X: -w * z / 2, Y: -h * z / 2, W: w * z, H: h * z},
				Animation: move,
			}).Cursor(ui.CursorGrab)
		} else {
			img.Draggable(&pv.dismiss, ui.DraggableOptions{
				Axis: ui.Vertical,
				Snap: func(pt ui.Point) ui.Point {
					if pt.Y > 160 || pt.Y < -160 {
						return ui.Point{Y: pt.Y * 3}
					}
					return ui.Point{}
				},
			})
			if pv.dismiss.Y != 0 && !img.Transitioning() && !img.Dragging() {
				pv.dismiss = ui.Point{}
				a.closePreview()
				return
			}
		}
		if pv.bmp != nil {
			a.imageMenu(img, pv)
		}
		if img.DoubleClicked() {
			if pv.zoom > 1 {
				pv.zoom, pv.pan = 1, ui.Point{}
			} else {
				pv.zoom = 2.5
			}
		}
		if pv.loading && pv.thumb != nil {
			ui.Box(c).Absolute().Bottom(0).Left(0).Right(0).Height(3).Children(func() {
				bar := ui.Box(c).Height(3).Background(t.Accent)
				bar.WidthPercent(bar.AnimateTo("p", pv.progress*100, motionPanel))
			})
		}
	})
	if len(pv.group) > 1 && pv.zoom == 1 {
		for _, side := range []int{-1, 1} {
			b := glassButton(c.Key(side), map[int]string{-1: "chevron-left", 1: "chevron-right"}[side], map[int]string{-1: "上一张", 1: "下一张"}[side]).
				Size(36, 36).Radius(5).Absolute().Top(0).Bottom(0).Margin(ui.Auto, 0)
			if side < 0 {
				b.Left(8)
			} else {
				b.Right(8)
			}
			if b.Clicked() {
				a.step(side)
			}
		}
	}
}

// imageMenu gives the picture its context menu.
func (a *App) imageMenu(img ui.Element, pv *previewState) {
	img.ContextMenu(func(m *ui.Menu) {
		if m.Item("复制图片").Shortcut(ui.Cmd, ui.KeyC).Chosen() {
			a.copyImage(pv.file, pv.data)
		}
		if m.Item("复制路径").Chosen() {
			a.copy(pv.file.Path, "已复制路径")
		}
		m.Separator()
		if m.Item("放大").Shortcut(ui.Cmd, ui.KeyEqual).Chosen() {
			pv.zoomBy(1.5)
		}
		if m.Item("缩小").Shortcut(ui.Cmd, ui.KeyMinus).Chosen() {
			pv.zoomBy(1 / 1.5)
		}
		if pv.zoom > 1 && m.Item("适合窗口").Shortcut(ui.Cmd, ui.Key0).Chosen() {
			pv.zoomBy(0)
		}
		m.Separator()
		if m.Item("下载").Chosen() {
			a.download([]baidu.File{pv.file})
		}
	})
}

// textViewer shows text and code, read-only with highlighting, and
// Markdown rendered.
func (a *App) textViewer(c *ui.Context, t *ui.Theme, p *palette, pv *previewState) {
	panel := ui.Column(c).Fill().MaxWidth(1040).Radius(8).Background(t.Background).Clip().
		Border(1, p.Hairline).Shadow(0, 16, 40, -12, ui.RGBA(0, 0, 0, 0.45)).
		Transition(ui.ElementTransition{Animation: &motionPanel, Enter: &ui.Motion{Y: 8, Opacity: 0}})
	panel.Children(func() {
		if pv.kind == kindMarkdown && !pv.source {
			ui.Scroll(c).Key("md").Grow(1).Padding(28, 40, 40).Children(func() {
				mdView(c, pv.markdown)
			})
			return
		}
		editor.View(c, &pv.ed).Grow(1)
	})
}

// The pages of a PDF, as the viewer lays them out.
const (
	pdfTop    = 4
	pdfGap    = 18
	pdfBottom = 40
	pdfSide   = 24 // beside pages wider than the view
	// pdfPixels is the most pixels a page is drawn whole with: zoomed in
	// further, the part of it in view is drawn sharp over it.
	pdfPixels = 8 << 20
	// pdfAround is how far around the view, in views, a part is drawn, so
	// that scrolling a little does not draw it again.
	pdfAround = 0.25
)

// pdfPage is what is drawn of a page of the PDF.
type pdfPage struct {
	bmp     *ui.Bitmap
	width   int // how many pixels wide bmp draws the page
	drawing bool

	part        *ui.Bitmap
	partOf      pdf.Part // where part is on the page
	partWidth   int      // how many pixels wide part draws the whole page
	drawingPart bool
}

// pdfLayout is where the pages of a PDF go, each w wide, centered in
// content contentW wide.
type pdfLayout struct {
	w, contentW, contentH float32
	tops, heights         []float32
}

func layoutPDF(pages []pdf.Page, base, zoom, viewW float32) pdfLayout {
	l := pdfLayout{w: base * zoom}
	l.contentW = max(viewW, l.w+2*pdfSide)
	y := float32(pdfTop)
	for _, pg := range pages {
		h := l.w * float32(pg.Height/pg.Width)
		l.tops = append(l.tops, y)
		l.heights = append(l.heights, h)
		y += h + pdfGap
	}
	l.contentH = y - pdfGap + pdfBottom
	return l
}

func (l *pdfLayout) left() float32 { return (l.contentW - l.w) / 2 }

// pageAt returns the page at y in the content: the one above, between
// pages.
func (l *pdfLayout) pageAt(y float32) int {
	i := 0
	for i+1 < len(l.tops) && l.tops[i+1] <= y {
		i++
	}
	return i
}

func (pv *previewState) pdfLayout() pdfLayout {
	return layoutPDF(pv.doc.Pages, pv.pdfBase, pv.zoom, pv.view.W)
}

// zoomPDF zooms the pages by f, within 1 to 8 times, 0 fitting them to
// the window again, keeping the point at (x, y) in the view where it is.
func (pv *previewState) zoomPDF(f, x, y float32) {
	was := pv.pdfLayout()
	old := pv.zoom
	pv.zoomBy(f)
	if pv.zoom == old || was.w == 0 {
		return
	}
	now := pv.pdfLayout()
	s := &pv.pdfScroll
	cx, cy := s.X+x, s.Y+y
	i := was.pageAt(cy)
	fx, fy := (cx-was.left())/was.w, (cy-was.tops[i])/was.heights[i]
	s.X = clamp(now.left()+fx*now.w-x, 0, max(0, now.contentW-pv.view.W))
	s.Y = clamp(now.tops[i]+fy*now.heights[i]-y, 0, max(0, now.contentH-pv.view.H))
}

// zoomStep zooms by f, as the buttons, the keys and the menu do: a PDF
// about the middle of the view.
func (pv *previewState) zoomStep(f float32) {
	if pv.doc != nil {
		pv.zoomPDF(f, pv.view.W/2, pv.view.H/2)
		return
	}
	pv.zoomBy(f)
}

// pdfShown returns the first page in view.
func (pv *previewState) pdfShown() int {
	l := pv.pdfLayout()
	i := l.pageAt(pv.pdfScroll.Y)
	if i+1 < len(l.tops) && l.tops[i]+l.heights[i] <= pv.pdfScroll.Y {
		i++ // the gap below it is at the top
	}
	return i
}

// pdfViewer shows the pages of a PDF, drawing those in view, zoomed with a
// pinch, Cmd and the wheel, the buttons and the keys.
func (a *App) pdfViewer(c *ui.Context, t *ui.Theme, p *palette, pv *previewState) {
	ww, wh := c.Size()
	pv.pdfBase = min(ww-120, 900)
	sc := ui.ScrollBoth(c).Fill().TrackScroll(&pv.pdfScroll)
	pv.view = sc.Bounds()
	if pv.view.W <= 0 {
		// Not laid out yet: as the body around it will be.
		pv.view = ui.Rect{W: ww - 48, H: wh - titleBarMin - 8 - 24}
	}
	sc.HandleInput(func(ev ui.InputEvent) bool {
		switch {
		case ev.Kind == ui.InputZoom:
			pv.zoomPDF(ev.Scale, ev.X, ev.Y)
		case ev.Kind == ui.InputScroll && ev.Mods&ui.Cmd != 0 && !ev.Momentum:
			// A fling's momentum scrolls on with Cmd pressed after it,
			// rather than zooming by what is left of it.
			pv.zoomPDF(float32(math.Pow(1.0015, float64(-ev.DY))), ev.X, ev.Y)
		default:
			return false
		}
		return true
	})
	l := pv.pdfLayout()
	s, vh := pv.pdfScroll, pv.view.H
	sc.Children(func() {
		ui.Box(c).Size(l.contentW, l.contentH).Children(func() {
			for i := range pv.pages {
				top, h := l.tops[i], l.heights[i]
				switch {
				case top+h < s.Y-vh/2 || top > s.Y+vh*1.5:
					// Out of view: kept a while, for scrolling back.
					if top+h < s.Y-4*vh || top > s.Y+5*vh {
						pv.pages[i] = pdfPage{drawing: pv.pages[i].drawing, drawingPart: pv.pages[i].drawingPart}
					}
					pv.pages[i].part = nil
					continue
				}
				a.pdfPage(c.Key(i), pv, &l, i)
			}
		})
	})
}

// pdfPage shows page i, drawing it as sharp as it shows.
func (a *App) pdfPage(c *ui.Context, pv *previewState, l *pdfLayout, i int) {
	pp := &pv.pages[i]
	pg := pv.doc.Pages[i]
	top, h := l.tops[i], l.heights[i]
	want := int(l.w * 2)
	whole := int(math.Sqrt(pdfPixels * pg.Width / pg.Height))
	if fw := min(want, whole); !pp.drawing && (pp.bmp == nil || pp.width < fw*9/10 || pp.width > fw*2) {
		a.drawPage(pv, i, fw)
	}
	if want <= whole*21/20 {
		pp.part = nil
	} else if !pp.drawingPart {
		// The part of the page in view, in fractions of it.
		s := pv.pdfScroll
		x0, x1 := (s.X-l.left())/l.w, (s.X+pv.view.W-l.left())/l.w
		y0, y1 := (s.Y-top)/h, (s.Y+pv.view.H-top)/h
		in := pdf.Part{X: float64(max(x0, 0)), Y: float64(max(y0, 0))}
		in.W, in.H = float64(min(x1, 1))-in.X, float64(min(y1, 1))-in.Y
		if in.W > 0 && in.H > 0 && (pp.part == nil || pp.partWidth != want || !covers(pp.partOf, in)) {
			ax, ay := pdfAround*pv.view.W/l.w, pdfAround*pv.view.H/h
			a.drawPart(pv, i, pixelPart(clamp(x0-ax, 0, 1), clamp(y0-ay, 0, 1), clamp(x1+ax, 0, 1), clamp(y1+ay, 0, 1), want, int(float64(want)*pg.Height/pg.Width)), want)
		}
	}
	page := ui.Box(c).Absolute().Top(top).Left(l.left()).Size(l.w, h).Radius(4).Background(ui.Hex("#ffffff")).Center().
		Shadow(0, 12, 40, -12, ui.RGBA(0, 0, 0, 0.5))
	page.Children(func() {
		if pp.bmp == nil {
			ui.Spinner(c).Label("渲染中")
			return
		}
		ui.Image(c, pp.bmp).Fill().Fit(ui.FillBox).Transition(ui.ElementTransition{Enter: &ui.Motion{Opacity: 0}, Animation: &motionHover})
		if pp.part != nil {
			r := pp.partOf
			ui.Image(c, pp.part).Absolute().Left(float32(r.X)*l.w).Top(float32(r.Y)*h).Size(float32(r.W)*l.w, float32(r.H)*h).Fit(ui.FillBox)
		}
	})
}

// pixelPart returns the part of a page from (x0, y0) to (x1, y1), in
// fractions of it, out to the whole pixels of the page drawn w by h, so
// that it lines up with them.
func pixelPart(x0, y0, x1, y1 float32, w, h int) pdf.Part {
	fw, fh := float64(w), float64(h)
	l, t := math.Floor(float64(x0)*fw), math.Floor(float64(y0)*fh)
	r, b := math.Ceil(float64(x1)*fw), math.Ceil(float64(y1)*fh)
	return pdf.Part{X: l / fw, Y: t / fh, W: (r - l) / fw, H: (b - t) / fh}
}

// covers reports whether part a holds all of part b.
func covers(a, b pdf.Part) bool {
	const e = 1e-6
	return a.X <= b.X+e && a.Y <= b.Y+e && a.X+a.W >= b.X+b.W-e && a.Y+a.H >= b.Y+b.H-e
}

// drawPage draws page i width pixels wide on a goroutine.
func (a *App) drawPage(pv *previewState, i, width int) {
	pv.pages[i].drawing = true
	doc := pv.doc
	go func() {
		img, err := doc.Render(i, width)
		var bmp *ui.Bitmap
		if err == nil {
			bmp = ui.NewBitmap(img)
		}
		a.update(func() {
			if a.pv != pv {
				return
			}
			pp := &pv.pages[i]
			pp.drawing = false
			if bmp != nil {
				pp.bmp, pp.width = bmp, width
			}
		})
	}()
}

// drawPart draws part of page i, at the scale that draws it width pixels
// wide, on a goroutine.
func (a *App) drawPart(pv *previewState, i int, part pdf.Part, width int) {
	pv.pages[i].drawingPart = true
	doc := pv.doc
	go func() {
		img, err := doc.RenderPart(i, part, width)
		var bmp *ui.Bitmap
		if err == nil {
			bmp = ui.NewBitmap(img)
		}
		a.update(func() {
			if a.pv != pv {
				return
			}
			pp := &pv.pages[i]
			pp.drawingPart = false
			if bmp != nil {
				pp.part, pp.partOf, pp.partWidth = bmp, part, width
			}
		})
	}()
}

// audioPlayer plays audio that streams as it plays.
func (a *App) audioPlayer(c *ui.Context, t *ui.Theme, p *palette, pv *previewState) {
	pl := pv.player
	playing := pl.Playing()
	if playing {
		c.After(250 * time.Millisecond)
	}
	ui.Column(c).Width(400).Padding(24).Gap(16).Radius(8).AlignItems(ui.Center).Background(p.Card).
		Border(1, p.Hairline).Shadow(0, 16, 40, -12, ui.RGBA(0, 0, 0, 0.45)).Transition(popIn).Children(func() {
		ui.Icon(c, icon("music")).FontSize(28).TextColor(p.Faint).Margin(8, 0, 0)
		ui.Text(c, strings.TrimSuffix(pv.file.Name, path.Ext(pv.file.Name))).FontSize(15).FontWeight(600).TextAlign(ui.Center).MaxLines(2)
		pos, dur := pl.Position(), pl.Duration()
		frac := float64(0)
		if dur > 0 {
			frac = float64(pos) / float64(dur)
		}
		seek := frac
		if ui.Slider(c, &seek, 0, 1).FillWidth().Label("进度").Changed() && dur > 0 {
			pl.Seek(time.Duration(seek * float64(dur)))
		}
		ui.Row(c).FillWidth().Children(func() {
			figure(c, clock(pos)).FontSize(12).TextColor(t.TextMuted)
			ui.Spacer(c)
			figure(c, clock(dur)).FontSize(12).TextColor(t.TextMuted)
		})
		btn := ui.ButtonBase(c).Label(map[bool]string{true: "暂停", false: "播放"}[playing]).Size(44, 44).Radius(22).Center().TextColor(t.AccentText)
		btn.Background(map[bool]ui.Color{true: t.AccentPressed, false: t.Accent}[btn.Pressed()])
		btn.Children(func() {
			ui.Icon(c, icon(map[bool]string{true: "pause", false: "play"}[playing])).FontSize(18).
				ContentTransition(ui.ContentTransition{})
		})
		if btn.Clicked() || c.Shortcut(0, ui.KeySpace) {
			if playing {
				pl.Pause()
			} else {
				pl.Play()
			}
		}
	})
}

func clock(d time.Duration) string {
	s := int(d.Seconds())
	return itoa(s/60) + ":" + pad2(s%60)
}

// groupIndex returns where the picture shown is among its folder's.
func (pv *previewState) groupIndex() int {
	for i, g := range pv.group {
		if g.FsID == pv.file.FsID {
			return i
		}
	}
	return 0
}
