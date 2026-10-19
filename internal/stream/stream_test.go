package stream

import (
	"bytes"
	"io"
	"math/rand"
	"net/http"
	"testing"

	"github.com/pengstem/baidunetgo/internal/baidu"
	"github.com/pengstem/baidunetgo/internal/fakepan"
)

func TestServesOpenRanges(t *testing.T) {
	srv := fakepan.New()
	defer srv.Close()
	data := make([]byte, 5*chunk+777)
	rand.New(rand.NewSource(2)).Read(data)
	srv.Add(fakepan.Node{Path: "/影片.mkv", Data: data, Category: 1})
	c := baidu.OpenAt(baidu.Credentials{BDUSS: "x", UID: 1}, srv.URL)
	s, err := Start(c, baidu.Expired)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	u := s.URL("/影片.mkv", int64(len(data)))
	for _, tc := range []struct {
		rng    string
		want   []byte
		status int
	}{
		{"", data, 200},
		{"bytes=0-", data, 206},
		{"bytes=3000000-", data[3000000:], 206},
		{"bytes=100-199", data[100:200], 206},
		{"bytes=-500", data[len(data)-500:], 206},
	} {
		req, _ := http.NewRequest("GET", u, nil)
		if tc.rng != "" {
			req.Header.Set("Range", tc.rng)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != tc.status || !bytes.Equal(got, tc.want) {
			t.Errorf("%q: %s, %d bytes, want %d", tc.rng, resp.Status, len(got), len(tc.want))
		}
	}
	resp, _ := http.Get(s.URL("/影片.mkv", 10)[:len("http://")+len(s.ln.Addr().String())] + "/guess/x")
	if resp.StatusCode != 404 {
		t.Error("an unknown token was served")
	}
}
