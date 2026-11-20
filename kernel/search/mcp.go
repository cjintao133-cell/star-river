package search

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cjintao133-cell/star-river/kernel/tool"
)

// ToolCaller 抽象「调用某个 MCP 搜索工具」的能力（*tool.Client 即满足）。
type ToolCaller interface {
	CallTool(ctx context.Context, name string, args map[string]any) (*tool.CallResult, error)
}

// MCPProvider 通用「搜索类 MCP」适配器。
//
// 任何把搜索暴露成工具的 MCP server（Tavily/Exa/Brave/Firecrawl/Perplexity/DuckDuckGo/百度 等搜索 MCP）
// 都可用本适配器接入：填 ServerName + ToolName + 一个 ToolCaller（kernel/tool 的 MCP 客户端）；
// 结果文本默认按常见 JSON 结构解析，必要时用 Parse 覆盖。
type MCPProvider struct {
	ServerName string // 显示名（如 "baidu-mcp"）
	ToolName   string // 搜索工具名（如 "search"/"web_search"/"baidu_search"）
	Caller     ToolCaller
	Parse      func(text string) ([]Result, error) // 可选；nil 用 DefaultParseMCPResults
}

// Name 实现 Provider。
func (p *MCPProvider) Name() string {
	if p.ServerName != "" {
		return "mcp:" + p.ServerName
	}
	return "mcp:" + p.ToolName
}

// Caps 实现 Provider：搜索 MCP 能力各异，保守声明（缩略图不一定有）。
func (p *MCPProvider) Caps() Capabilities {
	return Capabilities{HasThumbnail: false, SupportsRecency: false, SupportsChinese: false}
}

// Search 实现 Provider。
func (p *MCPProvider) Search(ctx context.Context, q Query) ([]Result, error) {
	if p.Caller == nil || strings.TrimSpace(p.ToolName) == "" {
		return nil, &APIError{Kind: KindDisabled, Message: "mcp search: caller/tool not configured"}
	}
	args := map[string]any{
		"query": q.Text,
		"count": normalizeCount(q.Count),
	}
	if q.Lang != "" {
		args["language"] = q.Lang
	}
	if q.Recency != "" {
		args["freshness"] = q.Recency
	}
	res, err := p.Caller.CallTool(ctx, p.ToolName, args)
	if err != nil {
		return nil, &APIError{Kind: KindNetwork, Message: "mcp search: " + err.Error()}
	}
	if res == nil {
		return nil, &APIError{Kind: KindProtocol, Message: "mcp search: nil result"}
	}
	if res.IsError {
		return nil, &APIError{Kind: KindProtocol, Message: "mcp search tool error: " + joinText(res.Content)}
	}
	text := joinText(res.Content)
	parse := p.Parse
	if parse == nil {
		parse = DefaultParseMCPResults
	}
	rs, err := parse(text)
	if err != nil {
		return nil, &APIError{Kind: KindProtocol, Message: "mcp search parse: " + err.Error()}
	}
	for i := range rs {
		if rs[i].Source == "" {
			rs[i].Source = p.Name()
		}
	}
	return limitResults(rs, q.Count), nil
}

// Healthy 实现 Provider：配置齐全即视为可用（真正连通性在调用时暴露）。
func (p *MCPProvider) Healthy(ctx context.Context) error {
	if p.Caller == nil || strings.TrimSpace(p.ToolName) == "" {
		return &APIError{Kind: KindDisabled, Message: "mcp search: not configured"}
	}
	return nil
}

// joinText 拼接工具结果里的全部文本块。
func joinText(cs []tool.Content) string {
	var b strings.Builder
	for _, c := range cs {
		if c.Text != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(c.Text)
		}
	}
	return b.String()
}

// DefaultParseMCPResults 解析常见搜索 MCP 的 JSON 文本结果。
// 支持：顶层数组、{results:[...]}、{items:[...]}、{data:[...]}、{web:{results:[...]}}。
// 条目字段兼容 title/url/description|snippet|content/thumbnail|img|image|img_src。
func DefaultParseMCPResults(text string) ([]Result, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	// 先尝试宽松：找出第一个 JSON 值（容忍前后有说明文字）。
	jsonStr := extractJSON(text)
	if jsonStr == "" {
		return nil, &APIError{Kind: KindProtocol, Message: "no JSON in tool output"}
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &arr); err == nil {
		return mapItems(arr), nil
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &obj); err != nil {
		return nil, &APIError{Kind: KindProtocol, Message: "not JSON object/array"}
	}
	if items := findItemsArray(obj); items != nil {
		return mapItems(items), nil
	}
	return nil, &APIError{Kind: KindProtocol, Message: "no results array in JSON"}
}

// findItemsArray 在常见键下找结果数组。
func findItemsArray(obj map[string]any) []map[string]any {
	keys := []string{"results", "items", "data", "web"}
	for _, k := range keys {
		v, ok := obj[k]
		if !ok {
			continue
		}
		if arr, ok := v.([]any); ok {
			return toMapSlice(arr)
		}
		if m, ok := v.(map[string]any); ok { // {web:{results:[...]}}
			if sub, ok := m["results"].([]any); ok {
				return toMapSlice(sub)
			}
		}
	}
	return nil
}

func toMapSlice(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, v := range arr {
		if m, ok := v.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// mapItems 把通用 map 条目映射为 Result。
func mapItems(items []map[string]any) []Result {
	out := make([]Result, 0, len(items))
	for _, m := range items {
		r := Result{
			Title:     str(m, "title", "name"),
			URL:       str(m, "url", "link", "href"),
			Snippet:   str(m, "snippet", "description", "content", "summary"),
			Thumbnail: str(m, "thumbnail", "img", "image", "img_src", "cover"),
			Favicon:   str(m, "favicon", "icon"),
		}
		if r.Title == "" && r.URL == "" {
			continue
		}
		out = append(out, r)
	}
	return out
}

// str 取第一个非空字符串字段。
func str(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// extractJSON 截取第一个 '{' 或 '[' 到匹配结尾的 JSON 片段（容忍前后说明文字）。
func extractJSON(s string) string {
	start := -1
	var open, close byte
	for i := 0; i < len(s); i++ {
		if s[i] == '{' || s[i] == '[' {
			start = i
			open = s[i]
			if open == '{' {
				close = '}'
			} else {
				close = ']'
			}
			break
		}
	}
	if start < 0 {
		return ""
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}
