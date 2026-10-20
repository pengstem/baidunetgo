package baidu

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseCredentials(t *testing.T) {
	cr, err := ParseCredentials("Cookie: BAIDUID=x; BDUSS=abc; STOKEN=def; PANWEB=1")
	if err != nil {
		t.Fatal(err)
	}
	if cr.BDUSS != "abc" || cr.STOKEN != "def" || cr.Cookies == "" {
		t.Errorf("got %+v", cr)
	}
	cr, err = ParseCredentials("  rawbduss  ")
	if err != nil || cr.BDUSS != "rawbduss" {
		t.Errorf("bare BDUSS: %+v %v", cr, err)
	}
	if _, err := ParseCredentials("STOKEN=only"); err == nil {
		t.Error("want an error without BDUSS")
	}
	got := Credentials{BDUSS: "a", Cookies: "BDUSS=old; X=1"}.cookieString()
	if got != "BDUSS=a; X=1" {
		t.Errorf("cookieString = %q", got)
	}
}

func TestJSONP(t *testing.T) {
	got := string(jsonp([]byte(`({"errno":0,"channel_v":"{\"status\":1}"})`)))
	if got != `{"errno":0,"channel_v":"{\"status\":1}"}` {
		t.Errorf("jsonp = %s", got)
	}
}

func TestList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/list" || r.URL.Query().Get("dir") != "/photos" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("User-Agent") != UserAgent {
			t.Errorf("User-Agent %q", r.Header.Get("User-Agent"))
		}
		w.Write([]byte(`{"errno":0,"list":[
			{"fs_id":1,"path":"/photos/a.jpg","server_filename":"a.jpg","size":10,"isdir":0,"category":3,"thumbs":{"url1":"s","url3":"l"}},
			{"fs_id":2,"path":"/photos/sub","server_filename":"sub","isdir":1}]}`))
	}))
	defer srv.Close()
	c := Open(Credentials{BDUSS: "x", STOKEN: "y", UID: 1})
	c.base = srv.URL
	files, err := c.List(context.Background(), "/photos", Order{})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Thumb != "l" || !files[1].IsDir || files[0].Category != CategoryImage {
		t.Errorf("files %+v", files)
	}
}

func TestErrno(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/list":
			w.Write([]byte(`{"errno":-6}`))
		default:
			w.Write([]byte(`{"errno":-9,"show_msg":""}`))
		}
	}))
	defer srv.Close()
	c := Open(Credentials{BDUSS: "x", UID: 1})
	c.base = srv.URL
	if _, err := c.List(context.Background(), "/", Order{}); !errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("errno -6: %v", err)
	}
	err := c.getJSON(context.Background(), "删除", "/api/other", nil, nil)
	var ae *APIError
	if !errors.As(err, &ae) || ae.Errno != -9 || ae.Error() != "删除失败：文件不存在 (-9)" {
		t.Errorf("errno -9: %v", err)
	}
}

func TestRankHosts(t *testing.T) {
	c := Open(Credentials{BDUSS: "x"})
	got := c.rank([]string{"https://nb.cache.x/1", "https://d.pcs.baidu.com/1", "https://b.baidupcs.com/1"})
	if Host(got[0]) != "d.pcs.baidu.com" || Host(got[2]) != "nb.cache.x" {
		t.Errorf("nb.cache not last: %v", got)
	}
	c.SetHosts([]string{"b.baidupcs.com"})
	if got := c.rank(got); Host(got[0]) != "b.baidupcs.com" {
		t.Errorf("ranked host not first: %v", got)
	}
}

// TestTransportVerifies checks that content reads and the library's own
// requests both check certificates, which the library's transport did not.
func TestTransportVerifies(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	c := Open(Credentials{BDUSS: "x"})
	if _, err := c.Fetch(context.Background(), srv.URL, 0, 1); err == nil {
		t.Error("Fetch trusted a self-signed certificate")
	}
	if _, err := c.PCS().GetClient().Req(http.MethodGet, srv.URL, nil, nil); err == nil {
		t.Error("the library's client trusted a self-signed certificate")
	}
}
