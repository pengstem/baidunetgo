//go:build live

package baidu_test

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/pcsimport"
)

// BAIDUNETGO_SHARE='https://pan.baidu.com/s/1…?pwd=abcd' go test -tags live ./internal/baidu -run LiveShare -v
// Opens someone's share, reads a folder of it, and saves its first item
// into a folder it deletes afterwards.
func TestLiveShare(t *testing.T) {
	text := os.Getenv("BAIDUNETGO_SHARE")
	acc, err := pcsimport.Find()
	if text == "" || err != nil {
		t.Skip("needs BAIDUNETGO_SHARE and BaiduPCS-Go's login")
	}
	link, pwd, ok := baidu.ParseShareLink(text)
	if !ok {
		t.Fatalf("not a share link: %q", text)
	}
	c := baidu.Open(acc.Creds)
	ctx := context.Background()
	if _, err := c.OpenShare(ctx, link, "zz99"); !errors.Is(err, baidu.ErrSharePassword) {
		t.Errorf("wrong password: %v", err)
	}
	if _, err := c.OpenShare(ctx, "https://pan.baidu.com/s/1AAAAAAAAAAAAAAAAAAAAAA", "abcd"); !errors.Is(err, baidu.ErrShareGone) {
		t.Errorf("no such link: %v", err)
	}
	s, err := c.OpenShare(ctx, link, pwd)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s by %q (mine %v, photo %v), made %s, expires %d, %d at the top",
		s.Link, s.Owner, s.Mine, s.Avatar != "", time.Unix(s.Created, 0).Format(time.DateOnly), s.Expires, len(s.Files))
	if len(s.Files) == 0 || s.Owner == "" {
		t.Fatal("nothing shared, or no owner")
	}
	if u, err := c.User(ctx); err == nil && !s.Mine && s.Avatar == u.Avatar {
		t.Errorf("the sharer's photo is the account's own: %s", s.Avatar)
	}
	t.Logf("photo %s", s.Avatar)
	for _, f := range s.Files {
		t.Logf("  %s dir=%v size=%d", f.Name, f.IsDir, f.Size)
		if f.IsDir {
			in, err := c.SharedDir(ctx, s, f.Path)
			if err != nil {
				t.Fatal(err)
			}
			for _, g := range in {
				t.Logf("    %s dir=%v size=%d thumb=%v", g.Name, g.IsDir, g.Size, g.Thumb != "")
			}
		}
	}
	box := "/BaiduNetGo自检-share-" + strconv.FormatInt(time.Now().Unix(), 10)
	if err := c.Mkdir(ctx, box); err != nil {
		t.Fatal(err)
	}
	defer c.Delete(ctx, []string{box})
	n, err := c.SaveShared(ctx, s, []int64{s.Files[0].FsID}, box)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.List(ctx, box, baidu.Order{})
	if err != nil || len(got) != 1 || got[0].Name != s.Files[0].Name {
		t.Fatalf("saved %d, found %+v %v", n, got, err)
	}
	if _, err := c.SaveShared(ctx, s, []int64{s.Files[0].FsID}, box); err != nil {
		t.Logf("saved again: %v", err)
	}
}
