//go:build live

package baidu_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/pcsimport"
	"github.com/pengstem/baidunetgo/internal/player"
	"github.com/pengstem/baidunetgo/internal/tune"
)

// go test -tags live ./internal/baidu -run LiveVIP -v
// Prints only membership-related fields of a few endpoints.
func TestLiveVIP(t *testing.T) {
	acc, err := pcsimport.Find()
	if err != nil {
		t.Skip(err)
	}
	c := baidu.Open(acc.Creds)
	for _, u := range []string{
		"https://pan.baidu.com/rest/2.0/membership/user?method=query&clienttype=0&app_id=250528&web=1",
		"https://pan.baidu.com/rest/2.0/xpan/nas?method=uinfo&clienttype=0&web=1",
		"https://pan.baidu.com/api/user/getinfo?need_selfinfo=1&clienttype=0&web=1",
	} {
		req, _ := http.NewRequestWithContext(context.Background(), "GET", u, nil)
		req.Header.Set("User-Agent", baidu.UserAgent)
		resp, err := c.HTTP().Do(req)
		if err != nil {
			t.Log(u, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var m any
		json.Unmarshal(body, &m)
		t.Logf("%s\n%s", u[:60], strings.Join(pick(m, ""), "\n"))
	}
}

func pick(v any, prefix string) []string {
	var out []string
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			p := prefix + "." + k
			lk := strings.ToLower(k)
			switch val.(type) {
			case map[string]any, []any:
				out = append(out, pick(val, p)...)
			default:
				if strings.Contains(lk, "vip") || strings.Contains(lk, "level") || strings.Contains(lk, "errno") || strings.Contains(lk, "end_time") || strings.Contains(lk, "product") || lk == "cluster" || lk == "status" || lk == "detail_cluster" {
					b, _ := json.Marshal(val)
					out = append(out, p+" = "+string(b))
				}
			}
		}
	case []any:
		for i, val := range x {
			if i > 5 {
				break
			}
			out = append(out, pick(val, prefix+"["+string(rune('0'+i))+"]")...)
		}
	}
	return out
}

func TestLiveUser(t *testing.T) {
	acc, err := pcsimport.Find()
	if err != nil {
		t.Skip(err)
	}
	u, err := baidu.Open(acc.Creds).User(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("VIP=%d label=%s until=%s avatar=%v", u.VIP, u.VIPLabel(), u.VIPUntil.Format("2006-01-02"), u.Avatar != "")
}

func TestLiveRange(t *testing.T) {
	acc, err := pcsimport.Find()
	if err != nil {
		t.Skip(err)
	}
	c := baidu.Open(acc.Creds)
	ctx := context.Background()
	f, err := tune.FindFile(ctx, c, 10<<20)
	if err != nil {
		t.Skip(err)
	}
	t.Logf("file of %d MB", f.Size>>20)
	u, err := c.DownloadURL(ctx, f.Path)
	if err != nil {
		t.Fatal(err)
	}
	try := func(name string, off, n int64) {
		resp, err := c.Fetch(ctx, u, off, n)
		if err != nil {
			t.Logf("%-18s %v", name, err)
			return
		}
		got, _ := io.CopyN(io.Discard, resp.Body, 1<<20)
		resp.Body.Close()
		t.Logf("%-18s %s, read %d", name, resp.Status, got)
	}
	try("no Range", 0, 0)
	try("bytes=0-1MB", 0, 1<<20)
	try("bytes=1MB-", 1<<20, 0)
	try("no Range again", 0, 0)
}

func TestLiveMPVBig(t *testing.T) {
	acc, err := pcsimport.Find()
	if err != nil {
		t.Skip(err)
	}
	c := baidu.Open(acc.Creds)
	ctx := context.Background()
	f, err := tune.FindFile(ctx, c, 10<<20)
	if err != nil {
		t.Skip(err)
	}
	u, err := c.DownloadURL(ctx, f.Path)
	if err != nil {
		t.Fatal(err)
	}
	mpv, err := player.Find("")
	if err != nil {
		t.Skip(err)
	}
	args := append([]string{"--ao=null", "--vo=null", "--no-config", "--length=3", "--msg-level=all=warn"}, player.Args(u, "t", c.Headers())...)
	out, err := exec.Command(mpv, args...).CombinedOutput()
	// mpv's output holds the signed link: report only whether it played.
	if err != nil {
		t.Errorf("mpv on %s: %v (%d bytes of output)", path.Ext(f.Name), err, len(out))
	}
}

// go test -tags live ./internal/baidu -run LiveProbe -v
func TestLiveProbe(t *testing.T) {
	acc, err := pcsimport.Find()
	if err != nil {
		t.Skip(err)
	}
	c := baidu.Open(acc.Creds)
	ctx := context.Background()
	f, err := tune.FindFile(ctx, c, 256<<20)
	if err != nil {
		t.Skip(err)
	}
	r, err := tune.Probe(ctx, c, f, []int{8, 16, 24, 32, 48}, 8*time.Second, 1500<<20)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range r.Samples {
		t.Logf("%2d 连接 %6.2f MB/s 错误 %d %s", s.Threads, s.Speed/(1<<20), s.Errors, s.Err)
	}
	t.Logf("best %d", r.Best())
}
