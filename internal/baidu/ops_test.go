package baidu_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/fakepan"
)

func TestOps(t *testing.T) {
	srv := fakepan.New()
	defer srv.Close()
	c := baidu.OpenAt(baidu.Credentials{BDUSS: "x", UID: 1}, srv.URL)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(c.Mkdir(ctx, "/新的"))
	must(c.Rename(ctx, "/文档/读我.txt", "说明.txt"))
	must(c.Move(ctx, []string{"/文档/说明.txt", "/代码"}, "/新的"))
	must(c.Copy(ctx, []string{"/新的/说明.txt"}, "/"))
	for _, p := range []string{"/新的/说明.txt", "/新的/代码/main.go", "/说明.txt"} {
		if !srv.Exists(p) {
			t.Errorf("%s missing", p)
		}
	}
	must(c.Delete(ctx, []string{"/新的"}))
	if srv.Exists("/新的/代码/main.go") {
		t.Error("delete left files")
	}
	bin, err := c.RecycleList(ctx)
	must(err)
	if len(bin) != 1 || bin[0].Name != "新的" || !bin[0].IsDir {
		t.Fatalf("recycle %+v", bin)
	}
	must(c.Restore(ctx, []int64{bin[0].FsID}))
	if !srv.Exists("/新的/代码/main.go") {
		t.Error("restore lost files")
	}
	if err := c.Mkdir(ctx, "/新的"); err == nil {
		t.Error("mkdir over a name taken succeeded")
	}
}

// TestSearchPages reads every page of the results, up to a bound.
func TestSearchPages(t *testing.T) {
	srv := fakepan.New()
	defer srv.Close()
	for i := range 1234 {
		srv.Add(fakepan.Node{Path: fmt.Sprintf("/批量/文件-%04d.txt", i), Data: []byte("x")})
	}
	c := baidu.OpenAt(baidu.Credentials{BDUSS: "x", UID: 1}, srv.URL)
	got, more, err := c.Search(context.Background(), "/", "文件-")
	if err != nil || len(got) != 1234 || more {
		t.Fatalf("%d results, more %v, %v", len(got), more, err)
	}
	for i := range 2500 {
		srv.Add(fakepan.Node{Path: fmt.Sprintf("/更多/文件-%04d.bin", i), Data: []byte("x")})
	}
	got, more, _ = c.Search(context.Background(), "/", "文件-")
	if len(got) != 3000 || !more {
		t.Errorf("%d results, more %v", len(got), more)
	}
}
