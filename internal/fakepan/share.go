package fakepan

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

type share struct {
	ID      int64
	Paths   []string
	Pwd     string
	Created int64
	Expires int64
	Status  int
}

type offline struct {
	ID      int64
	Name    string
	Source  string
	Save    string
	Status  int
	Created int64
}

func (s *Server) shareRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/share/pset", s.sharePSet)
	mux.HandleFunc("/share/record", s.shareRecord)
	mux.HandleFunc("/share/surlinfoinrecord", s.shareInfo)
	mux.HandleFunc("/share/cancel", s.shareCancel)
	mux.HandleFunc("/share/verify", s.shareVerify)
	mux.HandleFunc("/api/shorturlinfo", s.shortURLInfo)
	mux.HandleFunc("/share/list", s.sharedList)
	mux.HandleFunc("/s/", s.sharePage)
	mux.HandleFunc("/share/transfer", s.shareTransfer)
	mux.HandleFunc("/rest/2.0/services/cloud_dl", s.cloudDl)
}

func (s *Server) sharePSet(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var paths []string
	json.Unmarshal([]byte(r.FormValue("path_list")), &paths)
	days, _ := strconv.Atoi(r.FormValue("period"))
	s.nextID++
	sh := &share{ID: s.nextID, Paths: paths, Pwd: r.FormValue("pwd"), Created: time.Now().Unix()}
	if days > 0 {
		sh.Expires = time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix()
	}
	s.shares = append(s.shares, sh)
	writeJSON(w, map[string]any{"errno": 0, "link": "https://pan.baidu.com/s/1fake" + strconv.FormatInt(sh.ID, 10), "shareid": sh.ID})
}

func (s *Server) shareRecord(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []map[string]any
	for i := len(s.shares) - 1; i >= 0; i-- {
		sh := s.shares[i]
		var ids []int64
		for _, p := range sh.Paths {
			if n := s.nodes[p]; n != nil {
				ids = append(ids, n.FsID)
			}
		}
		list = append(list, map[string]any{
			"shareId": sh.ID, "shortlink": "https://pan.baidu.com/s/1fake" + strconv.FormatInt(sh.ID, 10),
			"typicalPath": sh.Paths[0], "fsIds": ids, "status": sh.Status, "expiredTime": sh.Expires,
			"vCnt": int(sh.ID % 37), "ctime": sh.Created, "typicalCategory": 6,
		})
	}
	writeJSON(w, map[string]any{"errno": 0, "list": list})
}

func (s *Server) shareInfo(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, _ := strconv.ParseInt(r.URL.Query().Get("shareid"), 10, 64)
	for _, sh := range s.shares {
		if sh.ID == id {
			writeJSON(w, map[string]any{"errno": 0, "pwd": sh.Pwd})
			return
		}
	}
	writeJSON(w, map[string]any{"errno": -9})
}

func (s *Server) shareCancel(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []int64
	json.Unmarshal([]byte(r.FormValue("shareid_list")), &ids)
	kept := s.shares[:0]
	for _, sh := range s.shares {
		cancelled := false
		for _, id := range ids {
			cancelled = cancelled || sh.ID == id
		}
		if !cancelled {
			kept = append(kept, sh)
		}
	}
	s.shares = kept
	writeJSON(w, map[string]any{"errno": 0})
}

// DemoShare is the link of a share of someone else's that the fake
// knows, and its password: a folder of travel pictures and a list.
const (
	DemoShare    = "https://pan.baidu.com/s/1demo"
	DemoSharePwd = "abcd"
	demoShareID  = 777
)

func (s *Server) seedShare() {
	s.demo = map[string]*Node{}
	for i := range 3 {
		s.addIn(s.demo, Node{Path: "/分享的旅行照片/shared-" + strconv.Itoa(i+1) + ".png", Category: 3, Data: swatchPNG(i*5+2, 400, 300)})
	}
	s.addIn(s.demo, Node{Path: "/分享的旅行照片/海边日落.mp4", Category: 1, Size: 286_000_000})
	s.addIn(s.demo, Node{Path: "/旅行清单.txt", Category: 4, Data: []byte("护照\n相机\n充电器\n")})
}

// shared is what a share link of the fake opens: its tree, the paths it
// shares and its password; ok is false when there is no such share.
type shared struct {
	id    int64
	nodes map[string]*Node
	roots []string
	pwd   string
	mine  bool
	made  int64
	until int64
}

func (s *Server) sharedBy(surl string) (sh shared, ok bool) {
	surl = strings.TrimPrefix(surl, "1")
	if surl == "demo" {
		var roots []string
		for _, n := range childrenIn(s.demo, "/") {
			roots = append(roots, n.Path)
		}
		return shared{id: demoShareID, nodes: s.demo, roots: roots, pwd: DemoSharePwd, made: 1755653442}, true
	}
	for _, x := range s.shares {
		if surl == "fake"+strconv.FormatInt(x.ID, 10) {
			return shared{id: x.ID, nodes: s.nodes, roots: x.Paths, pwd: x.Pwd, mine: true, made: x.Created, until: x.Expires}, true
		}
	}
	return shared{}, false
}

// sekey is what a share's password opens it with.
func sekey(id int64) string { return "sk" + strconv.FormatInt(id, 10) + "=" }

func (s *Server) shareVerify(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sh, ok := s.sharedBy("1" + r.URL.Query().Get("surl"))
	switch {
	case !ok:
		writeJSON(w, map[string]any{"errno": 105})
	case r.FormValue("pwd") != sh.pwd:
		writeJSON(w, map[string]any{"errno": -9})
	default:
		writeJSON(w, map[string]any{"errno": 0, "randsk": url.QueryEscape(sekey(sh.id))})
	}
}

// opened returns the share a call is about, when its password opened it.
func (s *Server) opened(w http.ResponseWriter, surl, key string) (shared, bool) {
	sh, ok := s.sharedBy(surl)
	switch {
	case !ok:
		writeJSON(w, map[string]any{"errno": 140, "show_msg": "啊哦，链接出错了"})
		return sh, false
	case key != sekey(sh.id):
		writeJSON(w, map[string]any{"errno": -9, "show_msg": "提取码验证失败，请重试"})
		return sh, false
	}
	return sh, true
}

func (s *Server) shortURLInfo(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := r.URL.Query()
	sh, ok := s.opened(w, q.Get("shorturl"), q.Get("spd"))
	if !ok {
		return
	}
	left := int64(0)
	if sh.until > 0 {
		left = sh.until - time.Now().Unix()
	}
	writeJSON(w, map[string]any{"errno": 0, "shareid": sh.id, "uk": 42, "ctime": sh.made, "expiredtype": left, "fcount": len(sh.roots)})
}

// sharedList lists a share's top, or a folder of it, with numbers as
// strings at the top as the netdisk sends them.
func (s *Server) sharedList(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := r.URL.Query()
	sh, ok := s.opened(w, q.Get("shorturl"), q.Get("sekey"))
	if !ok {
		return
	}
	list := []map[string]any{}
	if q.Get("root") == "1" {
		for _, p := range sh.roots {
			if n := sh.nodes[p]; n != nil {
				e := s.entry(n)
				for k, v := range e {
					if n, ok := v.(int64); ok {
						e[k] = strconv.FormatInt(n, 10)
					}
				}
				list = append(list, e)
			}
		}
		writeJSON(w, map[string]any{"errno": 0, "list": list, "share_id": sh.id, "uk": 42})
		return
	}
	dir := path.Clean("/" + q.Get("dir"))
	if !sh.holds(dir) {
		writeJSON(w, map[string]any{"errno": 2})
		return
	}
	for _, n := range childrenIn(sh.nodes, dir) {
		list = append(list, s.entry(n))
	}
	writeJSON(w, map[string]any{"errno": 0, "list": list})
}

// holds reports whether a path is in what the share shares.
func (sh shared) holds(p string) bool {
	for _, root := range sh.roots {
		if p == root || strings.HasPrefix(p, root+"/") {
			return true
		}
	}
	return false
}

// sharePage is the share's web page, which tells who shared it.
func (s *Server) sharePage(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sh, ok := s.sharedBy(path.Base(r.URL.Path))
	if !ok {
		w.Write([]byte(`<html><body class="error-404"></body></html>`))
		return
	}
	owner, photo := "旅行的猫", s.URL+"/thumb?id=11"
	if sh.mine {
		owner, photo = "演示用户", s.URL+"/thumb?id=7"
	}
	page, _ := json.Marshal(map[string]any{"loginstate": 1, "linkusername": owner, "photo": photo, "self": b2i(sh.mine), "shareid": sh.id})
	w.Write([]byte("<html><script>locals.mset(" + string(page) + ");</script></html>"))
}

// shareTransfer saves files of an opened share into a folder, a name
// taken already getting a number, as ondup=newcopy asks.
func (s *Server) shareTransfer(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := r.URL.Query()
	id, _ := strconv.ParseInt(q.Get("shareid"), 10, 64)
	var sh shared
	for _, surl := range s.shareURLs() {
		if x, ok := s.sharedBy(surl); ok && x.id == id {
			sh = x
		}
	}
	if sh.nodes == nil || q.Get("sekey") != sekey(sh.id) {
		writeJSON(w, map[string]any{"errno": -9})
		return
	}
	var ids []int64
	json.Unmarshal([]byte(r.FormValue("fsidlist")), &ids)
	dir := path.Clean("/" + r.FormValue("path"))
	if n := s.nodes[dir]; n == nil || !n.IsDir {
		writeJSON(w, map[string]any{"errno": 2, "show_msg": "目标文件夹不存在"})
		return
	}
	var info []map[string]any
	for _, id := range ids {
		var from *Node
		for _, n := range sh.nodes {
			if n.FsID == id && sh.holds(n.Path) {
				from = n
			}
		}
		if from == nil {
			writeJSON(w, map[string]any{"errno": 2})
			return
		}
		to := path.Join(dir, path.Base(from.Path))
		for i := 1; s.nodes[to] != nil; i++ {
			ext := path.Ext(from.Path)
			to = path.Join(dir, strings.TrimSuffix(path.Base(from.Path), ext)+"("+strconv.Itoa(i)+")"+ext)
		}
		var copies []Node
		for p, n := range sh.nodes {
			if p == from.Path || strings.HasPrefix(p, from.Path+"/") {
				c := *n
				c.FsID, c.Path = 0, to+strings.TrimPrefix(p, from.Path)
				copies = append(copies, c)
			}
		}
		for _, c := range copies {
			s.add(c)
		}
		info = append(info, map[string]any{"errno": 0, "fsid": id, "path": from.Path})
	}
	writeJSON(w, map[string]any{"errno": 0, "info": info, "task_id": 0})
}

// shareURLs are the short links of every share the fake knows.
func (s *Server) shareURLs() []string {
	out := []string{"1demo"}
	for _, x := range s.shares {
		out = append(out, "1fake"+strconv.FormatInt(x.ID, 10))
	}
	return out
}

func (s *Server) cloudDl(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := r.URL.Query()
	switch q.Get("method") {
	case "add_task":
		src := q.Get("source_url")
		if !strings.Contains(src, "://") && !strings.HasPrefix(src, "magnet:") {
			writeJSON(w, map[string]any{"error_code": 36016, "error_msg": "链接无效"})
			return
		}
		s.nextID++
		name := path.Base(strings.Split(src, "?")[0])
		s.offlines = append(s.offlines, &offline{ID: s.nextID, Name: name, Source: src, Save: q.Get("save_path"), Status: 1, Created: time.Now().Unix()})
		writeJSON(w, map[string]any{"task_id": s.nextID})
	case "list_task":
		var list []map[string]any
		for _, o := range s.offlines {
			// A task finishes the second time it is listed.
			done := "4000000"
			if o.Status == 1 && time.Since(time.Unix(o.Created, 0)) > time.Second {
				o.Status = 0
				s.add(Node{Path: path.Join(o.Save, o.Name), Category: 6, Size: 8_000_000})
			}
			if o.Status == 0 {
				done = "8000000"
			}
			list = append(list, map[string]any{
				"task_id": strconv.FormatInt(o.ID, 10), "status": strconv.Itoa(o.Status), "file_size": "8000000",
				"finished_size": done, "create_time": strconv.FormatInt(o.Created, 10), "save_path": o.Save,
				"source_url": o.Source, "task_name": o.Name,
			})
		}
		writeJSON(w, map[string]any{"task_info": list})
	case "cancel_task", "delete_task":
		id, _ := strconv.ParseInt(q.Get("task_id"), 10, 64)
		kept := s.offlines[:0]
		for _, o := range s.offlines {
			if o.ID == id && q.Get("method") == "cancel_task" {
				o.Status = 7
			}
			if o.ID != id || q.Get("method") == "cancel_task" {
				kept = append(kept, o)
			}
		}
		s.offlines = kept
		writeJSON(w, map[string]any{"request_id": 1})
	default:
		writeJSON(w, map[string]any{"error_code": 3, "error_msg": "unknown method"})
	}
}
