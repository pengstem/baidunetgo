package baidu

import (
	"cmp"
	"context"
	"net/url"
	"path"
	"strconv"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
)

// File is a file or directory of the netdisk.
type File struct {
	FsID     int64  `json:"fs_id"`
	Path     string `json:"path"`
	Name     string `json:"server_filename"`
	Size     int64  `json:"size"`
	IsDir    bool   `json:"-"`
	Ctime    int64  `json:"server_ctime"`
	Mtime    int64  `json:"server_mtime"`
	MD5      string `json:"md5"` // of the content, as the netdisk tells it
	Category int    `json:"category"`
	// Thumb is the URL of a thumbnail, for pictures and videos, about
	// 360×270; ThumbSmall of one about 140×90.
	Thumb      string `json:"-"`
	ThumbSmall string `json:"-"`
}

// Categories of files, as the netdisk sorts them.
const (
	CategoryVideo   = 1
	CategoryAudio   = 2
	CategoryImage   = 3
	CategoryDoc     = 4
	CategoryApp     = 5
	CategoryOther   = 6
	CategoryTorrent = 7
)

// rawFile is a file as /api/list returns it.
type rawFile struct {
	File
	IsDirInt int `json:"isdir"`
	Thumbs   struct {
		URL1 string `json:"url1"`
		URL2 string `json:"url2"`
		URL3 string `json:"url3"`
	} `json:"thumbs"`
}

func (r rawFile) file() File {
	f := r.File
	f.IsDir = r.IsDirInt == 1
	f.MD5 = baidupcs.DecryptMD5(f.MD5) // listed scrambled
	if f.Name == "" {
		f.Name = path.Base(f.Path)
	}
	f.Thumb = cmp.Or(r.Thumbs.URL2, r.Thumbs.URL3, r.Thumbs.URL1)
	f.ThumbSmall = cmp.Or(r.Thumbs.URL1, f.Thumb)
	return f
}

// Order is how a listing is sorted.
type Order struct {
	By   string // "name", "time" or "size"
	Desc bool
}

const listPage = 1000

// List returns the files of a directory, directories first, reading every
// page of it.
func (c *Client) List(ctx context.Context, dir string, o Order) ([]File, error) {
	if o.By == "" {
		o.By = "name"
	}
	var all []File
	seen := map[int64]bool{}
	for page := 1; ; page++ {
		pctx, cancel := timeout(ctx)
		var res struct {
			List []rawFile `json:"list"`
		}
		q := url.Values{
			"dir":        {dir},
			"order":      {o.By},
			"desc":       {boolStr(o.Desc)},
			"clienttype": {"0"},
			"web":        {"1"},
			"showempty":  {"1"},
			"num":        {strconv.Itoa(listPage)},
			"page":       {strconv.Itoa(page)},
		}
		err := c.getJSON(pctx, "读取目录", "/api/list", q, &res)
		cancel()
		if err != nil {
			return nil, err
		}
		fresh := 0
		for _, r := range res.List {
			if seen[r.FsID] {
				continue
			}
			seen[r.FsID] = true
			fresh++
			all = append(all, r.file())
		}
		if len(res.List) < listPage || fresh == 0 {
			break
		}
	}
	return all, nil
}

// Search finds files whose names contain key under dir, recursively.
func (c *Client) Search(ctx context.Context, dir, key string) ([]File, error) {
	ctx, cancel := timeout(ctx)
	defer cancel()
	var res struct {
		List []rawFile `json:"list"`
	}
	q := url.Values{
		"dir":        {dir},
		"key":        {key},
		"recursion":  {"1"},
		"clienttype": {"0"},
		"web":        {"1"},
		"num":        {"500"},
		"page":       {"1"},
	}
	if err := c.getJSON(ctx, "搜索", "/api/search", q, &res); err != nil {
		return nil, err
	}
	files := make([]File, len(res.List))
	for i, r := range res.List {
		files[i] = r.file()
	}
	return files, nil
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
