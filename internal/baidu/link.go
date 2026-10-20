package baidu

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

var pcsURL = &url.URL{Scheme: "https", Host: "pcs.baidu.com", Path: "/"}

// DownloadURL returns a link to the file's content that range requests
// with Headers can read, as a player streams it. Links expire within
// hours: ask for a new one each time.
func (c *Client) DownloadURL(ctx context.Context, path string) (string, error) {
	links, err := c.DownloadURLs(ctx, path)
	if err != nil {
		return "", err
	}
	return links[0], nil
}

// DownloadURLs returns the links the netdisk offers to the file's
// content, each on its own host, the one to use first: hosts SetHosts
// ranked, in its order, then others, then the nb.cache hosts, which
// throttle hard.
func (c *Client) DownloadURLs(ctx context.Context, path string) ([]string, error) {
	if c.fake() {
		return c.fakeURLs(ctx, path)
	}
	if _, err := c.uid(); err != nil {
		return nil, fmt.Errorf("获取下载链接失败：%w", err)
	}
	type result struct {
		links []string
		err   error
	}
	done := make(chan result, 1)
	go func() {
		info, perr := c.pcs.LocateDownload(path)
		if perr != nil {
			done <- result{err: fmt.Errorf("获取下载链接失败：%w", perr)}
			return
		}
		var links []string
		seen := map[string]bool{}
		for _, u := range info.URLStrings(true) {
			if !seen[u.Host] {
				seen[u.Host] = true
				links = append(links, u.String())
			}
		}
		if len(links) == 0 {
			done <- result{err: errors.New("获取下载链接失败：没有可用链接")}
			return
		}
		done <- result{links: c.rank(links)}
	}()
	select {
	case r := <-done:
		return r.links, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// fakeURLs asks a pretend netdisk for its links to the file.
func (c *Client) fakeURLs(ctx context.Context, path string) ([]string, error) {
	var res struct {
		URLs []struct {
			URL string `json:"url"`
		} `json:"urls"`
	}
	if err := c.getJSON(ctx, "获取下载链接", "/locatedownload", url.Values{"path": {path}}, &res); err != nil {
		return nil, err
	}
	var links []string
	for _, u := range res.URLs {
		links = append(links, u.URL)
	}
	if len(links) == 0 {
		return nil, errors.New("获取下载链接失败：没有可用链接")
	}
	return c.rank(links), nil
}

// SetHosts ranks download hosts, fastest first, as a measurement found
// them; DownloadURL prefers them in this order.
func (c *Client) SetHosts(hosts []string) {
	c.mu.Lock()
	c.hosts = slices.Clone(hosts)
	c.mu.Unlock()
}

// rank orders links as DownloadURLs returns them.
func (c *Client) rank(links []string) []string {
	c.mu.Lock()
	hosts := c.hosts
	c.mu.Unlock()
	score := func(link string) int {
		h := Host(link)
		if i := slices.Index(hosts, h); i >= 0 {
			return i
		}
		if strings.HasPrefix(h, "nb.cache") {
			return len(hosts) + 1
		}
		return len(hosts)
	}
	slices.SortStableFunc(links, func(a, b string) int { return score(a) - score(b) })
	return links
}

// Host returns the host of a link.
func Host(link string) string {
	if u, err := url.Parse(link); err == nil {
		return u.Host
	}
	return ""
}

// Cookie returns the cookie header that download hosts take, those the
// session holds for pcs.baidu.com.
func (c *Client) Cookie() string {
	var parts []string
	for _, ck := range c.http.Jar.Cookies(pcsURL) {
		parts = append(parts, ck.Name+"="+ck.Value)
	}
	return strings.Join(parts, "; ")
}

// Headers returns the headers requests for a download link need.
func (c *Client) Headers() http.Header {
	h := http.Header{}
	h.Set("User-Agent", UserAgent)
	if ck := c.Cookie(); ck != "" {
		h.Set("Cookie", ck)
	}
	return h
}

// Fetch GETs a download or thumbnail URL with the session's headers,
// reading bytes [off, off+n) when n > 0, all of it when n <= 0.
func (c *Client) Fetch(ctx context.Context, u string, off, n int64) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header = c.Headers()
	if n > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, off+n-1))
	} else if off > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", off))
	}
	resp, err := c.data().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, &HTTPError{Status: resp.StatusCode, Body: string(body)}
	}
	return resp, nil
}

// HTTPError is a download host refusing a request.
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	if e.Status == http.StatusForbidden {
		return "下载被拒绝 (403)，链接可能已过期"
	}
	return fmt.Sprintf("下载失败：HTTP %d", e.Status)
}

// Expired reports whether err means the link should be asked for again.
func Expired(err error) bool {
	var he *HTTPError
	return errors.As(err, &he) && (he.Status == http.StatusForbidden || he.Status == http.StatusUnauthorized || he.Status == http.StatusGone)
}

// data returns the HTTP client for content: the session's transport,
// without its jar (the cookie header is set by hand) and without a
// timeout over the whole body, which streams.
func (c *Client) data() *http.Client {
	return &http.Client{Transport: c.http.Transport}
}
