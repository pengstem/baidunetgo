package baidu

import (
	"context"
	"net/url"
	"sync"

	"github.com/qjfoidnh/baidu-tools/tieba"
)

// User is the signed-in account.
type User struct {
	UID    uint64
	UK     int64
	Name   string
	Avatar string // URL of the profile photo, may be empty
	VIP    int    // 0 none, 1 VIP, 2 SVIP
}

// VIPLabel names the membership level.
func (u *User) VIPLabel() string {
	switch u.VIP {
	case 1:
		return "VIP"
	case 2:
		return "SVIP"
	}
	return ""
}

var uidMu sync.Mutex

// User checks the session and returns the account, finding the uid the
// download links are signed with.
func (c *Client) User(ctx context.Context) (*User, error) {
	ctx, cancel := timeout(ctx)
	defer cancel()
	var st struct {
		LoginInfo struct {
			UK       int64  `json:"uk"`
			Username string `json:"username"`
			Photo    string `json:"photo_url"`
			VIPType  int    `json:"vip_type"`
			IsVIP    int    `json:"is_vip"`
			IsSVIP   int    `json:"is_svip"`
		} `json:"login_info"`
	}
	q := url.Values{"clienttype": {"0"}, "web": {"1"}}
	if err := c.getJSON(ctx, "检查登录", "/api/loginStatus", q, &st); err != nil {
		return nil, err
	}
	li := st.LoginInfo
	u := &User{UK: li.UK, Name: li.Username, Avatar: li.Photo}
	switch {
	case li.IsSVIP == 1 || li.VIPType == 2:
		u.VIP = 2
	case li.IsVIP == 1 || li.VIPType == 1:
		u.VIP = 1
	}
	uid, err := c.uid()
	if err == nil {
		u.UID = uid
	}
	if u.Name == "" && err == nil {
		u.Name = "百度用户"
	}
	return u, nil
}

// uid returns the Baidu account id, asking Tieba for it once.
func (c *Client) uid() (uint64, error) {
	uidMu.Lock()
	defer uidMu.Unlock()
	if c.creds.UID != 0 {
		return c.creds.UID, nil
	}
	t, err := tieba.NewUserInfoByBDUSS(c.creds.BDUSS)
	if err != nil {
		return 0, err
	}
	c.creds.UID = t.Baidu.UID
	c.pcs.SetUID(t.Baidu.UID)
	return c.creds.UID, nil
}

// Quota is the netdisk's space.
type Quota struct {
	Total, Used int64
}

// Quota returns how much of the netdisk is used.
func (c *Client) Quota(ctx context.Context) (Quota, error) {
	ctx, cancel := timeout(ctx)
	defer cancel()
	var q struct {
		Total int64 `json:"total"`
		Used  int64 `json:"used"`
	}
	err := c.getJSON(ctx, "获取空间", "/api/quota", url.Values{"checkexpire": {"1"}, "checkfree": {"1"}, "clienttype": {"0"}, "web": {"1"}}, &q)
	return Quota{Total: q.Total, Used: q.Used}, err
}
