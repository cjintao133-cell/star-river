package tooldisc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"star-river/kernel/tool"
)

// SearchToolName tool_search 元工具名。
const SearchToolName = "tool_search"

// SearchTool 「tool_search」内置元工具：在工具注册表里按关键词检索，返回**精简目录**
// （server/name/短描述），**不含完整 schema**。模型据此决定要不要调用某工具，再按名懒拉 schema。
type SearchTool struct {
	ix    *Index
	limit int
}

// NewSearchTool 建 tool_search；limit<=0 默认 10（目录条目数，远小于工具总数）。
func NewSearchTool(ix *Index) *SearchTool { return &SearchTool{ix: ix, limit: 10} }

// Name 实现 tool.BuiltinTool。
func (t *SearchTool) Name() string { return SearchToolName }

// Definition 实现 tool.BuiltinTool。
func (t *SearchTool) Definition() tool.Tool {
	return tool.Tool{
		Name:        SearchToolName,
		Title:       "搜索可用工具目录",
		Description: "按关键词搜索自己可用的工具目录，返回匹配的工具（服务器/名称/简述）。返回结果不含完整参数 schema；确定要调用某工具时再按其名称获取参数并调用。",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"关键词，匹配工具名或简述"},"category":{"type":"string","description":"可选分类过滤"},"limit":{"type":"integer","minimum":1,"maximum":50,"description":"返回条数上限，默认10"}},"required":["query"]}`),
	}
}

// Execute 实现 tool.BuiltinTool。
func (t *SearchTool) Execute(ctx context.Context, args json.RawMessage) (*tool.CallResult, error) {
	if t.ix == nil {
		return nil, fmt.Errorf("tool_search: index not configured")
	}
	var p struct {
		Query    string `json:"query"`
		Category string `json:"category"`
		Limit    int    `json:"limit"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, fmt.Errorf("tool_search: bad arguments: %w", err)
		}
	}
	limit := p.Limit
	if limit <= 0 {
		limit = t.limit
	}
	if limit > 50 {
		limit = 50
	}
	metas, err := t.ix.Search(ctx, p.Query, SearchOptions{Category: p.Category, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("tool_search: %w", err)
	}
	return &tool.CallResult{Content: []tool.Content{{Type: "text", Text: formatCatalog(metas)}}}, nil
}

// formatCatalog 把命中列表格式化为紧凑目录文本（每行一个，绝不含 schema）。
func formatCatalog(metas []ToolMeta) string {
	if len(metas) == 0 {
		return "（没有匹配的工具）"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "找到 %d 个工具（调用前再按名获取参数）：\n", len(metas))
	for _, m := range metas {
		display := m.Name
		if m.Title != "" {
			display = m.Name + "（" + m.Title + "）"
		}
		fmt.Fprintf(&b, "- [%s] %s — %s\n", m.Server, display, m.ShortDesc)
	}
	return strings.TrimRight(b.String(), "\n")
}

// FootprintTools 返回注入模型上下文的**固定小 footprint**：恒为 tool_search 一个元工具，
// 与注册工具总数无关（10 或 2000 都一样）。这是「两段式按需发现」的核心保证。
func FootprintTools(ix *Index) []tool.Tool {
	return []tool.Tool{NewSearchTool(ix).Definition()}
}

// EstimateTokens 粗略估算 token（≈ 字节数/4，向上取整）。用于配额断言，非精确计费。
func EstimateTokens(s string) int { return (len(s) + 3) / 4 }

// FootprintTokens 估算固定 footprint 的 token 占用（与工具总数无关）。
func FootprintTokens(ix *Index) int {
	total := 0
	for _, t := range FootprintTools(ix) {
		total += EstimateTokens(t.Name) + EstimateTokens(t.Description) + EstimateTokens(string(t.InputSchema))
	}
	return total
}
