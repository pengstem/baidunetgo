package baidu

import (
	"context"
	"encoding/json"
	"net/url"
	"path"
	"strconv"
	"time"
)

// webQuery returns the query the web API's writes take, with the session's
// bdstoken.
func (c *Client) webQuery(ctx context.Context, extra url.Values) (url.Values, error) {
	tok, err := c.bdstoken(ctx)
	if err != nil {
		return nil, err
	}
	q := url.Values{"bdstoken": {tok}, "clienttype": {"0"}, "app_id": {"250528"}, "web": {"1"}}
	for k, v := range extra {
		q[k] = v
	}
	return q, nil
}

// bdstoken returns the token that the web API's writes carry, asking for it
// once.
func (c *Client) bdstoken(ctx context.Context) (string, error) {
	c.mu.Lock()
	tok := c.token
	c.mu.Unlock()
	if tok != "" {
		return tok, nil
	}
	var st struct {
		LoginInfo struct {
			BDSToken string `json:"bdstoken"`
		} `json:"login_info"`
	}
	if err := c.getJSON(ctx, "检查登录", "/api/loginStatus", url.Values{"clienttype": {"0"}, "web": {"1"}}, &st); err != nil {
		return "", err
	}
	c.mu.Lock()
	c.token = st.LoginInfo.BDSToken
	c.mu.Unlock()
	return st.LoginInfo.BDSToken, nil
}

// Mkdir makes a directory.
func (c *Client) Mkdir(ctx context.Context, p string) error {
	ctx, cancel := timeout(ctx)
	defer cancel()
	q, err := c.webQuery(ctx, url.Values{"a": {"commit"}})
	if err != nil {
		return err
	}
	form := url.Values{"path": {p}, "isdir": {"1"}, "block_list": {"[]"}}
	return c.postForm(ctx, "新建文件夹", "/api/create", q, form, nil)
}

// fileOp runs an operation of the file manager on a list of files.
func (c *Client) fileOp(ctx context.Context, op, opera string, list any) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	q, err := c.webQuery(ctx, url.Values{"opera": {opera}, "async": {"2"}, "onnest": {"fail"}})
	if err != nil {
		return err
	}
	data, _ := json.Marshal(list)
	var res struct {
		TaskID int64 `json:"taskid"`
	}
	if err := c.postForm(ctx, op, "/api/filemanager", q, url.Values{"filelist": {string(data)}}, &res); err != nil {
		return err
	}
	if res.TaskID == 0 {
		return nil
	}
	return c.waitTask(ctx, op, res.TaskID)
}

// waitTask waits for an operation the netdisk carries on in the
// background.
func (c *Client) waitTask(ctx context.Context, op string, id int64) error {
	for {
		var st struct {
			Status string `json:"status"`
			Errno  int    `json:"task_errno"`
		}
		q := url.Values{"taskid": {strconv.FormatInt(id, 10)}, "clienttype": {"0"}, "web": {"1"}}
		if err := c.getJSON(ctx, op, "/share/taskquery", q, &st); err != nil {
			return err
		}
		switch st.Status {
		case "success":
			return nil
		case "failed":
			return &APIError{Op: op, Errno: st.Errno}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(700 * time.Millisecond):
		}
	}
}

// Rename gives a file a new name in its directory.
func (c *Client) Rename(ctx context.Context, p, name string) error {
	return c.fileOp(ctx, "重命名", "rename", []map[string]string{{"path": p, "newname": name}})
}

// Move moves files into a directory.
func (c *Client) Move(ctx context.Context, paths []string, dir string) error {
	return c.MoveAs(ctx, paths, nil, dir)
}

// Copy copies files into a directory.
func (c *Client) Copy(ctx context.Context, paths []string, dir string) error {
	return c.CopyAs(ctx, paths, nil, dir)
}

// MoveAs moves files into a directory, each under the name names gives
// it, or its own where names is nil.
func (c *Client) MoveAs(ctx context.Context, paths, names []string, dir string) error {
	return c.fileOp(ctx, "移动", "move", moveList(paths, names, dir))
}

// CopyAs copies files into a directory, each under the name names gives
// it, or its own where names is nil.
func (c *Client) CopyAs(ctx context.Context, paths, names []string, dir string) error {
	return c.fileOp(ctx, "复制", "copy", moveList(paths, names, dir))
}

func moveList(paths, names []string, dir string) []map[string]string {
	list := make([]map[string]string, len(paths))
	for i, p := range paths {
		name := path.Base(p)
		if names != nil {
			name = names[i]
		}
		list[i] = map[string]string{"path": p, "dest": dir, "newname": name}
	}
	return list
}

// Delete moves files to the recycle bin.
func (c *Client) Delete(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	return c.fileOp(ctx, "删除", "delete", paths)
}

// Recycled is a file in the recycle bin.
type Recycled struct {
	File
	LeftDays int
}

// RecycleList returns what is in the recycle bin.
func (c *Client) RecycleList(ctx context.Context) ([]Recycled, error) {
	var all []Recycled
	for page := 1; page < 50; page++ {
		pctx, cancel := timeout(ctx)
		var res struct {
			List []struct {
				rawFile
				LeftTime int `json:"leftTime"`
			} `json:"list"`
		}
		q := url.Values{"num": {"100"}, "page": {strconv.Itoa(page)}, "clienttype": {"0"}, "web": {"1"}}
		err := c.getJSON(pctx, "读取回收站", "/api/recycle/list", q, &res)
		cancel()
		if err != nil {
			return nil, err
		}
		for _, r := range res.List {
			all = append(all, Recycled{File: r.rawFile.file(), LeftDays: r.LeftTime})
		}
		if len(res.List) < 100 {
			break
		}
	}
	return all, nil
}

func fidList(ids []int64) string {
	data, _ := json.Marshal(ids)
	return string(data)
}

// Restore puts files of the recycle bin back.
func (c *Client) Restore(ctx context.Context, ids []int64) error {
	ctx, cancel := timeout(ctx)
	defer cancel()
	q, err := c.webQuery(ctx, url.Values{"async": {"1"}})
	if err != nil {
		return err
	}
	return c.postForm(ctx, "还原", "/api/recycle/restore", q, url.Values{"fidlist": {fidList(ids)}}, nil)
}

// Purge deletes files of the recycle bin for good.
func (c *Client) Purge(ctx context.Context, ids []int64) error {
	ctx, cancel := timeout(ctx)
	defer cancel()
	q, err := c.webQuery(ctx, url.Values{"async": {"1"}})
	if err != nil {
		return err
	}
	return c.postForm(ctx, "彻底删除", "/api/recycle/delete", q, url.Values{"fidlist": {fidList(ids)}}, nil)
}

// EmptyRecycle deletes everything in the recycle bin for good.
func (c *Client) EmptyRecycle(ctx context.Context) error {
	ctx, cancel := timeout(ctx)
	defer cancel()
	q, err := c.webQuery(ctx, url.Values{"async": {"1"}})
	if err != nil {
		return err
	}
	return c.getJSON(ctx, "清空回收站", "/api/recycle/clear", q, nil)
}
