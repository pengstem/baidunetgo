package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestInfo opens a file's details from its context menu, and a folder's,
// which counts what the folder holds.
func TestInfo(t *testing.T) {
	a, tt, _ := signedIn(t)
	if err := tt.RightClick("照片"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("显示详情"); err != nil {
		t.Fatalf("%v; menu %q", err, tt.Menu())
	}
	waitFor(t, tt, "包含")
	waitFor(t, tt, "个文件共")
	if !tt.HasText("我的文件") || a.dlg.kind != dlgInfo {
		t.Errorf("texts %q", tt.Texts())
	}
	save(t, tt, "info-folder")
	if err := tt.Click("完成"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)

	l := a.listings["/"]
	var name string
	for _, f := range l.files {
		if !f.IsDir {
			name = f.Name
			break
		}
	}
	tt.Click(name)
	tt.Key(ui.Cmd, ui.KeyI)
	waitFor(t, tt, "修改时间")
	if !tt.HasText("字节") && !tt.HasText("字节）") {
		t.Errorf("no size in bytes; texts %q", tt.Texts())
	}
}
