package fakepan

import (
	"encoding/json"
	"net/http"
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

// shareTransfer saves the one share the fake knows, 1demo with password
// abcd: a folder of pictures.
func (s *Server) shareTransfer(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !strings.HasSuffix(r.FormValue("link"), "/1demo") {
		writeJSON(w, map[string]any{"errno": -21})
		return
	}
	if r.FormValue("pwd") != "abcd" {
		writeJSON(w, map[string]any{"errno": -12, "show_msg": "提取码错误"})
		return
	}
	dir := path.Join(r.FormValue("path"), "分享的旅行照片")
	for i := range 3 {
		s.add(Node{Path: path.Join(dir, "shared-"+strconv.Itoa(i+1)+".png"), Category: 3, Data: swatchPNG(i*5+2, 400, 300)})
	}
	writeJSON(w, map[string]any{"errno": 0, "count": 1})
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
