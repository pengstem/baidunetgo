package app

import (
	"strings"
	"testing"
	"time"

	"github.com/pengstem/baidunetgo/internal/baidu"
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

	// Save someone's link.
	tt.Click("转存链接")
	tt.Advance(400 * time.Millisecond)
	tt.Type("链接: https://pan.baidu.com/s/1demo 提取码: abcd")
	tt.Advance(400 * time.Millisecond)
	save(t, tt, "dialog-savelink")
	if err := tt.Click("保存到「我的文件」"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !srv.Exists("/分享的旅行照片/shared-2.png") {
		t.Errorf("not saved: %q", a.dlg.err)
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
