package app

import (
	"context"
	"strings"
	"testing"
	"time"

	_ "github.com/egoist/mygo/locales/zh-Hans"
	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/fakepan"
)

// signedIn returns an app signed in to a fake netdisk, and its tester.
func signedIn(t *testing.T) (*App, *ui.Tester, *fakepan.Server) {
	t.Helper()
	srv := fakepan.New()
	t.Cleanup(srv.Close)
	a := New()
	a.fake = true
	go a.signInWith(baidu.OpenAt(baidu.Credentials{BDUSS: "fake", UID: 1}, srv.URL), false)
	tt := ui.NewTester(a.View, 1180, 760)
	tt.SetLocale("zh-Hans")
	waitFor(t, tt, "照片")
	return a, tt, srv
}

// waitFor builds frames until the text shows, the netdisk answering on
// goroutines.
func waitFor(t *testing.T, tt *ui.Tester, text string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !tt.HasText(text) {
		if time.Now().After(deadline) {
			t.Fatalf("%q never showed; texts %q", text, tt.Texts())
		}
		time.Sleep(10 * time.Millisecond)
		tt.Advance(50 * time.Millisecond)
	}
}

func TestBrowse(t *testing.T) {
	a, tt, _ := signedIn(t)
	if !tt.HasText("演示用户") {
		t.Errorf("profile missing; texts %q", tt.Texts())
	}
	// Folders come first, sorted by pinyin: 代码 (dai) before 照片 (zhao).
	files := a.listings["/"].files
	if !files[0].IsDir || files[0].Name != "代码" || files[len(files)-1].IsDir {
		t.Errorf("order %v", names(files))
	}
	r, _ := tt.Find("照片")
	tt.ClickAt(r.X+5, r.Y+5)
	tt.ClickAt(r.X+5, r.Y+5) // a double click opens it
	waitFor(t, tt, "IMG_2310.jpg")
	if a.currentDir() != "/照片" {
		t.Errorf("dir %q", a.currentDir())
	}
	if err := tt.Click("我的文件"); err != nil { // the breadcrumb
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.currentDir() != "/" {
		t.Errorf("after breadcrumb, dir %q", a.currentDir())
	}
}

func TestSortByHeader(t *testing.T) {
	a, tt, _ := signedIn(t)
	if err := tt.Click("按大小排序"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(100 * time.Millisecond)
	if a.order.By != "size" || !a.order.Desc {
		t.Errorf("order %+v", a.order)
	}
	files := a.listings["/"].files
	var last int64 = 1 << 62
	for _, f := range files {
		if f.IsDir {
			continue
		}
		if f.Size > last {
			t.Errorf("not by size desc: %v", names(files))
		}
		last = f.Size
	}
}

func TestRoutes(t *testing.T) {
	for _, dir := range []string{"/", "/照片", "/a b/50% off?#", "/文档/旅行 计划"} {
		got := filesRoute(dir)
		param := ""
		if got != "/files" {
			param = got[len("/files/"):]
		}
		if back := dirOfRoute(param); back != dir {
			t.Errorf("%q -> %q -> %q", dir, got, back)
		}
	}
}

func names(files []baidu.File) []string {
	var n []string
	for _, f := range files {
		n = append(n, f.Name)
	}
	return n
}

func TestSettingsPage(t *testing.T) {
	a, tt, _ := signedIn(t)
	a.router.Push("/settings")
	waitFor(t, tt, "下载位置")
	tt.Advance(time.Second)
	save(t, tt, "settings")
	tt.SetDark(true)
	tt.Advance(500 * time.Millisecond)
	save(t, tt, "settings-dark")
}

func TestTuneFromSettings(t *testing.T) {
	old := tunePer
	tunePer = 100 * time.Millisecond
	t.Cleanup(func() { tunePer = old })
	a, tt, _ := signedIn(t)
	a.router.Push("/settings")
	waitFor(t, tt, "重测")
	tt.Click("重测")
	waitFor(t, tt, "已根据测速优化")
	for _, want := range []string{"下载线路", "每个文件的连接数", "请求块大小", "同时传输的文件数", "每个上传的连接数"} {
		if !tt.HasText(want) {
			t.Errorf("%q missing; texts %q", want, tt.Texts())
		}
	}
	cfg := a.transferConfig()
	if len(cfg.Hosts) != 2 || a.xfer.Block != cfg.Block || a.xfer.Upload != cfg.Upload {
		t.Errorf("config %+v not in force (block %d, upload %d)", cfg, a.xfer.Block, a.xfer.Upload)
	}
	if first, _ := a.sess.client.DownloadURL(context.Background(), "/安装包.dmg"); baidu.Host(first) != cfg.Hosts[0] {
		t.Errorf("link %s not on the fastest host %s", first, cfg.Hosts[0])
	}
	tt.Advance(time.Second)
	save(t, tt, "settings-tune")
}

// TestGridLongNames keeps a name with no spaces, as release names are,
// inside its tile instead of running over the tiles beside it.
func TestGridLongNames(t *testing.T) {
	a, tt, srv := signedIn(t)
	names := []string{
		"Andor.S01E02.ITA.ENG.2160p.UHD.BluRay.DDP.MeM.GP.mkv",
		"Andor.S01E03.Reckoning.ITA.ENG.2160p.UHD.BluRay.DDP.MeM.GP.mkv",
		"Andor.S01E04.Aldhani.ITA.ENG.2160p.UHD.BluRay.DDP.MeM.GP.mkv",
	}
	for _, n := range names {
		srv.Add(fakepan.Node{Path: "/剧集/" + n, Category: 1, Size: 8 << 30})
	}
	a.gridView = true
	a.router.Push(filesRoute("/剧集"))
	waitFor(t, tt, names[0])
	tt.Advance(time.Second)
	save(t, tt, "grid-long-names")
	var prev ui.Rect
	for i, n := range names {
		r, ok := tt.Find(a.tileNames[n])
		if !ok {
			t.Fatalf("%s not shown; texts %q", n, tt.Texts())
		}
		if r.W > tileNameWidth {
			t.Errorf("%s is %.0f wide, wider than its tile", n, r.W)
		}
		if s := a.tileNames[n]; !strings.HasSuffix(s, "GP.mkv") || strings.Count(s, "\n") != 1 {
			t.Errorf("%s laid out as %q, not two lines ending in its extension", n, s)
		}
		if i > 0 && r.X < prev.X+prev.W {
			t.Errorf("%s overlaps the name before it", n)
		}
		prev = r
	}
}
