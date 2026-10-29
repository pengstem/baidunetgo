package app

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/fakepan"
)

const (
	keyEscape = ui.KeyEscape
	keyRight  = ui.KeyRight
)

func fileNamed(t *testing.T, a *App, dir, name string) baidu.File {
	t.Helper()
	for _, f := range a.listings[dir].files {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("no %s in %s", name, dir)
	return baidu.File{}
}

func TestPreviewText(t *testing.T) {
	a, tt, _ := signedIn(t)
	a.router.Push(filesRoute("/文档"))
	waitFor(t, tt, "读我.txt")
	a.open(fileNamed(t, a, "/文档", "读我.txt"))
	deadline := time.Now().Add(5 * time.Second)
	for a.pv.buf == nil {
		if a.pv.err != "" || time.Now().After(deadline) {
			t.Fatalf("text did not load: %q", a.pv.err)
		}
		time.Sleep(10 * time.Millisecond)
		tt.Advance(50 * time.Millisecond)
	}
	if got := a.pv.buf.Line(0); got != "BaiduNetGo 演示文件" {
		t.Errorf("first line %q", got)
	}
	tt.Advance(time.Second)
	save(t, tt, "preview-text")
	if err := tt.Click("复制全文"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(100 * time.Millisecond)
	if got := tt.Clipboard(); !strings.HasPrefix(got, "BaiduNetGo 演示文件\n") {
		t.Errorf("clipboard %q", got)
	}
	tt.Key(0, keyEscape)
	tt.Advance(time.Second)
	if a.pv != nil {
		t.Error("Escape did not close the preview")
	}
}

// Files of the newly highlighted languages must reach the code viewer,
// including their alternate extensions, rather than the download card.
func TestPreviewCodeLanguages(t *testing.T) {
	a, tt, srv := signedIn(t)
	cases := []struct{ name, language, source string }{
		{"main.dart", "Dart", "void main() { print('你好'); }"},
		{"query.graphql", "GraphQL", "query { viewer { name } }"},
		{"query.gql", "GraphQL", "query { viewer { name } }"},
		{"schema.graphqls", "GraphQL", "type Query { greeting: String }"},
		{"analysis.R", "R", "mean(c(1, 2, 3))"},
		{".Rprofile", "R", "options(stringsAsFactors = FALSE)"},
		{"Main.scala", "Scala", "object Main { val greeting = \"你好\" }"},
		{"main.sc", "Scala", "val greeting = \"你好\""},
		{"build.sbt", "Scala", "scalaVersion := \"3.3.1\""},
	}
	for _, tc := range cases {
		srv.Add(fakepan.Node{Path: "/代码/" + tc.name, Data: []byte(tc.source)})
	}
	a.router.Push(filesRoute("/代码"))
	waitFor(t, tt, "main.dart")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a.open(fileNamed(t, a, "/代码", tc.name))
			deadline := time.Now().Add(5 * time.Second)
			for a.pv == nil || a.pv.buf == nil {
				if a.pv == nil || a.pv.kind != kindCode || a.pv.err != "" || time.Now().After(deadline) {
					t.Fatalf("code preview did not load: %+v", a.pv)
				}
				time.Sleep(10 * time.Millisecond)
				tt.Advance(50 * time.Millisecond)
			}
			if lang := a.pv.buf.Language(); lang == nil || lang.Name != tc.language {
				t.Fatalf("highlighting %v, want %s", lang, tc.language)
			}
			if got := a.pv.buf.Line(0); got != tc.source {
				t.Errorf("source %q, want %q", got, tc.source)
			}
			if !a.pv.ed.ReadOnly || a.pv.ed.SoftWrap {
				t.Error("code preview must stay read-only without wrapping")
			}
		})
	}
	a.closePreview()
}

func TestPreviewMarkdownAndPDF(t *testing.T) {
	a, tt, _ := signedIn(t)
	a.router.Push(filesRoute("/文档"))
	waitFor(t, tt, "旅行计划.md")
	a.open(fileNamed(t, a, "/文档", "旅行计划.md"))
	waitFor(t, tt, "慢慢走，比较快。")
	tt.Advance(time.Second)
	save(t, tt, "preview-md")
	a.open(fileNamed(t, a, "/文档", "年度报告.pdf"))
	deadline := time.Now().Add(20 * time.Second)
	for a.pv.doc == nil || a.pv.pages[0].bmp == nil {
		if a.pv.err != "" || time.Now().After(deadline) {
			t.Fatalf("pdf did not render: %q", a.pv.err)
		}
		time.Sleep(20 * time.Millisecond)
		tt.Advance(50 * time.Millisecond)
	}
	tt.Advance(time.Second)
	save(t, tt, "preview-pdf")
	if len(a.pv.doc.Pages) != 4 {
		t.Errorf("pages %d", len(a.pv.doc.Pages))
	}
}

func TestPreviewPDFZoom(t *testing.T) {
	a, tt, _ := signedIn(t)
	a.router.Push(filesRoute("/文档"))
	waitFor(t, tt, "年度报告.pdf")
	a.open(fileNamed(t, a, "/文档", "年度报告.pdf"))
	settle := func(what string, done func() bool) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for !done() {
			if a.pv.err != "" || time.Now().After(deadline) {
				t.Fatalf("%s: %q", what, a.pv.err)
			}
			time.Sleep(20 * time.Millisecond)
			tt.Advance(50 * time.Millisecond)
		}
	}
	settle("pdf did not render", func() bool { return a.pv.doc != nil && a.pv.pages[0].bmp != nil })
	tt.Advance(time.Second)
	pv := a.pv
	if pv.pages[0].width != int(pv.pdfBase*2) {
		t.Errorf("drawn %d wide, fits %v", pv.pages[0].width, pv.pdfBase)
	}

	// The buttons and the keys zoom, about the middle of the view.
	if err := tt.Click("放大"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(100 * time.Millisecond)
	if pv.zoom != 1.5 {
		t.Errorf("放大 zoomed to %v", pv.zoom)
	}
	tt.Key(ui.Cmd, ui.Key0)
	tt.Advance(100 * time.Millisecond)
	if pv.zoom != 1 || pv.pdfScroll.X != 0 {
		t.Errorf("Cmd0: zoom %v, scrolled %v", pv.zoom, pv.pdfScroll.X)
	}
	tt.Key(ui.Cmd, ui.KeyEqual)
	tt.Advance(100 * time.Millisecond)
	if pv.zoom != 1.5 {
		t.Errorf("Cmd= zoomed to %v", pv.zoom)
	}
	tt.Key(ui.Cmd, ui.KeyMinus)
	tt.Advance(100 * time.Millisecond)
	if pv.zoom != 1 {
		t.Errorf("Cmd- zoomed to %v", pv.zoom)
	}

	// A pinch keeps the point under it where it is on its page.
	v := pv.view
	x, y := v.W/2+150, v.H/2
	where := func() (int, float32, float32) {
		l := pv.pdfLayout()
		cx, cy := pv.pdfScroll.X+x, pv.pdfScroll.Y+y
		i := l.pageAt(cy)
		return i, (cx - l.left()) / l.w, (cy - l.tops[i]) / l.heights[i]
	}
	i0, fx0, fy0 := where()
	tt.Pinch(v.X+x, v.Y+y, 3)
	tt.Advance(100 * time.Millisecond)
	i1, fx1, fy1 := where()
	if pv.zoom != 3 || i1 != i0 || abs(fx1-fx0) > 0.002 || abs(fy1-fy0) > 0.002 {
		t.Errorf("pinch: zoom %v, page %d at %v,%v, was %d at %v,%v", pv.zoom, i1, fx1, fy1, i0, fx0, fy0)
	}

	// Zoomed past what a page is drawn whole with, the part in view is
	// drawn sharp over it, and the whole page no larger.
	pp := &pv.pages[i0]
	settle("no part drawn", func() bool { return pp.part != nil && !pp.drawing })
	pg := pv.doc.Pages[i0]
	if max := int(math.Sqrt(pdfPixels * pg.Width / pg.Height)); pp.width > max {
		t.Errorf("whole page drawn %d wide, past %d", pp.width, max)
	}
	if want := int(pv.pdfLayout().w * 2); pp.partWidth != want {
		t.Errorf("part drawn at %d wide, wants %d", pp.partWidth, want)
	}
	if pp.partOf.W >= 1 || pp.partOf.H >= 1 {
		t.Errorf("part %+v is the whole page", pp.partOf)
	}
	save(t, tt, "preview-pdf-zoom")

	// Fitting it again lets the part go.
	tt.Key(ui.Cmd, ui.Key0)
	tt.Advance(100 * time.Millisecond)
	if pv.zoom != 1 || pv.pages[i0].part != nil {
		t.Errorf("Cmd0: zoom %v, part %v", pv.zoom, pv.pages[i0].part != nil)
	}

	// The wheel alone scrolls the pages, rather than zoom them.
	tt.Wheel(v.X+x, v.Y+y, 0, 200)
	tt.Advance(500 * time.Millisecond)
	if pv.zoom != 1 || pv.pdfScroll.Y == 0 {
		t.Errorf("wheel: zoom %v, scrolled %v", pv.zoom, pv.pdfScroll.Y)
	}
}

func abs(f float32) float32 {
	if f < 0 {
		return -f
	}
	return f
}

func TestPreviewImage(t *testing.T) {
	a, tt, _ := signedIn(t)
	a.gridView = true
	a.router.Push(filesRoute("/照片"))
	waitFor(t, tt, "IMG_2310.jpg")
	a.open(fileNamed(t, a, "/照片", "IMG_2312.jpg"))
	deadline := time.Now().Add(5 * time.Second)
	for !a.pv.hasImage() {
		if a.pv.err != "" || time.Now().After(deadline) {
			t.Fatalf("image did not load: %q", a.pv.err)
		}
		time.Sleep(10 * time.Millisecond)
		tt.Advance(50 * time.Millisecond)
	}
	tt.Advance(time.Second)
	save(t, tt, "preview-image")
	if !tt.HasText("图片 · 15 KB · 640×480 · 3 / 24") {
		t.Errorf("counter missing: %q", tt.Texts())
	}
	tt.Key(0, keyRight)
	tt.Advance(100 * time.Millisecond)
	if a.pv.file.Name != "IMG_2313.jpg" {
		t.Errorf("Right went to %s", a.pv.file.Name)
	}
	// A swipe between pages, or a mouse's side button.
	tt.Key(0, ui.KeyForward)
	tt.Advance(100 * time.Millisecond)
	if a.pv.file.Name != "IMG_2314.jpg" {
		t.Errorf("Forward went to %s", a.pv.file.Name)
	}
	tt.Key(0, ui.KeyBack)
	tt.Advance(100 * time.Millisecond)
	if a.pv.file.Name != "IMG_2313.jpg" || a.router.Path() != "/files/照片" {
		t.Errorf("Back went to %s in %s", a.pv.file.Name, a.router.Path())
	}
}

// clipboardPicture waits for a picture on the clipboard and decodes it.
func clipboardPicture(t *testing.T, tt *ui.Tester) image.Image {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if data, err := tt.ClipboardData().Read(transfer.PNG); err == nil {
			img, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			return img
		}
		if time.Now().After(deadline) {
			t.Fatal("no picture was copied")
		}
		time.Sleep(10 * time.Millisecond)
		tt.Advance(50 * time.Millisecond)
	}
}

func TestPreviewCopyImage(t *testing.T) {
	a, tt, _ := signedIn(t)
	a.router.Push(filesRoute("/照片"))
	waitFor(t, tt, "IMG_2310.jpg")
	a.open(fileNamed(t, a, "/照片", "IMG_2311.jpg"))
	deadline := time.Now().Add(5 * time.Second)
	for !a.pv.hasImage() {
		if a.pv.err != "" || time.Now().After(deadline) {
			t.Fatalf("image did not load: %q", a.pv.err)
		}
		time.Sleep(10 * time.Millisecond)
		tt.Advance(50 * time.Millisecond)
	}
	tt.Advance(500 * time.Millisecond)
	tt.Key(ui.Cmd, ui.KeyC)
	if b := clipboardPicture(t, tt).Bounds(); b.Dx() != 640 || b.Dy() != 480 {
		t.Errorf("copied %v", b)
	}
	tt.Key(ui.Cmd, ui.KeyEqual)
	if a.pv.zoom != 1.5 {
		t.Errorf("Cmd= zoomed to %v", a.pv.zoom)
	}
	tt.Key(ui.Cmd, ui.Key0)
	if a.pv.zoom != 1 {
		t.Errorf("Cmd0 left zoom %v", a.pv.zoom)
	}
	tt.Advance(300 * time.Millisecond)
	tt.RightClickAt(1180/2, 760/2)
	tt.Advance(100 * time.Millisecond)
	if err := tt.ChooseMenuItem("复制路径"); err != nil {
		t.Fatalf("%v: %q", err, tt.Menu())
	}
	tt.Advance(100 * time.Millisecond)
	if got := tt.Clipboard(); got != "/照片/IMG_2311.jpg" {
		t.Errorf("clipboard %q", got)
	}
}

func TestPreviewPinchAndPan(t *testing.T) {
	a, tt, _ := signedIn(t)
	a.router.Push(filesRoute("/照片"))
	waitFor(t, tt, "IMG_2310.jpg")
	a.open(fileNamed(t, a, "/照片", "IMG_2311.jpg"))
	deadline := time.Now().Add(5 * time.Second)
	for !a.pv.hasImage() {
		if a.pv.err != "" || time.Now().After(deadline) {
			t.Fatalf("image did not load: %q", a.pv.err)
		}
		time.Sleep(10 * time.Millisecond)
		tt.Advance(50 * time.Millisecond)
	}
	tt.Advance(500 * time.Millisecond)
	v := a.pv.view
	cx, cy := v.X+v.W/2, v.Y+v.H/2

	// Two fingers on the picture as it fits move nothing.
	tt.Scroll(cx, cy, 0, 40)
	if a.pv.zoom != 1 || a.pv.pan != (ui.Point{}) {
		t.Errorf("scrolling at fit: zoom %v, pan %v", a.pv.zoom, a.pv.pan)
	}
	// A pinch 100 DIPs right of the center keeps that point under it.
	tt.Pinch(cx+100, cy, 2)
	tt.Advance(100 * time.Millisecond)
	if a.pv.zoom != 2 || a.pv.pan != (ui.Point{X: -100}) {
		t.Errorf("pinch: zoom %v, pan %v", a.pv.zoom, a.pv.pan)
	}
	tt.Scroll(cx, cy, 30, 40)
	tt.Advance(100 * time.Millisecond)
	if a.pv.zoom != 2 || a.pv.pan != (ui.Point{X: -130, Y: -40}) {
		t.Errorf("pan: zoom %v, pan %v", a.pv.zoom, a.pv.pan)
	}
	tt.Pinch(cx, cy, 0.25)
	tt.Advance(100 * time.Millisecond)
	if a.pv.zoom != 1 || a.pv.pan != (ui.Point{}) {
		t.Errorf("pinch back: zoom %v, pan %v", a.pv.zoom, a.pv.pan)
	}
	// A mouse wheel's notches still zoom.
	tt.Wheel(cx, cy, 0, -100)
	tt.Advance(300 * time.Millisecond)
	if a.pv.zoom <= 1 {
		t.Errorf("wheel left zoom %v", a.pv.zoom)
	}
}

func TestCopyImageFromList(t *testing.T) {
	a, tt, _ := signedIn(t)
	a.router.Push(filesRoute("/照片/京都"))
	waitFor(t, tt, "kyoto-1.png")
	if err := tt.RightClick("kyoto-1.png"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(100 * time.Millisecond)
	if err := tt.ChooseMenuItem("复制图片"); err != nil {
		t.Fatalf("%v: %q", err, tt.Menu())
	}
	if b := clipboardPicture(t, tt).Bounds(); b.Dx() != 800 || b.Dy() != 600 {
		t.Errorf("copied %v", b)
	}
	if !tt.HasText("已复制图片") {
		t.Error("no toast")
	}
}

func save(t *testing.T, tt interface {
	Image() *image.RGBA
}, name string) {
	dir := os.Getenv("BAIDUNETGO_SNAPSHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(dir + "/" + name + ".png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, tt.Image())
}

func TestPreviewAudioStreams(t *testing.T) {
	a, tt, _ := signedIn(t)
	a.router.Push(filesRoute("/音乐"))
	waitFor(t, tt, "和弦.wav")
	a.open(fileNamed(t, a, "/音乐", "和弦.wav"))
	deadline := time.Now().Add(5 * time.Second)
	for a.pv.player == nil {
		if a.pv.err != "" || time.Now().After(deadline) {
			t.Fatalf("audio did not open: %q", a.pv.err)
		}
		time.Sleep(10 * time.Millisecond)
		tt.Advance(50 * time.Millisecond)
	}
	if d := a.pv.player.Duration(); d < 5*time.Second || d > 7*time.Second {
		t.Errorf("duration %v", d)
	}
	tt.Advance(500 * time.Millisecond)
	save(t, tt, "preview-audio")
	a.closePreview()
}
