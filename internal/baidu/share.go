package baidu

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs/netdisksign"
)

// Share is a link sharing files of the netdisk.
type Share struct {
	ID       int64   `json:"shareId"`
	Link     string  `json:"shortlink"`
	Path     string  `json:"typicalPath"`
	Category int     `json:"typicalCategory"`
	Status   int     `json:"status"`
	Expires  int64   `json:"expiredTime"` // unix time, 0 for never
	Views    int     `json:"vCnt"`
	Created  int64   `json:"ctime"`
	FsIDs    []int64 `json:"fsIds"`
}

// Expired reports whether the link no longer works.
func (s *Share) Expired(now time.Time) bool {
	return s.Status != 0 || (s.Expires > 0 && s.Expires < now.Unix())
}

// CreateShare shares files with a link and its 4-character password, for
// days days (0 for ever).
func (c *Client) CreateShare(ctx context.Context, paths []string, pwd string, days int) (link string, err error) {
	ctx, cancel := timeout(ctx)
	defer cancel()
	q, err := c.webQuery(ctx, url.Values{"channel": {"chunlei"}})
	if err != nil {
		return "", err
	}
	list, _ := json.Marshal(paths)
	form := url.Values{
		"path_list": {string(list)}, "schannel": {"4"}, "channel_list": {"[]"},
		"period": {strconv.Itoa(days)}, "pwd": {pwd}, "share_type": {"9"},
	}
	var res struct {
		Link string `json:"link"`
	}
	if err := c.postForm(ctx, "创建分享", "/share/pset", q, form, &res); err != nil {
		return "", err
	}
	return res.Link, nil
}

// Shares returns the links the account made, newest first.
func (c *Client) Shares(ctx context.Context) ([]Share, error) {
	var all []Share
	for page := 1; page < 20; page++ {
		pctx, cancel := timeout(ctx)
		var res struct {
			List []Share `json:"list"`
		}
		q := url.Values{"page": {strconv.Itoa(page)}, "desc": {"1"}, "order": {"time"}, "num": {"100"}, "clienttype": {"0"}, "web": {"1"}}
		err := c.getJSON(pctx, "读取分享", "/share/record", q, &res)
		cancel()
		if err != nil {
			return nil, err
		}
		all = append(all, res.List...)
		if len(res.List) < 100 {
			break
		}
	}
	return all, nil
}

// SharePassword returns the password of a share of the account.
func (c *Client) SharePassword(ctx context.Context, id int64) (string, error) {
	ctx, cancel := timeout(ctx)
	defer cancel()
	var res struct {
		Pwd string `json:"pwd"`
	}
	q := url.Values{"shareid": {strconv.FormatInt(id, 10)}, "sign": {string(netdisksign.ShareSURLInfoSign(id))}}
	err := c.getJSON(ctx, "读取提取码", "/share/surlinfoinrecord", q, &res)
	if res.Pwd == "0" {
		res.Pwd = ""
	}
	return res.Pwd, err
}

// CancelShares stops links.
func (c *Client) CancelShares(ctx context.Context, ids []int64) error {
	ctx, cancel := timeout(ctx)
	defer cancel()
	q, err := c.webQuery(ctx, nil)
	if err != nil {
		return err
	}
	return c.postForm(ctx, "取消分享", "/share/cancel", q, url.Values{"shareid_list": {fidList(ids)}}, nil)
}

var (
	shareLinkRe = regexp.MustCompile(`https?://pan\.baidu\.com/(?:s/([0-9A-Za-z_-]+)|share/init\?surl=([0-9A-Za-z_-]+))`)
	sharePwdRe  = regexp.MustCompile(`(?:pwd=|提取码[:：]?\s*|密码[:：]?\s*)([0-9A-Za-z]{4})`)
)

// ParseShareLink finds a share link, and its password, in what a user
// pastes: a link alone, or the text the netdisk writes around it.
func ParseShareLink(text string) (link, pwd string, ok bool) {
	m := shareLinkRe.FindStringSubmatch(text)
	if m == nil {
		return "", "", false
	}
	surl := m[1]
	if surl == "" {
		surl = "1" + m[2]
	}
	if p := sharePwdRe.FindStringSubmatch(text); p != nil {
		pwd = p[1]
	}
	return "https://pan.baidu.com/s/" + surl, pwd, true
}

// ErrSharePassword reports that a share link asks for a password it was
// not given, or was given a wrong one.
var ErrSharePassword = errors.New("提取码不对")

// ErrShareGone reports that a share link does not work, or no longer does.
var ErrShareGone = errors.New("分享链接已失效")

// Shared is what a share link shares, opened with its password: read
// before saving any of it.
type Shared struct {
	Link    string // https://pan.baidu.com/s/1…
	Pwd     string
	ID      int64
	UK      int64  // the sharer's
	Owner   string // the sharer's name as the netdisk shows it, partly hidden; "" when unknown
	Avatar  string // the URL of the sharer's photo
	Created int64  // unix time
	Expires int64  // unix time, 0 for never
	Mine    bool   // shared by the account itself
	Files   []File // what it shares, at its top
	sekey   string // what the password opened, which the share's calls carry
}

// sharedPage is how many entries a share's listing asks for at a time.
const sharedPage = 100

// shareQuery is the query every call about a share carries.
func shareQuery(extra url.Values) url.Values {
	q := url.Values{"channel": {"chunlei"}, "web": {"1"}, "app_id": {"250528"}, "clienttype": {"0"}}
	for k, v := range extra {
		q[k] = v
	}
	return q
}

// shareError says why a share's call failed in the user's words.
func shareError(err error) error {
	var ae *APIError
	if !errors.As(err, &ae) {
		return err
	}
	switch ae.Errno {
	case -9, -12:
		return ErrSharePassword
	case -62, -63, -64:
		return errors.New("提取码试错太多次，请稍后再试")
	}
	return ErrShareGone
}

// OpenShare opens a share link with its password, "" for none, and reads
// what it shares and who shared it.
func (c *Client) OpenShare(ctx context.Context, link, pwd string) (*Shared, error) {
	ctx, cancel := timeout(ctx)
	defer cancel()
	feature := path.Base(strings.TrimSuffix(link, "/"))
	if len(feature) < 2 || len(feature) > 32 || !strings.HasPrefix(feature, "1") {
		return nil, errors.New("分享链接格式不对")
	}
	surl, page := feature[1:], "https://pan.baidu.com/share/init?surl="+feature[1:]
	s := &Shared{Link: "https://pan.baidu.com/s/" + feature, Pwd: pwd}
	if pwd != "" {
		var res struct {
			Randsk string `json:"randsk"`
		}
		q := shareQuery(url.Values{"surl": {surl}, "t": {strconv.FormatInt(time.Now().UnixMilli(), 10)}})
		form := url.Values{"pwd": {pwd}, "vcode": {""}, "vcode_str": {""}}
		if err := c.doJSONFrom(ctx, "打开分享", http.MethodPost, "/share/verify", page, q, form, &res); err != nil {
			return nil, shareError(err)
		}
		s.sekey, _ = url.QueryUnescape(res.Randsk)
		// The netdisk reads the opened share from BDCLND, set for
		// pan.baidu.com; one kept from another session for .baidu.com,
		// sent first, would hide it.
		c.http.Jar.SetCookies(&url.URL{Scheme: "https", Host: "baidu.com", Path: "/"},
			[]*http.Cookie{{Name: "BDCLND", Value: res.Randsk, Domain: ".baidu.com", Path: "/"}})
	}
	var info struct {
		ShareID     int64 `json:"shareid"`
		UK          int64 `json:"uk"`
		Ctime       int64 `json:"ctime"`
		ExpiredType int64 `json:"expiredtype"` // seconds left, 0 for never
	}
	q := shareQuery(url.Values{"shorturl": {feature}, "root": {"1"}, "spd": {s.sekey}})
	if err := c.doJSONFrom(ctx, "打开分享", http.MethodGet, "/api/shorturlinfo", page, q, nil, &info); err != nil {
		return nil, shareError(err)
	}
	s.ID, s.UK, s.Created = info.ShareID, info.UK, info.Ctime
	if info.ExpiredType > 0 {
		s.Expires = time.Now().Unix() + info.ExpiredType
	}
	// Who shared it shows only on the share's page; it is not worth
	// failing for.
	owner := make(chan struct{})
	go func() {
		defer close(owner)
		c.shareOwner(ctx, s)
	}()
	files, err := c.sharedList(ctx, s, url.Values{"root": {"1"}})
	<-owner
	if err != nil {
		return nil, err
	}
	s.Files = files
	return s, nil
}

var (
	ownerRe = regexp.MustCompile(`"linkusername":"([^"]*)"`)
	photoRe = regexp.MustCompile(`"share_photo":"([^"]*)"`) // "photo" is who is signed in
	selfRe  = regexp.MustCompile(`"self":(\d)`)
)

// shareOwner reads who shared s from the share's page.
func (c *Client) shareOwner(ctx context.Context, s *Shared) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/s/"+path.Base(s.Link), nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	page, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	unquote := func(b []byte) string {
		var v string
		json.Unmarshal([]byte(`"`+string(b)+`"`), &v)
		return v
	}
	if m := ownerRe.FindSubmatch(page); m != nil {
		s.Owner = unquote(m[1])
	}
	if m := photoRe.FindSubmatch(page); m != nil {
		s.Avatar = unquote(m[1])
	}
	if m := selfRe.FindSubmatch(page); m != nil {
		s.Mine = string(m[1]) == "1"
	}
}

// SharedDir returns what a folder of a share holds; dir is its path as
// the share's listing gives it.
func (c *Client) SharedDir(ctx context.Context, s *Shared, dir string) ([]File, error) {
	ctx, cancel := timeout(ctx)
	defer cancel()
	return c.sharedList(ctx, s, url.Values{
		"root": {"0"}, "dir": {dir},
		"shareid": {strconv.FormatInt(s.ID, 10)}, "uk": {strconv.FormatInt(s.UK, 10)},
	})
}

// sharedList reads a listing of a share, every page of it, folders first.
func (c *Client) sharedList(ctx context.Context, s *Shared, where url.Values) ([]File, error) {
	var all []File
	for page := 1; page < 50; page++ {
		var res struct {
			List []sharedFile `json:"list"`
		}
		q := shareQuery(url.Values{
			"shorturl": {path.Base(s.Link)[1:]}, "sekey": {s.sekey}, "web": {"5"},
			"page": {strconv.Itoa(page)}, "num": {strconv.Itoa(sharedPage)}, "order": {"name"}, "desc": {"0"},
		})
		for k, v := range where {
			q[k] = v
		}
		if err := c.doJSONFrom(ctx, "读取分享", http.MethodGet, "/share/list", s.Link, q, nil, &res); err != nil {
			return nil, shareError(err)
		}
		for _, f := range res.List {
			all = append(all, f.file())
		}
		if len(res.List) < sharedPage {
			break
		}
	}
	slices.SortStableFunc(all, func(a, b File) int {
		switch {
		case a.IsDir && !b.IsDir:
			return -1
		case !a.IsDir && b.IsDir:
			return 1
		}
		return 0
	})
	return all, nil
}

// sharedFile is a file as a share's listing gives it.
type sharedFile struct {
	FsID     flexInt `json:"fs_id"`
	Path     string  `json:"path"`
	Name     string  `json:"server_filename"`
	Size     flexInt `json:"size"`
	IsDir    flexInt `json:"isdir"`
	Ctime    flexInt `json:"server_ctime"`
	Mtime    flexInt `json:"server_mtime"`
	Category flexInt `json:"category"`
	Thumbs   struct {
		URL1 string `json:"url1"`
		URL2 string `json:"url2"`
		URL3 string `json:"url3"`
	} `json:"thumbs"`
}

func (r sharedFile) file() File {
	f := File{
		FsID: int64(r.FsID), Path: r.Path, Name: r.Name, Size: int64(r.Size), IsDir: r.IsDir == 1,
		Ctime: int64(r.Ctime), Mtime: int64(r.Mtime), Category: int(r.Category),
	}
	if f.Name == "" {
		f.Name = path.Base(f.Path)
	}
	f.Thumb = cmp.Or(r.Thumbs.URL2, r.Thumbs.URL3, r.Thumbs.URL1)
	f.ThumbSmall = cmp.Or(r.Thumbs.URL1, f.Thumb)
	return f
}

// SaveShared saves files of a share, by their ids, into dir of the
// netdisk; none saves everything it shares. It returns how many it saved.
func (c *Client) SaveShared(ctx context.Context, s *Shared, fsids []int64, dir string) (int, error) {
	if len(fsids) == 0 {
		for _, f := range s.Files {
			fsids = append(fsids, f.FsID)
		}
	}
	if len(fsids) == 0 {
		return 0, errors.New("分享里没有文件")
	}
	tctx, cancel := timeout(ctx)
	defer cancel()
	q, err := c.webQuery(tctx, shareQuery(url.Values{
		"shareid": {strconv.FormatInt(s.ID, 10)}, "from": {strconv.FormatInt(s.UK, 10)},
		"sekey": {s.sekey}, "ondup": {"newcopy"}, "async": {"1"},
	}))
	if err != nil {
		return 0, err
	}
	var res struct {
		TaskID int64 `json:"task_id"`
	}
	form := url.Values{"fsidlist": {fidList(fsids)}, "path": {dir}}
	if err := c.doJSONFrom(tctx, "转存", http.MethodPost, "/share/transfer", s.Link, q, form, &res); err != nil {
		return 0, transferError(err)
	}
	if res.TaskID != 0 {
		if err := c.waitShareTask(ctx, res.TaskID); err != nil {
			return 0, err
		}
	}
	return len(fsids), nil
}

// transferError says why saving a share failed in the user's words.
func transferError(err error) error {
	var ae *APIError
	if !errors.As(err, &ae) {
		return err
	}
	var res struct {
		Info []struct {
			Errno int    `json:"errno"`
			Path  string `json:"path"`
		} `json:"info"`
		Count int `json:"target_file_nums"`
		Limit int `json:"target_file_nums_limit"`
	}
	json.Unmarshal(ae.body, &res)
	switch {
	case res.Limit > 0 && res.Count > res.Limit:
		return fmt.Errorf("一次最多转存 %d 个文件，这里有 %d 个", res.Limit, res.Count)
	case len(res.Info) > 0 && res.Info[0].Errno == -30:
		return fmt.Errorf("「%s」已经在那里了", path.Base(res.Info[0].Path))
	case ae.Errno == -10 || ae.Errno == -32:
		return errors.New("网盘空间不足")
	case ae.Errno == 4:
		return errors.New("这些文件已经转存过了")
	case ae.Errno == -9 || ae.Errno == -12:
		return ErrSharePassword
	}
	return err
}

// waitShareTask waits for the netdisk to finish a save it took on as a
// task, for the many files it saves.
func (c *Client) waitShareTask(ctx context.Context, id int64) error {
	for {
		var res struct {
			Status string `json:"status"`
		}
		tctx, cancel := timeout(ctx)
		err := c.getJSON(tctx, "转存", "/share/taskquery", shareQuery(url.Values{"taskid": {strconv.FormatInt(id, 10)}}), &res)
		cancel()
		switch {
		case err != nil:
			return transferError(err)
		case res.Status == "success":
			return nil
		case res.Status == "failed":
			return errors.New("转存失败")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// SaveShare saves everything a share link shares into dir of the netdisk,
// and returns how many items it saved.
func (c *Client) SaveShare(ctx context.Context, link, pwd, dir string) (int, error) {
	s, err := c.OpenShare(ctx, link, pwd)
	if err != nil {
		return 0, err
	}
	return c.SaveShared(ctx, s, nil, dir)
}

// Offline is a task downloading a URL into the netdisk.
type Offline struct {
	ID       int64
	Name     string
	Source   string
	SavePath string
	Size     int64
	Done     int64
	Status   int // 0 done, 1 running, 2..7 failed, see StatusText
	Created  int64
}

// StatusText says what an offline task's status means.
func (o *Offline) StatusText() string {
	switch o.Status {
	case 0:
		return "已完成"
	case 1:
		return "下载中"
	case 3:
		return "资源不存在"
	case 4:
		return "下载超时"
	case 5:
		return "下载失败"
	case 6:
		return "空间不足"
	case 7:
		return "已取消"
	}
	return "系统错误"
}

func (c *Client) cloudDl(ctx context.Context, op, method string, q url.Values, v any) error {
	ctx, cancel := timeout(ctx)
	defer cancel()
	q.Set("method", method)
	q.Set("app_id", "250528")
	return c.postForm(ctx, op, "/rest/2.0/services/cloud_dl", q, url.Values{}, v)
}

// AddOffline asks the netdisk to download a URL (http, ftp or magnet)
// into dir.
func (c *Client) AddOffline(ctx context.Context, source, dir string) error {
	return c.cloudDl(ctx, "添加离线任务", "add_task", url.Values{"source_url": {source}, "save_path": {dir}, "task_from": {"0"}, "selected_idx": {"1"}}, nil)
}

// Offlines returns the offline tasks.
func (c *Client) Offlines(ctx context.Context) ([]Offline, error) {
	var res struct {
		TaskInfo []struct {
			TaskID       string `json:"task_id"`
			Status       string `json:"status"`
			FileSize     string `json:"file_size"`
			FinishedSize string `json:"finished_size"`
			CreateTime   string `json:"create_time"`
			SavePath     string `json:"save_path"`
			SourceURL    string `json:"source_url"`
			TaskName     string `json:"task_name"`
		} `json:"task_info"`
	}
	q := url.Values{"need_task_info": {"1"}, "status": {"255"}, "start": {"0"}, "limit": {"1000"}}
	if err := c.cloudDl(ctx, "读取离线任务", "list_task", q, &res); err != nil {
		return nil, err
	}
	n := func(s string) int64 { v, _ := strconv.ParseInt(s, 10, 64); return v }
	out := make([]Offline, len(res.TaskInfo))
	for i, t := range res.TaskInfo {
		out[i] = Offline{ID: n(t.TaskID), Name: t.TaskName, Source: t.SourceURL, SavePath: t.SavePath,
			Size: n(t.FileSize), Done: n(t.FinishedSize), Status: int(n(t.Status)), Created: n(t.CreateTime)}
	}
	return out, nil
}

// CancelOffline stops an offline task; DeleteOffline forgets it.
func (c *Client) CancelOffline(ctx context.Context, id int64) error {
	return c.cloudDl(ctx, "取消离线任务", "cancel_task", url.Values{"task_id": {strconv.FormatInt(id, 10)}}, nil)
}

func (c *Client) DeleteOffline(ctx context.Context, id int64) error {
	return c.cloudDl(ctx, "删除离线任务", "delete_task", url.Values{"task_id": {strconv.FormatInt(id, 10)}}, nil)
}
