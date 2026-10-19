package baidu

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// QRState is where a QR login is.
type QRState int

const (
	QRWaiting   QRState = iota // shown, not scanned yet
	QRScanned                  // scanned, waiting for the phone to confirm
	QRConfirmed                // confirmed: call Finish
	QRExpired                  // the code is no longer valid: make another
)

const browserUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"

// passport is Baidu's sign-in host, a variable for tests.
var passport = "https://passport.baidu.com"

// QRLogin signs in by scanning a QR code with the Baidu Netdisk app, as the
// netdisk's web page does.
type QRLogin struct {
	// Image is the QR code, a PNG.
	Image []byte

	hc      *http.Client
	sign    string
	gid     string
	v       string
	created time.Time
}

// NewQRLogin asks for a QR code to show.
func NewQRLogin(ctx context.Context) (*QRLogin, error) {
	jar, _ := cookiejar.New(nil)
	q := &QRLogin{
		hc:      &http.Client{Jar: jar, Timeout: 45 * time.Second},
		gid:     newGID(),
		created: time.Now(),
	}
	var res struct {
		ImgURL string `json:"imgurl"`
		Sign   string `json:"sign"`
		Errno  int    `json:"errno"`
	}
	params := url.Values{
		"lp":          {"pc"},
		"qrloginfrom": {"pc"},
		"gid":         {q.gid},
		"apiver":      {"v3"},
		"tt":          {millis()},
		"tpl":         {"netdisk"},
		"_":           {millis()},
	}
	body, err := q.get(ctx, passport+"/v2/api/getqrcode?"+params.Encode())
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(jsonp(body), &res); err != nil || res.Sign == "" {
		return nil, errors.New("获取二维码失败")
	}
	q.sign = res.Sign
	img := res.ImgURL
	if !strings.HasPrefix(img, "http") {
		img = "https://" + strings.TrimPrefix(img, "//")
	}
	q.Image, err = q.get(ctx, img)
	if err != nil {
		return nil, fmt.Errorf("获取二维码失败：%w", err)
	}
	return q, nil
}

// Poll waits for the code's state to change, for up to about half a
// minute, and returns it.
func (q *QRLogin) Poll(ctx context.Context) (QRState, error) {
	if time.Since(q.created) > 3*time.Minute {
		return QRExpired, nil
	}
	params := url.Values{
		"channel_id": {q.sign},
		"gid":        {q.gid},
		"tpl":        {"netdisk"},
		"apiver":     {"v3"},
		"callback":   {""},
		"tt":         {millis()},
		"_":          {millis()},
	}
	body, err := q.get(ctx, passport+"/channel/unicast?"+params.Encode())
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) && ue.Timeout() {
			return QRWaiting, nil
		}
		return QRWaiting, err
	}
	var res struct {
		Errno    int    `json:"errno"`
		ChannelV string `json:"channel_v"`
	}
	if err := json.Unmarshal(jsonp(body), &res); err != nil {
		return QRWaiting, nil
	}
	switch res.Errno {
	case 0:
	case 1: // nothing happened while polling
		return QRWaiting, nil
	default:
		return QRExpired, nil
	}
	var cv struct {
		Status int    `json:"status"`
		V      string `json:"v"`
	}
	if err := json.Unmarshal([]byte(res.ChannelV), &cv); err != nil {
		return QRWaiting, nil
	}
	switch {
	case cv.Status == 0 && cv.V != "":
		q.v = cv.V
		return QRConfirmed, nil
	case cv.Status == 1:
		return QRScanned, nil
	case cv.Status == 2: // cancelled on the phone
		return QRExpired, nil
	}
	return QRWaiting, nil
}

// Finish signs in with a confirmed code and returns the session's
// credentials for the netdisk.
func (q *QRLogin) Finish(ctx context.Context) (Credentials, error) {
	if q.v == "" {
		return Credentials{}, errors.New("二维码尚未确认")
	}
	home := "https://pan.baidu.com/disk/home"
	params := url.Values{
		"v":            {millis()},
		"bduss":        {q.v},
		"u":            {home},
		"loginVersion": {"v4"},
		"qrcode":       {"1"},
		"tpl":          {"netdisk"},
		"apiver":       {"v3"},
		"tt":           {millis()},
		"traceid":      {""},
		"callback":     {""},
	}
	if _, err := q.get(ctx, passport+"/v3/login/main/qrbdusslogin?"+params.Encode()); err != nil {
		return Credentials{}, fmt.Errorf("登录失败：%w", err)
	}
	// The netdisk's STOKEN comes from passport's auth redirect to it.
	auth := url.Values{"return_type": {"5"}, "tpl": {"netdisk"}, "u": {home}}
	if _, err := q.get(ctx, passport+"/v3/login/api/auth/?"+auth.Encode()); err != nil {
		return Credentials{}, fmt.Errorf("获取 STOKEN 失败：%w", err)
	}
	pan, _ := url.Parse("https://pan.baidu.com/")
	cr := Credentials{}
	var parts []string
	for _, ck := range q.hc.Jar.Cookies(pan) {
		parts = append(parts, ck.Name+"="+ck.Value)
		switch ck.Name {
		case "BDUSS":
			cr.BDUSS = ck.Value
		case "STOKEN":
			cr.STOKEN = ck.Value
		}
	}
	if cr.BDUSS == "" {
		return Credentials{}, errors.New("登录失败：没有拿到 BDUSS")
	}
	if cr.STOKEN == "" {
		return Credentials{}, errors.New("登录失败：没有拿到网盘的 STOKEN，请改用 Cookie 登录")
	}
	cr.Cookies = strings.Join(parts, "; ")
	return cr, nil
}

func (q *QRLogin) get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Referer", "https://pan.baidu.com/")
	resp, err := q.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

// jsonp returns the JSON object inside a JSONP or parenthesized body.
func jsonp(b []byte) []byte {
	s := string(b)
	i, j := strings.IndexByte(s, '{'), strings.LastIndexByte(s, '}')
	if i < 0 || j < i {
		return b
	}
	return []byte(s[i : j+1])
}

func millis() string { return strconv.FormatInt(time.Now().UnixMilli(), 10) }

// newGID makes the random id the sign-in page gives itself.
func newGID() string {
	var b [16]byte
	rand.Read(b[:])
	h := fmt.Sprintf("%X", b[:])
	return h[0:7] + "-" + h[7:11] + "-4" + h[12:15] + "-" + h[15:19] + "-" + h[19:31]
}
