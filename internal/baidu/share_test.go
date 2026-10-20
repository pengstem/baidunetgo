package baidu_test

import (
	"context"
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
	if n, err := c.SaveShare(ctx, "https://pan.baidu.com/s/1demo", "abcd", "/"); err != nil || n != 1 || !srv.Exists("/分享的旅行照片/shared-1.png") {
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
