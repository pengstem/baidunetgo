package baidu

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qjfoidnh/baidu-tools/tieba"
)

// User is the signed-in account.
type User struct {
	UID    uint64
	UK     int64
	Name   string
	Avatar string // URL of the profile photo, may be empty
	VIP    int    // 0 none, 1 VIP, 2 SVIP
	// VIPUntil is when the membership ends, zero if unknown or none.
	VIPUntil time.Time
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
			UK       flexInt `json:"uk"`
			Username string  `json:"username"`
			Photo    string  `json:"photo_url"`
			VIPType  flexInt `json:"vip_type"`
			IsVIP    flexInt `json:"is_vip"`
			IsSVIP   flexInt `json:"is_svip"`
		} `json:"login_info"`
	}
	q := url.Values{"clienttype": {"0"}, "web": {"1"}}
	if err := c.getJSON(ctx, "检查登录", "/api/loginStatus", q, &st); err != nil {
		return nil, err
	}
	li := st.LoginInfo
	u := &User{UK: int64(li.UK), Name: li.Username, Avatar: li.Photo}
	// loginStatus's vip_type follows a scheme of its own (22 for an
	// annual SVIP); the open platform's uinfo says 0, 1 or 2.
	var ui struct {
		VIPType flexInt `json:"vip_type"`
		Avatar  string  `json:"avatar_url"`
		Name    string  `json:"baidu_name"`
	}
	if err := c.getJSON(ctx, "读取会员", "/rest/2.0/xpan/nas", url.Values{"method": {"uinfo"}, "clienttype": {"0"}, "web": {"1"}}, &ui); err == nil {
		u.VIP = int(ui.VIPType)
		if u.Avatar == "" {
			u.Avatar = ui.Avatar
		}
	} else {
		switch {
		case li.IsSVIP == 1 || li.VIPType == 2:
			u.VIP = 2
		case li.IsVIP == 1 || li.VIPType == 1:
			u.VIP = 1
		}
	}
	if u.VIP > 0 {
		u.VIPUntil = c.vipUntil(ctx)
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
	if c.creds.UID != 0 || c.fake() {
		defer uidMu.Unlock()
		return c.creds.UID, nil
	}
	t, err := tieba.NewUserInfoByBDUSS(c.creds.BDUSS)
	if err != nil {
		uidMu.Unlock()
		return 0, err
	}
	c.creds.UID = t.Baidu.UID
	c.pcs.SetUID(t.Baidu.UID)
	uidMu.Unlock()
	c.changed()
	return t.Baidu.UID, nil
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

// LoginInfo returns the login_info object of /api/loginStatus as it came,
// for diagnostics.
func (c *Client) LoginInfo(ctx context.Context) (map[string]any, error) {
	ctx, cancel := timeout(ctx)
	defer cancel()
	var st struct {
		LoginInfo map[string]any `json:"login_info"`
	}
	err := c.getJSON(ctx, "检查登录", "/api/loginStatus", url.Values{"clienttype": {"0"}, "web": {"1"}}, &st)
	return st.LoginInfo, err
}

// flexInt is a number the netdisk sends as a number or as a string.
type flexInt int64

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	*f = flexInt(n)
	return err
}

// vipUntil returns when the membership ends, from the products the
// account holds.
func (c *Client) vipUntil(ctx context.Context) time.Time {
	var res struct {
		Products []struct {
			Cluster string  `json:"cluster"`
			EndTime flexInt `json:"end_time"`
		} `json:"product_infos"`
	}
	q := url.Values{"method": {"query"}, "clienttype": {"0"}, "app_id": {"250528"}, "web": {"1"}}
	if c.getJSON(ctx, "读取会员", "/rest/2.0/membership/user", q, &res) != nil {
		return time.Time{}
	}
	var end int64
	for _, p := range res.Products {
		if p.Cluster == "vip" && int64(p.EndTime) < 1<<31 {
			end = max(end, int64(p.EndTime))
		}
	}
	if end == 0 {
		return time.Time{}
	}
	return time.Unix(end, 0)
}
