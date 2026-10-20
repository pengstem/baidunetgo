// Package fakepan is a pretend Baidu Netdisk: an HTTP server answering the
// web API the client uses, over a tree of files kept in memory. Tests use
// it, and `BAIDUNETGO_FAKE=1` runs the app against it, to work on the
// interface without an account.
package fakepan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Node is a file or directory of the fake netdisk.
type Node struct {
	FsID     int64
	Path     string
	IsDir    bool
	Category int
	Mtime    int64
	Data     []byte // content of a file
	Size     int64  // size of a file without Data
}

// Server is the fake netdisk.
type Server struct {
	*httptest.Server
	// mirror is a second download host, the same netdisk on another
	// port.
	mirror *httptest.Server

	mu       sync.Mutex
	nodes    map[string]*Node
	trash    map[int64]*recycled
	shares   []*share
	offlines []*offline
	nextID   int64
}

// New starts a fake netdisk holding a few sample folders and files.
func New() *Server {
	s := &Server{nodes: map[string]*Node{}, nextID: 1000}
	s.nodes["/"] = &Node{FsID: 1, Path: "/", IsDir: true}
	s.seed()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/loginStatus", s.loginStatus)
	mux.HandleFunc("/api/quota", s.quota)
	mux.HandleFunc("/rest/2.0/xpan/nas", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"errno": 0, "vip_type": 2, "baidu_name": "演示用户"})
	})
	mux.HandleFunc("/rest/2.0/membership/user", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"errno": 0, "product_infos": []any{
			map[string]any{"cluster": "vip", "detail_cluster": "svip", "end_time": 1824047999},
		}})
	})
	mux.HandleFunc("/api/list", s.list)
	mux.HandleFunc("/api/search", s.search)
	mux.HandleFunc("/thumb", s.thumb)
	mux.HandleFunc("/file", s.file)
	mux.HandleFunc("/locatedownload", s.locate)
	s.opsRoutes(mux)
	s.shareRoutes(mux)
	s.Server = httptest.NewServer(mux)
	s.mirror = httptest.NewServer(mux)
	return s
}

// Close stops both hosts.
func (s *Server) Close() {
	s.mirror.Close()
	s.Server.Close()
}

// locate lists a file's download links, one on each host, as the
// netdisk's locatedownload does.
func (s *Server) locate(w http.ResponseWriter, r *http.Request) {
	q := "/file?" + url.Values{"path": {r.URL.Query().Get("path")}}.Encode()
	writeJSON(w, map[string]any{"urls": []map[string]string{{"url": s.URL + q}, {"url": s.mirror.URL + q}}})
}

// Add puts a node in the tree, making its parent directories.
func (s *Server) Add(n Node) *Node {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.add(n)
}

func (s *Server) add(n Node) *Node {
	n.Path = path.Clean("/" + n.Path)
	if dir := path.Dir(n.Path); s.nodes[dir] == nil {
		s.add(Node{Path: dir, IsDir: true, Mtime: n.Mtime})
	}
	if n.FsID == 0 {
		s.nextID++
		n.FsID = s.nextID
	}
	if n.Mtime == 0 {
		n.Mtime = time.Now().Add(-time.Duration(n.FsID%500) * time.Hour).Unix()
	}
	if n.Data != nil {
		n.Size = int64(len(n.Data))
	}
	s.nodes[n.Path] = &n
	return &n
}

func (s *Server) seed() {
	text := "BaiduNetGo 演示文件\n\n这是一段放在假网盘里的文本，用来预览。\n"
	code := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"你好，网盘\")\n}\n"
	md := "# 旅行计划\n\n- **第一天**：抵达京都，入住町屋\n- **第二天**：伏见稻荷、清水寺\n\n> 慢慢走，比较快。\n\n```go\nfmt.Println(\"出发\")\n```\n"
	s.add(Node{Path: "/照片/京都", IsDir: true})
	for i := range 24 {
		s.add(Node{Path: fmt.Sprintf("/照片/IMG_%04d.jpg", 2310+i), Category: 3, Data: swatchPNG(i, 640, 480)})
	}
	for i := range 8 {
		s.add(Node{Path: fmt.Sprintf("/照片/京都/kyoto-%d.png", i+1), Category: 3, Data: swatchPNG(i*3+1, 800, 600)})
	}
	for i, name := range []string{"星际穿越.mkv", "千与千寻.mp4", "旅行 vlog 2026.mov"} {
		s.add(Node{Path: "/电影/" + name, Category: 1, Size: int64(1+i) * 1_800_000_000})
	}
	s.add(Node{Path: "/音乐/和弦.wav", Category: 2, Data: SampleWAV(6)})
	for _, name := range []string{"晴天.flac", "夜曲.mp3", "Clair de Lune.ogg"} {
		s.add(Node{Path: "/音乐/" + name, Category: 2, Size: 9_400_000})
	}
	s.add(Node{Path: "/文档/读我.txt", Category: 4, Data: []byte(text)})
	s.add(Node{Path: "/文档/旅行计划.md", Category: 4, Data: []byte(md)})
	s.add(Node{Path: "/文档/年度报告.pdf", Category: 4, Data: SamplePDF(4)})
	s.add(Node{Path: "/文档/预算.xlsx", Category: 4, Size: 84_000})
	s.add(Node{Path: "/代码/main.go", Category: 6, Data: []byte(code)})
	s.add(Node{Path: "/代码/项目备份.zip", Category: 6, Size: 380_000_000})
	s.add(Node{Path: "/空文件夹", IsDir: true})
	s.add(Node{Path: "/安装包.dmg", Category: 5, Size: 120_000_000})
	s.add(Node{Path: "/README.md", Category: 4, Data: []byte(md)})
}

// swatchPNG draws a soft gradient picture, different for each seed.
func swatchPNG(seed, w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	hue := float64(seed*47%360) / 360
	for y := range h {
		for x := range w {
			t := float64(x+y) / float64(w+h)
			r, g, b := hsl(math.Mod(hue+t*0.18, 1), 0.55, 0.42+0.3*t)
			img.SetRGBA(x, y, color.RGBA{r, g, b, 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func hsl(h, s, l float64) (uint8, uint8, uint8) {
	f := func(n float64) uint8 {
		k := math.Mod(n+h*12, 12)
		a := s * math.Min(l, 1-l)
		v := l - a*math.Max(-1, math.Min(math.Min(k-3, 9-k), 1))
		return uint8(v * 255)
	}
	return f(0), f(8), f(4)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (s *Server) loginStatus(w http.ResponseWriter, r *http.Request) {
	// The real endpoint sends vip_type as a string ("22" for an annual SVIP).
	writeJSON(w, map[string]any{"errno": 0, "login_info": map[string]any{
		"uk": 42, "username": "演示用户", "bdstoken": "faketoken", "photo_url": s.URL + "/thumb?id=7", "vip_type": "22", "is_svip": 1,
	}})
}

func (s *Server) quota(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"errno": 0, "total": int64(2) << 40, "used": int64(734) << 30})
}

func (s *Server) entry(n *Node) map[string]any {
	e := map[string]any{
		"fs_id": n.FsID, "path": n.Path, "server_filename": path.Base(n.Path),
		"size": n.Size, "isdir": b2i(n.IsDir), "server_mtime": n.Mtime, "server_ctime": n.Mtime,
		"category": n.Category,
	}
	if n.Category == 3 || n.Category == 1 {
		u := s.URL + "/thumb?id=" + strconv.FormatInt(n.FsID, 10)
		e["thumbs"] = map[string]string{"url1": u, "url2": u, "url3": u}
	}
	return e
}

func (s *Server) children(dir string) []*Node {
	var out []*Node
	for p, n := range s.nodes {
		if p != "/" && path.Dir(p) == dir {
			out = append(out, n)
		}
	}
	slices.SortFunc(out, func(a, b *Node) int { return strings.Compare(a.Path, b.Path) })
	return out
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := path.Clean("/" + r.URL.Query().Get("dir"))
	if n := s.nodes[dir]; n == nil || !n.IsDir {
		writeJSON(w, map[string]any{"errno": -9})
		return
	}
	var list []map[string]any
	for _, n := range s.children(dir) {
		list = append(list, s.entry(n))
	}
	writeJSON(w, map[string]any{"errno": 0, "list": list})
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strings.ToLower(r.URL.Query().Get("key"))
	dir := path.Clean("/" + r.URL.Query().Get("dir"))
	var list []map[string]any
	for p, n := range s.nodes {
		if p != "/" && strings.HasPrefix(p, strings.TrimSuffix(dir, "/")+"/") && strings.Contains(strings.ToLower(path.Base(p)), key) {
			list = append(list, s.entry(n))
		}
	}
	writeJSON(w, map[string]any{"errno": 0, "list": list})
}

func (s *Server) byID(id int64) *Node {
	for _, n := range s.nodes {
		if n.FsID == id {
			return n
		}
	}
	return nil
}

func (s *Server) thumb(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	s.mu.Lock()
	n := s.byID(id)
	s.mu.Unlock()
	if n != nil && n.Data != nil && n.Category == 3 {
		w.Write(n.Data)
		return
	}
	w.Write(swatchPNG(int(id), 240, 180))
}

// file serves a file's content with range requests, as download hosts do.
func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	n := s.nodes[path.Clean("/"+r.URL.Query().Get("path"))]
	s.mu.Unlock()
	if n == nil || n.IsDir {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	// As the netdisk's download hosts: bounded ranges only.
	rng := r.Header.Get("Range")
	if a, b, ok := strings.Cut(strings.TrimPrefix(rng, "bytes="), "-"); !ok || a == "" || b == "" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var content io.ReadSeeker = bytes.NewReader(n.Data)
	if n.Data == nil {
		content = io.NewSectionReader(zeros{}, 0, n.Size) // zeros, never allocated
	}
	http.ServeContent(w, r, path.Base(n.Path), time.Unix(n.Mtime, 0), content)
}

// zeros reads as zero bytes everywhere.
type zeros struct{}

func (zeros) ReadAt(p []byte, off int64) (int, error) {
	clear(p)
	return len(p), nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SamplePNG draws a soft gradient picture, different for each seed.
func SamplePNG(seed, w, h int) []byte { return swatchPNG(seed, w, h) }
