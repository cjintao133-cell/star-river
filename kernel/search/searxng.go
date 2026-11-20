package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"star-river/kernel/netx"
)

// defaultHTTPTimeout 适配器默认 HTTP 超时。
const defaultHTTPTimeout = 15 * time.Second

// maxRespBytes 响应体上限。
const maxRespBytes = 8 << 20

// SearXNGProvider 自托管 SearXNG 适配器（GET {BaseURL}/search?format=json）。
type SearXNGProvider struct {
	BaseURL string
	HTTP    *http.Client
}

// Name 实现 Provider。
func (p *SearXNGProvider) Name() string { return "searxng" }

// Caps 实现 Provider。
func (p *SearXNGProvider) Caps() Capabilities {
	return Capabilities{HasThumbnail: true, SupportsRecency: true, SupportsChinese: true}
}

type searxngResp struct {
	Results []struct {
		Title         string `json:"title"`
		URL           string `json:"url"`
		Content       string `json:"content"`
		ImgSrc        string `json:"img_src"`
		Thumbnail     string `json:"thumbnail"`
		PublishedDate string `json:"publishedDate"`
		Engine        string `json:"engine"`
	} `json:"results"`
}

// Search 实现 Provider。
func (p *SearXNGProvider) Search(ctx context.Context, q Query) ([]Result, error) {
	if strings.TrimSpace(p.BaseURL) == "" {
		return nil, &APIError{Kind: KindDisabled, Message: "searxng: empty base url"}
	}
	u, err := url.Parse(strings.TrimRight(p.BaseURL, "/") + "/search")
	if err != nil {
		return nil, &APIError{Kind: KindProtocol, Message: "searxng: bad url: " + err.Error()}
	}
	vals := u.Query()
	vals.Set("q", q.Text)
	vals.Set("format", "json")
	if q.Lang != "" {
		vals.Set("language", q.Lang)
	}
	if q.Recency != "" {
		vals.Set("time_range", searxngTimeRange(q.Recency))
	}
	u.RawQuery = vals.Encode()

	hc := p.HTTP
	if hc == nil {
		hc = netx.Client(defaultHTTPTimeout)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, &APIError{Kind: KindProtocol, Message: err.Error()}
	}
	req.Header.Set("Accept", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return nil, transportErr("searxng", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, classifyStatus(resp.StatusCode, "searxng: "+string(body))
	}
	var r searxngResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, &APIError{Kind: KindProtocol, Message: "searxng: bad json: " + err.Error()}
	}
	out := make([]Result, 0, len(r.Results))
	for _, it := range r.Results {
		thumb := it.Thumbnail
		if thumb == "" {
			thumb = it.ImgSrc
		}
		out = append(out, Result{
			Title: it.Title, URL: it.URL, Snippet: it.Content,
			Thumbnail: thumb, PublishedAt: it.PublishedDate, Source: "searxng",
		})
	}
	return limitResults(out, q.Count), nil
}

// Healthy 实现 Provider：GET / 探活。
func (p *SearXNGProvider) Healthy(ctx context.Context) error {
	if strings.TrimSpace(p.BaseURL) == "" {
		return &APIError{Kind: KindDisabled, Message: "searxng: empty base url"}
	}
	hc := p.HTTP
	if hc == nil {
		hc = netx.Client(defaultHTTPTimeout)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.BaseURL, "/")+"/", nil)
	resp, err := hc.Do(req)
	if err != nil {
		return transportErr("searxng", err)
	}
	resp.Body.Close()
	return nil
}

// searxngTimeRange 把通用时效映射到 SearXNG time_range（day/month/year）。
func searxngTimeRange(rec string) string {
	switch rec {
	case "pd":
		return "day"
	case "pw":
		return "month" // SearXNG 无周粒度，近似到月
	case "pm":
		return "month"
	case "py":
		return "year"
	default:
		return ""
	}
}

// limitResults 截断到 count（<=0 不截断）。
func limitResults(rs []Result, count int) []Result {
	n := normalizeCount(count)
	if len(rs) > n {
		return rs[:n]
	}
	return rs
}

// transportErr 归类传输错误。
func transportErr(provider string, err error) *APIError {
	msg := provider + ": " + err.Error()
	if isTimeout(err) {
		return &APIError{Kind: KindTimeout, Message: msg}
	}
	return &APIError{Kind: KindNetwork, Message: msg}
}

// isTimeout 判定超时（避免额外 import net 判型，做字符串+接口双判）。
func isTimeout(err error) bool {
	type timeouter interface{ Timeout() bool }
	if t, ok := err.(timeouter); ok && t.Timeout() {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "timeout") || strings.Contains(s, "deadline exceeded")
}

// String 便于日志。
func (p *SearXNGProvider) String() string { return fmt.Sprintf("searxng(%s)", p.BaseURL) }
