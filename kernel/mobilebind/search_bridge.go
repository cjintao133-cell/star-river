package mobilebind

// 搜索桥（T-新增）：把 kernel/search 暴露给 UI。薄门面，JSON 进/出，错误前缀 "ERR:"。
// gomobile 类型限制：导出函数只用 string/int 等有限类型；结果以 JSON 字符串回传。

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/cjintao133-cell/star-river/kernel/search"
)

// searchResult 回传 UI 的搜索结果条目。
type searchResult struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Snippet   string `json:"snippet"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Favicon   string `json:"favicon,omitempty"`
	Source    string `json:"source,omitempty"`
}

// WebSearch 执行一次网页搜索。provider="searxng"（用 baseURL 自建实例）或 "brave"（用 apiKey）。
// 返回 JSON 数组字符串；失败返回 "ERR:<脱敏信息>"；空结果返回 "[]"。
func WebSearch(provider, baseURL, apiKey, query string, count int, recency string) string {
	if strings.TrimSpace(query) == "" {
		return "ERR:empty query"
	}
	var p search.Provider
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "searxng":
		p = &search.SearXNGProvider{BaseURL: baseURL}
	case "brave":
		p = &search.BraveProvider{APIKey: apiKey}
	default:
		return "ERR:unknown search provider: " + provider
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	rs, err := p.Search(ctx, search.Query{Text: query, Count: count, Recency: recency})
	if err != nil {
		return "ERR:" + redactKey(err.Error(), apiKey)
	}
	out := make([]searchResult, 0, len(rs))
	for _, r := range rs {
		out = append(out, searchResult{Title: r.Title, URL: r.URL, Snippet: r.Snippet, Thumbnail: r.Thumbnail, Favicon: r.Favicon, Source: r.Source})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "ERR:" + redactKey(err.Error(), apiKey)
	}
	return string(b)
}
