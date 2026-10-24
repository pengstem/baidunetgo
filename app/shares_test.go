package app

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/fakepan"
)

func TestShareFlow(t *testing.T) {
	a, tt, srv := signedIn(t)
	a.share([]baidu.File{fileNamed(t, a, "/", "README.md")})
	tt.Advance(400 * time.Millisecond)
	save(t, tt, "dialog-share")
	if err := tt.Click("创建链接"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !a.shareForm.created {
		t.Fatalf("not created: %q", a.dlg.err)
	}
	save(t, tt, "dialog-share-made")
	tt.Click("复制链接和提取码")
	tt.Advance(100 * time.Millisecond)
	if clip := tt.Clipboard(); !strings.Contains(clip, "提取码: "+a.shareForm.pwd) {
		t.Errorf("clipboard %q", clip)
	}
	a.router.Push("/shares")
	waitFor(t, tt, "README.md")
	tt.Advance(500 * time.Millisecond)
	save(t, tt, "shares")

	// Save someone's link, typed in.
	tt.Click("转存链接")
	tt.Advance(400 * time.Millisecond)
	tt.Type("链接: https://pan.baidu.com/s/1demo 提取码: abcd")
	waitFor(t, tt, "旅行的猫 的分享")
	tt.Advance(600 * time.Millisecond)
	save(t, tt, "dialog-savelink")
	if err := tt.Click("全部保存"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !srv.Exists("/分享的旅行照片/shared-2.png") || !srv.Exists("/旅行清单.txt") {
		t.Errorf("not saved: %q", a.dlg.err)
	}
}

func TestShareFromClipboard(t *testing.T) {
	a, tt, srv := signedIn(t)
	srv.Add(fakepan.Node{Path: "/收到", IsDir: true})
	a.settings.ShareDest = "/收到"

	// A link without its password: the dialog asks for it.
	tt.SetClipboard("快来看 https://pan.baidu.com/s/1demo")
	a.clipCheck = true
	waitFor(t, tt, "这个分享需要提取码")
	tt.Advance(400 * time.Millisecond)
	save(t, tt, "share-clip-password")
	tt.Type("abcz")
	waitFor(t, tt, "提取码不对，再试一次")
	for range 4 {
		tt.Key(0, ui.KeyBackspace)
	}
	tt.Type("abcd")
	waitFor(t, tt, "旅行的猫 的分享")
	if !tt.HasText("来自剪贴板") || !tt.HasText("永久有效") {
		t.Errorf("header: %q", tt.Texts())
	}
	tt.Click("取消")
	settle(tt)

	// The same link again is not opened again; one with its password is.
	a.clipCheck = true
	settle(tt)
	if a.dlg.open {
		t.Fatal("opened a link seen already")
	}
	tt.SetClipboard("链接: https://pan.baidu.com/s/1demo?pwd=abcd 提取码: abcd 复制这段内容后打开百度网盘")
	a.clipCheck = true
	waitFor(t, tt, "旅行的猫 的分享")
	waitFor(t, tt, "旅行清单.txt")
	tt.Advance(800 * time.Millisecond)
	save(t, tt, "share-clip")

	// Into the folder, choose two pictures, save them.
	if err := tt.Click("分享的旅行照片"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, tt, "海边日落.mp4")
	tt.Click("选择「shared-1.png」")
	tt.Click("shared-3.png")
	tt.Advance(800 * time.Millisecond)
	save(t, tt, "share-clip-folder")
	if err := tt.Click("保存所选 2 项"); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	settle(tt)
	if !srv.Exists("/收到/shared-1.png") || !srv.Exists("/收到/shared-3.png") || srv.Exists("/收到/shared-2.png") {
		t.Errorf("saved the wrong files: %q", a.dlg.err)
	}
	if a.dlg.open {
		t.Error("dialog still open")
	}

	// The app's own copies are not opened.
	a.copy("链接: https://pan.baidu.com/s/1fake9?pwd=wxyz 提取码: wxyz", "")
	settle(tt)
	a.clipCheck = true
	settle(tt)
	if a.dlg.open {
		t.Error("opened a link the app copied")
	}
}

func TestOfflineFlow(t *testing.T) {
	a, tt, srv := signedIn(t)
	a.router.Push("/offline")
	waitFor(t, tt, "没有离线下载")
	tt.Click("新建任务")
	tt.Advance(400 * time.Millisecond)
	tt.Type("https://example.com/debian.iso")
	save(t, tt, "dialog-offline")
	tt.Click("开始下载")
	settle(tt)
	waitFor(t, tt, "debian.iso")
	save(t, tt, "offline")
	deadline := time.Now().Add(8 * time.Second)
	for !srv.Exists("/debian.iso") {
		if time.Now().After(deadline) {
			t.Fatal("offline task never finished")
		}
		time.Sleep(50 * time.Millisecond)
		tt.Advance(500 * time.Millisecond)
	}
}
