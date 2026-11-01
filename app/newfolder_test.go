package app

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/fakepan"
)

func TestFolderDraft(t *testing.T) {
	for _, grid := range []bool{false, true} {
		t.Run(map[bool]string{false: "list", true: "grid"}[grid], func(t *testing.T) {
			a, tt, srv := signedIn(t)
			a.gridView = grid
			tt.Advance(time.Second)
			if err := tt.Click("新建文件夹"); err != nil {
				t.Fatal(err)
			}
			tt.Advance(time.Second)
			l := a.listings["/"]
			if a.creating == nil || !tt.Focused("新文件夹名称") {
				t.Fatal("draft did not take focus")
			}
			tt.Type("相册")
			a.order.Desc = true
			a.reload("/")
			settle(tt)
			if l.files[0].FsID != draftFolderID || a.creating.name != "相册" || srv.Exists("/相册") {
				t.Fatal("refresh or sorting moved, changed or created the draft")
			}
			save(t, tt, "new-folder-"+map[bool]string{false: "list", true: "grid"}[grid])
			tt.SetSize(780, 540)
			tt.SetDark(true)
			tt.Advance(time.Second)
			for _, label := range []string{"新文件夹名称", "创建文件夹", "取消新建"} {
				if r, ok := tt.Find(label); !ok || r.W <= 0 || r.X < sidebarWidth || r.X+r.W > 780 || r.Y+r.H > 540 {
					t.Fatalf("%s outside narrow window: %v", label, r)
				}
			}
			save(t, tt, "new-folder-dark-narrow-"+map[bool]string{false: "list", true: "grid"}[grid])
			tt.Key(0, ui.KeyEscape)
			tt.Advance(time.Second)
			if a.creating != nil || l.index(draftFolderID) >= 0 || srv.Exists("/相册") {
				t.Fatal("Escape left a draft or created a folder")
			}

			a.newFolder("/")
			tt.Advance(time.Second)
			tt.Type("..")
			tt.Key(0, ui.KeyEnter)
			if a.creating == nil || a.creating.err == "" {
				t.Fatal("dot-dot name accepted")
			}
			tt.Key(ui.Cmd, ui.KeyA)
			tt.Type("照片")
			tt.Key(0, ui.KeyEnter)
			if a.creating == nil || !strings.Contains(a.creating.err, "已有") {
				t.Fatal("duplicate name accepted")
			}

			// Another client created this after our listing was read.
			srv.Add(fakepan.Node{Path: "/同时创建", IsDir: true})
			tt.Key(ui.Cmd, ui.KeyA)
			tt.Type("同时创建")
			tt.Key(0, ui.KeyEnter)
			waitUntil(t, tt, "server failure", func() bool { return a.creating != nil && !a.creating.busy && a.creating.err != "" })
			if a.creating.name != "同时创建" || !tt.Focused("新文件夹名称") {
				t.Fatal("failure lost the name or focus")
			}
			save(t, tt, "new-folder-error-"+map[bool]string{false: "list", true: "grid"}[grid])
			tt.Key(ui.Cmd, ui.KeyA)
			tt.Type("重新创建")
			tt.Key(0, ui.KeyEnter)
			waitUntil(t, tt, "retry", func() bool { return a.creating == nil && srv.Exists("/重新创建") })
			settle(tt)
			if l.index(draftFolderID) >= 0 || !tt.HasText("重新创建") {
				t.Fatal("success did not replace the draft")
			}
		})
	}
}

func TestEmptyFolderActions(t *testing.T) {
	a, tt, srv := signedIn(t)
	a.router.Push(filesRoute("/空文件夹"))
	waitFor(t, tt, "上传文件")
	tt.Advance(time.Second)
	save(t, tt, "empty-folder-actions")
	if !tt.HasText("这里还空着") {
		t.Fatal("empty state missing")
	}
	if err := tt.Click("在此新建文件夹"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Type("未确认")
	a.router.Push(filesRoute("/"))
	settle(tt)
	a.router.Back()
	settle(tt)
	if a.creating != nil || srv.Exists("/空文件夹/未确认") || a.listings["/空文件夹"].index(draftFolderID) >= 0 {
		t.Fatal("navigation kept a draft or created it")
	}
}
