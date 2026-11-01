package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/fakepan"
)

func dragToFolder(t *testing.T, tt *ui.Tester, name, folder string, grid bool, over func()) {
	t.Helper()
	from, ok := tt.Find(name)
	if !ok {
		t.Fatalf("no source %q", name)
	}
	to, ok := tt.Find(folder)
	if !ok {
		t.Fatalf("no destination %q", folder)
	}
	handle, ok := tt.Find("拖动 " + name)
	if !grid && !ok {
		t.Fatalf("no drag handle %q", name)
	}
	x, y := handle.X+handle.W/2, handle.Y+handle.H/2
	if grid {
		x, y = from.X+from.W/2, from.Y+40
	}
	tt.Press(x, y)
	tt.Move(x+8, y)
	tt.Move(to.X+to.W/2, to.Y+to.H/2)
	if over != nil {
		over()
	}
	tt.Release(to.X+to.W/2, to.Y+to.H/2)
}

func TestDropOnFolder(t *testing.T) {
	for _, grid := range []bool{false, true} {
		t.Run(map[bool]string{false: "list", true: "grid"}[grid], func(t *testing.T) {
			a, tt, srv := signedIn(t)
			a.gridView = grid
			tt.Advance(time.Second)
			local := filepath.Join(t.TempDir(), "拖入子目录.txt")
			if err := os.WriteFile(local, []byte("uploaded"), 0o600); err != nil {
				t.Fatal(err)
			}
			r, _ := tt.Find("空文件夹")
			if !tt.DropFiles(r.X+r.W/2, r.Y+r.H/2, local) {
				t.Fatal("folder rejected local file")
			}
			waitUntil(t, tt, "upload into target", func() bool { return string(srv.Data("/空文件夹/拖入子目录.txt")) == "uploaded" })
			if srv.Exists("/拖入子目录.txt") {
				t.Fatal("page also uploaded to its own directory")
			}

			l := a.listings["/"]
			for _, n := range []string{"README.md", "安装包.dmg"} {
				l.sel.Add(fileNamed(t, a, "/", n).FsID)
			}
			tt.Advance(time.Second)
			dragToFolder(t, tt, "README.md", "空文件夹", grid, func() {
				if a.dropHint != "松开以移动2 项到「空文件夹」" {
					t.Fatalf("missing destination: %q", a.dropHint)
				}
				save(t, tt, "drop-folder-"+map[bool]string{false: "list", true: "grid"}[grid])
			})
			waitUntil(t, tt, "move selected files", func() bool { return srv.Exists("/空文件夹/README.md") && srv.Exists("/空文件夹/安装包.dmg") })
			if srv.Exists("/README.md") || srv.Exists("/安装包.dmg") {
				t.Fatal("move kept source files")
			}
		})
	}
}

func TestDropMoveConflictAndCancel(t *testing.T) {
	a, tt, srv := signedIn(t)
	srv.Add(fakepan.Node{Path: "/空文件夹/README.md", Data: []byte("original")})
	tt.Advance(time.Second)
	dragToFolder(t, tt, "README.md", "空文件夹", false, func() { tt.Key(0, ui.KeyEscape) })
	settle(tt)
	if !srv.Exists("/README.md") || a.dlg.open {
		t.Fatal("Escape did not cancel drag")
	}
	dragToFolder(t, tt, "README.md", "空文件夹", false, nil)
	waitUntil(t, tt, "move conflict", func() bool { return a.dlg.open && a.dlg.kind == dlgConflict })
	if !srv.Exists("/README.md") || string(srv.Data("/空文件夹/README.md")) != "original" {
		t.Fatal("drop overwrote a conflicting name before resolution")
	}
	if err := tt.Click("保留两者"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, tt, "keep both", func() bool { return srv.Exists("/空文件夹/README (1).md") && !srv.Exists("/README.md") })
}

func TestInvalidFileDrop(t *testing.T) {
	d := fileDrag{from: "/", files: []baidu.File{{FsID: 1, Name: "照片", Path: "/照片", IsDir: true}}}
	for _, to := range []string{"/", "/照片", "/照片/京都"} {
		if validFileDrop(d, to) {
			t.Errorf("accepted move to %q", to)
		}
	}
	if !validFileDrop(d, "/照片备份") {
		t.Fatal("a sibling sharing a prefix was rejected")
	}
}
