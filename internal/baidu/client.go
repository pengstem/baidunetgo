// Package baidu talks to Baidu Netdisk. It wraps BaiduPCS-Go's baidupcs
// package, building its client from cookies without its global config, and
// adds what the GUI needs that the library leaves out: listings with
// thumbnails, the user's profile, and links ready for range requests and
// mpv.
package baidu

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/qjfoidnh/BaiduPCS-Go/baidupcs"
	"github.com/qjfoidnh/BaiduPCS-Go/pcsverbose"
)

// AppID is the PCS app id the BaiduPCS-Go CLI uses by default.
const AppID = 266719

// UserAgent is what the netdisk's own client sends, which download links
// require.
var UserAgent = baidupcs.NetdiskUA

// ErrNotLoggedIn reports that the cookies no longer sign in.
var ErrNotLoggedIn = errors.New("登录已失效，请重新登录")

func init() {
	// The library logs to stderr when verbose; keep it quiet.
	pcsverbose.IsVerbose = false
}

// Credentials sign in to the netdisk.
type Credentials struct {
	BDUSS  string `json:"bduss"`
	STOKEN string `json:"stoken"`
	// Cookies is the whole cookie string of a browser or of the QR login,
	// which carries more than BDUSS and STOKEN; it may be empty.
	Cookies string `json:"cookies,omitempty"`
	UID     uint64 `json:"uid,omitempty"`
}

// cookieString returns every cookie of the credentials as one header, with
// BDUSS and STOKEN given priority.
func (cr Credentials) cookieString() string {
	seen := map[string]bool{}
	var parts []string
	add := func(name, value string) {
		if value == "" || seen[name] {
			return
		}
		seen[name] = true
		parts = append(parts, name+"="+value)
	}
	add("BDUSS", cr.BDUSS)
	add("STOKEN", cr.STOKEN)
	for name, value := range parseCookies(cr.Cookies) {
		add(name, value)
	}
	return strings.Join(parts, "; ")
}

// ParseCredentials reads what a user pastes: a browser's cookie header, a
// "BDUSS=…; STOKEN=…" pair, or a bare BDUSS.
func ParseCredentials(s string) (Credentials, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "Cookie:")
	s = strings.TrimPrefix(s, "cookie:")
	s = strings.TrimSpace(s)
	if s == "" {
		return Credentials{}, errors.New("请粘贴 Cookie 或 BDUSS")
	}
	if !strings.Contains(s, "=") {
		return Credentials{BDUSS: s}, nil
	}
	m := parseCookies(s)
	cr := Credentials{BDUSS: m["BDUSS"], STOKEN: m["STOKEN"]}
	if cr.BDUSS == "" {
		return Credentials{}, errors.New("没有找到 BDUSS")
	}
	if len(m) > 2 {
		cr.Cookies = s
	}
	return cr, nil
}

// parseCookies splits a cookie header into names and values.
func parseCookies(s string) map[string]string {
	m := map[string]string{}
	for _, part := range strings.Split(s, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || name == "" {
			continue
		}
		m[strings.TrimSpace(name)] = strings.TrimSpace(value)
	}
	return m
}

// Client is a signed-in netdisk session. Its methods are safe from any
// goroutine.
type Client struct {
	mu    sync.Mutex
	token string // bdstoken, once asked for

	pcs   *baidupcs.BaiduPCS
	http  *http.Client
	creds Credentials
	// base is the netdisk's web API, a variable for tests.
	base string
}

// Open makes a client of the credentials. It does not check them: User
// does.
func Open(cr Credentials) *Client {
	pcs := baidupcs.NewPCSWithCookieStr(AppID, cr.cookieString())
	pcs.SetHTTPS(true)
	pcs.SetPanUserAgent(UserAgent)
	if cr.STOKEN != "" {
		pcs.SetStoken(cr.STOKEN)
	}
	if cr.UID != 0 {
		pcs.SetUID(cr.UID)
	}
	hc := pcs.GetClient()
	// The library turns certificate checks off; turn them back on.
	hc.SetKeepAlive(true)
	if tr, ok := hc.Client.Transport.(*http.Transport); ok {
		tr.TLSClientConfig = &tls.Config{}
	}
	return &Client{pcs: pcs, http: &hc.Client, creds: cr, base: panBase}
}

const panBase = "https://pan.baidu.com"

// OpenAt makes a client of a netdisk at base, a fake one in tests: its
// download links are base's /file.
func OpenAt(cr Credentials, base string) *Client {
	c := Open(cr)
	c.base = base
	return c
}

// fake reports whether the client talks to a pretend netdisk.
func (c *Client) fake() bool { return c.base != panBase }

// Credentials returns what the client signs in with, with the uid once
// User has found it.
func (c *Client) Credentials() Credentials { return c.creds }

// PCS returns the library's client, for operations this package does not
// wrap.
func (c *Client) PCS() *baidupcs.BaiduPCS { return c.pcs }

// HTTP returns the client's HTTP client, whose jar holds the session.
func (c *Client) HTTP() *http.Client { return c.http }

// panError is the error envelope of pan.baidu.com's API.
type panError struct {
	Errno   int    `json:"errno"`
	ErrMsg  string `json:"errmsg"`
	ShowMsg string `json:"show_msg"`
	// The PCS-style services (cloud_dl) answer with these instead.
	ErrorCode int    `json:"error_code"`
	ErrorMsg  string `json:"error_msg"`
}

// APIError is an error the netdisk answered with.
type APIError struct {
	Op    string
	Errno int
	Msg   string
}

func (e *APIError) Error() string {
	msg := e.Msg
	if msg == "" {
		msg = errnoText(e.Errno)
	}
	return fmt.Sprintf("%s失败：%s (%d)", e.Op, msg, e.Errno)
}

func errnoText(n int) string {
	switch n {
	case -6, -4, 111:
		return "登录已失效"
	case -7:
		return "文件名不合法"
	case -8:
		return "已存在同名文件"
	case -9:
		return "文件不存在"
	case -10, -32:
		return "网盘空间不足"
	case 2:
		return "参数错误"
	case 12:
		return "部分文件操作失败"
	case 31066:
		return "文件不存在"
	case 105:
		return "链接格式错误"
	case -21:
		return "分享已失效"
	case -62, -70:
		return "需要验证码，请稍后再试"
	}
	return "未知错误"
}

// getJSON GETs a netdisk API path with the session's cookies and decodes
// its JSON into v, turning errno into errors.
func (c *Client) getJSON(ctx context.Context, op, path string, q url.Values, v any) error {
	return c.doJSON(ctx, op, http.MethodGet, path, q, nil, v)
}

// postForm POSTs a form to a netdisk API path.
func (c *Client) postForm(ctx context.Context, op, path string, q url.Values, form url.Values, v any) error {
	return c.doJSON(ctx, op, http.MethodPost, path, q, form, v)
}

func (c *Client) doJSON(ctx context.Context, op, method, path string, q url.Values, form url.Values, v any) error {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Referer", "https://pan.baidu.com/disk/home")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s失败：%w", op, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("%s失败：%w", op, err)
	}
	var pe panError
	if err := json.Unmarshal(data, &pe); err != nil {
		return fmt.Errorf("%s失败：服务器返回 %s", op, resp.Status)
	}
	if pe.Errno == 0 && pe.ErrorCode != 0 {
		pe.Errno, pe.ShowMsg = pe.ErrorCode, pe.ErrorMsg
	}
	if pe.Errno != 0 {
		if pe.Errno == -6 || pe.Errno == -4 || pe.Errno == 111 {
			return ErrNotLoggedIn
		}
		msg := pe.ShowMsg
		if msg == "" {
			msg = pe.ErrMsg
		}
		return &APIError{Op: op, Errno: pe.Errno, Msg: msg}
	}
	if v == nil {
		return nil
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s失败：%w", op, err)
	}
	return nil
}

// timeout gives a context a deadline for one API call.
func timeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 30*time.Second)
}
