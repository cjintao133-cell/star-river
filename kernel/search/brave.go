package search

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/cjintao133-cell/star-river/kernel/netx"
)

// BraveProvider Brave Search API 适配器（GET /res/v1/web/search）。
type BraveProvider struct {
	APIKey  string
	BaseURL string // 默认 https://api.search.brave.com；测试可覆盖
	HTTP    *http.Client
}

// Name 实现 Provider。
func (p *BraveProvider) Name() string { return "brave" }

// Caps 实现 Provider。
func (p *BraveProvider) Caps() Capabilities {
	return Capabilities{HasThumbnail: true, SupportsRecency: false, SupportsChinese: false}
}

type braveResp struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
			Age         string `json:"age"`
			Profile     struct {
				Img string `json:"img"`
			} `json:"profile"`
			Thumbnail struct {
				Src string `json:"src"`
			} `json:"thumbnail"`
		} `json:"results"`
	} `json:"web"`
}

// Search 实现 Provider。
func (p *BraveProvider) Search(ctx context.Context, q Query) ([]Result, error) {
	if strings.TrimSpace(p.APIKey) == "" {
		return nil, &APIError{Kind: KindDisabled, Message: "brave: empty api key"}
	}
	base := p.BaseURL
	if base == "" {
		base = "https://api.search.brave.com"
	}
	u, err := url.Parse(strings.TrimRight(base, "/") + "/res/v1/web/search")
	if err != nil {
		return nil, &APIError{Kind: KindProtocol, Message: "brave: bad url: " + err.Error()}
	}
	vals := u.Query()
	vals.Set("q", q.Text)
	vals.Set("count", strconv.Itoa(normalizeCount(q.Count)))
	if q.Lang != "" {
		vals.Set("search_lang", q.Lang)
	}
	if q.Recency != "" {
		vals.Set("freshness", braveFreshness(q.Recency))
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
	req.Header.Set("X-Subscription-Token", p.APIKey)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, transportErr("brave", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, classifyStatus(resp.StatusCode, "brave: "+string(body))
	}
	var r braveResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, &APIError{Kind: KindProtocol, Message: "brave: bad json: " + err.Error()}
	}
	out := make([]Result, 0, len(r.Web.Results))
	for _, it := range r.Web.Results {
		out = append(out, Result{
			Title: it.Title, URL: it.URL, Snippet: it.Description,
			Thumbnail: it.Thumbnail.Src, Favicon: it.Profile.Img,
			PublishedAt: it.Age, Source: "brave",
		})
	}
	return limitResults(out, q.Count), nil
}

// Healthy 实现 Provider：API key 非空即视为配置可用（不额外联网探活，省额度）。
func (p *BraveProvider) Healthy(ctx context.Context) error {
	if strings.TrimSpace(p.APIKey) == "" {
		return &APIError{Kind: KindDisabled, Message: "brave: empty api key"}
	}
	return nil
}

// braveFreshness 映射 Brave freshness（pd/pw/pm/py 通用集）。
func braveFreshness(rec string) string {
	switch rec {
	case "pd", "pw", "pm", "py":
		return rec
	default:
		return ""
	}
}
