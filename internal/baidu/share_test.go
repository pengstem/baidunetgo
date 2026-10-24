package baidu_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/fakepan"
)

func TestParseShareLink(t *testing.T) {
	for _, tc := range []struct{ in, link, pwd string }{
		{"https://pan.baidu.com/s/1AbC-d_9?pwd=x7k2", "https://pan.baidu.com/s/1AbC-d_9", "x7k2"},
		{"通过网盘分享的文件：照片\n链接: https://pan.baidu.com/s/1xyz 提取码: ab12 \n--来自百度网盘", "https://pan.baidu.com/s/1xyz", "ab12"},
		{"https://pan.baidu.com/share/init?surl=Qwe 密码：9z9z", "https://pan.baidu.com/s/1Qwe", "9z9z"},
	} {
		link, pwd, ok := baidu.ParseShareLink(tc.in)
		if !ok || link != tc.link || pwd != tc.pwd {
			t.Errorf("%q: %q %q %v", tc.in, link, pwd, ok)
		}
	}
	if _, _, ok := baidu.ParseShareLink("hello"); ok {
		t.Error("parsed no link")
	}
}

func TestSharesAndOffline(t *testing.T) {
	srv := fakepan.New()
	defer srv.Close()
	c := baidu.OpenAt(baidu.Credentials{BDUSS: "x", UID: 1}, srv.URL)
	ctx := context.Background()
	link, err := c.CreateShare(ctx, []string{"/照片"}, "wxyz", 7)
	if err != nil || link == "" {
		t.Fatal(link, err)
	}
	shares, err := c.Shares(ctx)
	if err != nil || len(shares) != 1 || shares[0].Path != "/照片" || shares[0].Expired(time.Now()) {
		t.Fatalf("%+v %v", shares, err)
	}
	if pwd, err := c.SharePassword(ctx, shares[0].ID); err != nil || pwd != "wxyz" {
		t.Errorf("pwd %q %v", pwd, err)
	}
	if err := c.CancelShares(ctx, []int64{shares[0].ID}); err != nil {
		t.Fatal(err)
	}
	if shares, _ = c.Shares(ctx); len(shares) != 0 {
		t.Error("share not cancelled")
	}
	if _, err := c.SaveShare(ctx, "https://pan.baidu.com/s/1demo", "wrong", "/"); err == nil {
		t.Error("wrong password saved")
	}
	if n, err := c.SaveShare(ctx, "https://pan.baidu.com/s/1demo", "abcd", "/"); err != nil || n != 2 || !srv.Exists("/分享的旅行照片/shared-1.png") || !srv.Exists("/旅行清单.txt") {
		t.Errorf("save: %d %v", n, err)
	}

	if err := c.AddOffline(ctx, "not a link", "/"); err == nil {
		t.Error("bad offline link accepted")
	}
	if err := c.AddOffline(ctx, "https://example.com/ubuntu.iso", "/下载"); err != nil {
		t.Fatal(err)
	}
	tasks, err := c.Offlines(ctx)
	if err != nil || len(tasks) != 1 || tasks[0].Status != 1 || tasks[0].Name != "ubuntu.iso" {
		t.Fatalf("%+v %v", tasks, err)
	}
}

func TestDownloadURLsFollowHosts(t *testing.T) {
	srv := fakepan.New()
	defer srv.Close()
	c := baidu.OpenAt(baidu.Credentials{BDUSS: "x", UID: 1}, srv.URL)
	links, err := c.DownloadURLs(context.Background(), "/a.bin")
	if err != nil || len(links) != 2 {
		t.Fatalf("links %v, %v", links, err)
	}
	second := baidu.Host(links[1])
	c.SetHosts([]string{second})
	if first, _ := c.DownloadURL(context.Background(), "/a.bin"); baidu.Host(first) != second {
		t.Errorf("first %s, want the ranked %s", first, second)
	}
}

func TestOpenShare(t *testing.T) {
	srv := fakepan.New()
	defer srv.Close()
	c := baidu.OpenAt(baidu.Credentials{BDUSS: "x", UID: 1}, srv.URL)
	ctx := context.Background()
	if _, err := c.OpenShare(ctx, fakepan.DemoShare, ""); !errors.Is(err, baidu.ErrSharePassword) {
		t.Errorf("no password: %v", err)
	}
	if _, err := c.OpenShare(ctx, fakepan.DemoShare, "zzzz"); !errors.Is(err, baidu.ErrSharePassword) {
		t.Errorf("wrong password: %v", err)
	}
	if _, err := c.OpenShare(ctx, "https://pan.baidu.com/s/1nothing", "abcd"); !errors.Is(err, baidu.ErrShareGone) {
		t.Errorf("no such share: %v", err)
	}
	s, err := c.OpenShare(ctx, fakepan.DemoShare, fakepan.DemoSharePwd)
	if err != nil {
		t.Fatal(err)
	}
	if s.Owner != "旅行的猫" || s.Mine || !strings.HasSuffix(s.Avatar, "/thumb?id=11") || s.Expires != 0 || len(s.Files) != 2 || !s.Files[0].IsDir || s.Files[1].Name != "旅行清单.txt" {
		t.Fatalf("%+v", s)
	}
	in, err := c.SharedDir(ctx, s, s.Files[0].Path)
	if err != nil || len(in) != 4 || in[0].Thumb == "" {
		t.Fatalf("%+v %v", in, err)
	}
	// One picture of the folder, alone, twice: the second gets a number.
	srv.Add(fakepan.Node{Path: "/收到", IsDir: true})
	for range 2 {
		if n, err := c.SaveShared(ctx, s, []int64{in[1].FsID}, "/收到"); err != nil || n != 1 {
			t.Fatal(n, err)
		}
	}
	if !srv.Exists("/收到/"+in[1].Name) || !srv.Exists("/收到/shared-2(1).png") || srv.Exists("/收到/旅行清单.txt") {
		t.Error("saved the wrong files")
	}

	// The account's own share opens too, and says so.
	link, _ := c.CreateShare(ctx, []string{"/文档"}, "wxyz", 7)
	mine, err := c.OpenShare(ctx, link, "wxyz")
	if err != nil || !mine.Mine || mine.Expires == 0 || len(mine.Files) != 1 || mine.Files[0].Name != "文档" {
		t.Fatalf("%+v %v", mine, err)
	}
}
