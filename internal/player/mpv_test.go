package player

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/pengstem/baidunetgo/internal/fakepan"
)

func TestArgs(t *testing.T) {
	h := http.Header{}
	h.Set("User-Agent", "netdisk;1")
	h.Set("Cookie", "BDUSS=a,b; STOKEN=c")
	args := Args("https://x/f", "电影.mkv", h)
	want := map[string]bool{"--user-agent=netdisk;1": true, "--http-header-fields-append=Cookie: BDUSS=a,b; STOKEN=c": true, "--force-media-title=电影.mkv": true}
	for _, a := range args {
		delete(want, a)
	}
	if len(want) > 0 || args[len(args)-1] != "https://x/f" || args[len(args)-2] != "--" {
		t.Errorf("args %q, missing %v", args, want)
	}
}

// TestMPVStreams plays a WAV through the installed mpv, which must send
// the netdisk's headers.
func TestMPVStreams(t *testing.T) {
	mpv, err := Find("")
	if err != nil {
		t.Skip("mpv not installed")
	}
	wav := fakepan.SampleWAV(0.5)
	var mu sync.Mutex
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = r.Header.Clone()
		mu.Unlock()
		http.ServeContent(w, r, "a.wav", time.Time{}, bytes.NewReader(wav))
	}))
	defer srv.Close()
	h := http.Header{}
	h.Set("User-Agent", "netdisk;test")
	h.Set("Cookie", "BDUSS=x")
	args := append([]string{"--ao=null", "--vo=null", "--really-quiet", "--no-config"}, Args(srv.URL+"/a.wav", "a", h)...)
	cmd := exec.Command(mpv, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mpv: %v\n%s", err, out)
	}
	mu.Lock()
	defer mu.Unlock()
	if got.Get("User-Agent") != "netdisk;test" || got.Get("Cookie") != "BDUSS=x" {
		t.Errorf("mpv sent %v", got)
	}
}
