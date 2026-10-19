package selftest

import (
	"context"
	"testing"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/fakepan"
)

func TestAgainstFake(t *testing.T) {
	srv := fakepan.New()
	defer srv.Close()
	rep := Run(context.Background(), baidu.OpenAt(baidu.Credentials{BDUSS: "x", UID: 1}, srv.URL), false, t.Logf)
	for _, c := range rep.Checks {
		// The fake knows one share of its own, not those it makes.
		// Nor can it play its videos, which are zeros.
		if !c.OK && c.Name != "转存分享链接" && c.Name != "mpv 播放大文件（经本地串流）" {
			t.Errorf("%s: %s", c.Name, c.Detail)
		}
	}
}
