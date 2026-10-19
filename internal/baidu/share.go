package baidu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
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

// SaveShare saves what a share link shares into dir of the netdisk, and
// returns how many items it saved.
func (c *Client) SaveShare(ctx context.Context, link, pwd, dir string) (int, error) {
	if c.fake() {
		var res struct {
			Count int `json:"count"`
		}
		q, err := c.webQuery(ctx, nil)
		if err != nil {
			return 0, err
		}
		err = c.postForm(ctx, "转存", "/share/transfer", q, url.Values{"link": {link}, "pwd": {pwd}, "path": {dir}}, &res)
		return res.Count, err
	}
	feature := path.Base(strings.TrimSuffix(link, "/"))
	if len(feature) > 23 || !strings.HasPrefix(feature, "1") {
		return 0, errors.New("分享链接格式不对")
	}
	if len(pwd) != 4 {
		return 0, errors.New("请填写 4 位提取码")
	}
	type result struct {
		n   int
		err error
	}
	done := make(chan result, 1)
	go func() {
		n, err := c.saveShare(feature, link, pwd, dir)
		done <- result{n, err}
	}()
	select {
	case r := <-done:
		return r.n, r.err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// saveShare follows BaiduPCS-Go's RunShareTransfer: open the share's page,
// give its password, read what it shares, and transfer it.
func (c *Client) saveShare(feature, link, pwd, dir string) (int, error) {
	c.mu.Lock() // the library resets the session's cookies on the way
	defer c.mu.Unlock()
	pcs := c.pcs
	tokens := pcs.AccessSharePage(feature, true)
	if tokens["ErrMsg"] != "0" {
		return 0, fmt.Errorf("打开分享失败：%s", tokens["ErrMsg"])
	}
	verify := pcs.GenerateShareQueryURL("verify", map[string]string{
		"shareid": tokens["shareid"], "time": strconv.FormatInt(time.Now().UnixMilli(), 10),
		"clienttype": "1", "uk": tokens["share_uk"],
	}).String()
	res := pcs.PostShareQuery(verify, link, map[string]string{"pwd": pwd, "vcode": "null", "vcode_str": "null", "bdstoken": tokens["bdstoken"]})
	if res["ErrMsg"] != "0" {
		return 0, fmt.Errorf("提取码不对：%s", res["ErrMsg"])
	}
	pcs.UpdatePCSCookies(true)
	tokens = pcs.AccessSharePage(feature, false)
	if tokens["ErrMsg"] != "0" {
		return 0, fmt.Errorf("打开分享失败：%s", tokens["ErrMsg"])
	}
	list := pcs.GenerateShareQueryURL("list", map[string]string{
		"bdstoken": tokens["bdstoken"], "root": "1", "web": "5", "app_id": "250528",
		"shorturl": feature[1:], "channel": "chunlei",
	}).String()
	metas := pcs.ExtractShareInfo(list, tokens["shareid"], tokens["share_uk"], tokens["bdstoken"])
	if metas["ErrMsg"] != "success" {
		return 0, fmt.Errorf("读取分享内容失败：%s", metas["ErrMsg"])
	}
	metas["path"] = dir
	metas["referer"] = "https://pan.baidu.com/s/" + feature
	pcs.UpdatePCSCookies(true)
	resp := pcs.GenerateRequestQuery("POST", metas)
	if resp["ErrNo"] != "0" {
		return 0, fmt.Errorf("转存失败：%s", resp["ErrMsg"])
	}
	n, _ := strconv.Atoi(metas["item_num"])
	return max(n, 1), nil
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
