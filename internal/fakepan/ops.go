package fakepan

import (
	"encoding/json"
	"io"
	"net/http"
	"path"
	"strings"
)

// recycled is a deleted node and what was under it.
type recycled struct {
	root  *Node
	nodes []*Node
}

func (s *Server) opsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/create", s.create)
	mux.HandleFunc("/api/filemanager", s.filemanager)
	mux.HandleFunc("/api/recycle/list", s.recycleList)
	mux.HandleFunc("/api/recycle/restore", s.recycleRestore)
	mux.HandleFunc("/api/recycle/delete", s.recyclePurge)
	mux.HandleFunc("/api/recycle/clear", s.recycleClear)
	mux.HandleFunc("/upload", s.upload)
}

// upload stores the request's body as a file, as the netdisk's uploads end.
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	// A speed probe's parts, never joined into a file.
	if r.URL.Query().Has("probe") {
		io.Copy(io.Discard, r.Body)
		return
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := path.Clean("/" + r.URL.Query().Get("path"))
	cat := 6
	if strings.HasSuffix(p, ".png") || strings.HasSuffix(p, ".jpg") {
		cat = 3
	}
	s.add(Node{Path: p, Data: data, Category: cat})
	writeJSON(w, map[string]any{"errno": 0})
}

// Data returns the content of a file of the fake netdisk.
func (s *Server) Data(p string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n := s.nodes[path.Clean(p)]; n != nil {
		return n.Data
	}
	return nil
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := path.Clean("/" + r.FormValue("path"))
	if s.nodes[p] != nil {
		writeJSON(w, map[string]any{"errno": -8})
		return
	}
	n := s.add(Node{Path: p, IsDir: r.FormValue("isdir") == "1"})
	writeJSON(w, map[string]any{"errno": 0, "fs_id": n.FsID, "path": n.Path})
}

// subtree returns the node at p and every node under it.
func (s *Server) subtree(p string) []*Node {
	var out []*Node
	for q, n := range s.nodes {
		if q == p || strings.HasPrefix(q, p+"/") {
			out = append(out, n)
		}
	}
	return out
}

func (s *Server) filemanager(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw := r.FormValue("filelist")
	switch r.URL.Query().Get("opera") {
	case "delete":
		var paths []string
		json.Unmarshal([]byte(raw), &paths)
		for _, p := range paths {
			p = path.Clean(p)
			n := s.nodes[p]
			if n == nil {
				writeJSON(w, map[string]any{"errno": 12})
				return
			}
			sub := s.subtree(p)
			for _, m := range sub {
				delete(s.nodes, m.Path)
			}
			if s.trash == nil {
				s.trash = map[int64]*recycled{}
			}
			s.trash[n.FsID] = &recycled{root: n, nodes: sub}
		}
	case "rename", "move", "copy":
		var list []map[string]string
		json.Unmarshal([]byte(raw), &list)
		op := r.URL.Query().Get("opera")
		for _, it := range list {
			from := path.Clean(it["path"])
			to := path.Join(path.Dir(from), it["newname"])
			if op != "rename" {
				to = path.Join(it["dest"], it["newname"])
			}
			if s.nodes[from] == nil || s.nodes[to] != nil || to == from || strings.HasPrefix(to, from+"/") {
				writeJSON(w, map[string]any{"errno": 12})
				return
			}
			for _, m := range s.subtree(from) {
				moved := *m
				moved.Path = to + strings.TrimPrefix(m.Path, from)
				if op == "copy" {
					s.nextID++
					moved.FsID = s.nextID
				} else {
					delete(s.nodes, m.Path)
				}
				s.nodes[moved.Path] = &moved
			}
		}
	default:
		writeJSON(w, map[string]any{"errno": 2})
		return
	}
	writeJSON(w, map[string]any{"errno": 0, "info": []any{}, "taskid": 0})
}

func (s *Server) recycleList(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []map[string]any
	for _, t := range s.trash {
		e := s.entry(t.root)
		e["leftTime"] = 10
		list = append(list, e)
	}
	writeJSON(w, map[string]any{"errno": 0, "list": list})
}

func (s *Server) fidlist(r *http.Request) []int64 {
	var ids []int64
	json.Unmarshal([]byte(r.FormValue("fidlist")), &ids)
	return ids
}

func (s *Server) recycleRestore(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range s.fidlist(r) {
		if t := s.trash[id]; t != nil {
			for _, n := range t.nodes {
				s.nodes[n.Path] = n
			}
			delete(s.trash, id)
		}
	}
	writeJSON(w, map[string]any{"errno": 0})
}

func (s *Server) recyclePurge(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range s.fidlist(r) {
		delete(s.trash, id)
	}
	writeJSON(w, map[string]any{"errno": 0})
}

func (s *Server) recycleClear(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trash = nil
	writeJSON(w, map[string]any{"errno": 0})
}

// Exists reports whether the fake netdisk holds a path.
func (s *Server) Exists(p string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nodes[path.Clean(p)] != nil
}
