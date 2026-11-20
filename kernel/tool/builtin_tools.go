package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// —— memory_query ——

// MemoryQueryTool 内置工具：查自身记忆（读 hippocampus，经 MemorySource 注入）。
type MemoryQueryTool struct {
	src   MemorySource
	limit int // 默认返回条数上限（<=0 用 5）
}

// NewMemoryQueryTool 建 memory_query 工具；src 为 nil 表示未配置（工具仍定义，调用报未配置）。
func NewMemoryQueryTool(src MemorySource) *MemoryQueryTool {
	return &MemoryQueryTool{src: src, limit: 5}
}

// Name 实现 BuiltinTool。
func (t *MemoryQueryTool) Name() string { return "memory_query" }

// Definition 实现 BuiltinTool。
func (t *MemoryQueryTool) Definition() Tool {
	return Tool{
		Name:        "memory_query",
		Title:       "查询自身长期记忆",
		Description: "按语义相关度检索自己的长期记忆，用于回忆过去与用户的相关对话与事实。",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"检索关键词或自然语言描述"},"limit":{"type":"integer","description":"返回条数上限（默认5）","minimum":1,"maximum":20}},"required":["query"]}`),
	}
}

// Execute 实现 BuiltinTool。
func (t *MemoryQueryTool) Execute(_ context.Context, args json.RawMessage) (*CallResult, error) {
	if t.src == nil {
		return nil, &APIError{Kind: KindProtocol, Message: "memory_query: memory source not configured"}
	}
	var p struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, &APIError{Kind: KindProtocol, Message: "memory_query: bad arguments: " + err.Error()}
		}
	}
	if strings.TrimSpace(p.Query) == "" {
		return nil, &APIError{Kind: KindProtocol, Message: "memory_query: query required"}
	}
	limit := p.Limit
	if limit <= 0 {
		limit = t.limit
	}
	if limit > 20 {
		limit = 20
	}
	hits := t.src.Query(p.Query, limit)
	if len(hits) == 0 {
		return textResult("（没有找到相关记忆）"), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "找到 %d 条相关记忆：\n", len(hits))
	for _, h := range hits {
		fmt.Fprintf(&b, "- [%.2f] %s\n", h.Score, h.Text)
	}
	return textResult(strings.TrimRight(b.String(), "\n")), nil
}

// —— mood_report ——

// MoodReportTool 内置工具：情绪自述（读 endocrine，经 MoodSource 注入）。
type MoodReportTool struct {
	src MoodSource
}

// NewMoodReportTool 建 mood_report 工具。
func NewMoodReportTool(src MoodSource) *MoodReportTool { return &MoodReportTool{src: src} }

// Name 实现 BuiltinTool。
func (t *MoodReportTool) Name() string { return "mood_report" }

// Definition 实现 BuiltinTool。
func (t *MoodReportTool) Definition() Tool {
	return Tool{
		Name:        "mood_report",
		Title:       "情绪状态自述",
		Description: "读取自己当前各情绪维度的数值（效价/唤醒/亲密/疲劳/好奇等），用于如实自述当前心情。",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}
}

// Execute 实现 BuiltinTool。
func (t *MoodReportTool) Execute(_ context.Context, _ json.RawMessage) (*CallResult, error) {
	if t.src == nil {
		return nil, &APIError{Kind: KindProtocol, Message: "mood_report: mood source not configured"}
	}
	st := t.src.Snapshot()
	if len(st) == 0 {
		return textResult("（暂无情绪数据）"), nil
	}
	var b strings.Builder
	b.WriteString("当前情绪状态：\n")
	for _, k := range sortedKeys(st) {
		fmt.Fprintf(&b, "- %s: %.2f\n", k, st[k])
	}
	return textResult(strings.TrimRight(b.String(), "\n")), nil
}

// —— visual_request ——

// VisualRequestTool 内置工具：发起图像生成（经 VisualRequester 注入）。
type VisualRequestTool struct {
	req VisualRequester
}

// NewVisualRequestTool 建 visual_request 工具。
func NewVisualRequestTool(req VisualRequester) *VisualRequestTool {
	return &VisualRequestTool{req: req}
}

// Name 实现 BuiltinTool。
func (t *VisualRequestTool) Name() string { return "visual_request" }

// Definition 实现 BuiltinTool。
func (t *VisualRequestTool) Definition() Tool {
	return Tool{
		Name:        "visual_request",
		Title:       "发起图像生成",
		Description: "根据文本描述发起一次图像生成任务（异步），返回任务 id。生成物将带不可关闭的 AI 标识水印。",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"prompt":{"type":"string","description":"画面描述"},"params":{"type":"object","description":"可调参数（如尺寸/垫图强度），可选"}},"required":["prompt"]}`),
	}
}

// Execute 实现 BuiltinTool。
func (t *VisualRequestTool) Execute(ctx context.Context, args json.RawMessage) (*CallResult, error) {
	if t.req == nil {
		return nil, &APIError{Kind: KindProtocol, Message: "visual_request: visual backend not configured"}
	}
	var p struct {
		Prompt string         `json:"prompt"`
		Params map[string]any `json:"params"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, &APIError{Kind: KindProtocol, Message: "visual_request: bad arguments: " + err.Error()}
		}
	}
	if strings.TrimSpace(p.Prompt) == "" {
		return nil, &APIError{Kind: KindProtocol, Message: "visual_request: prompt required"}
	}
	jobID, err := t.req.Request(ctx, p.Prompt, p.Params)
	if err != nil {
		return nil, &APIError{Kind: KindNetwork, Message: "visual_request: " + err.Error()}
	}
	return textResult(fmt.Sprintf("已发起图像生成，任务 id=%s（异步，稍后查看画廊）", jobID)), nil
}

// textResult 构造纯文本 CallResult。
func textResult(s string) *CallResult {
	return &CallResult{Content: []Content{{Type: "text", Text: s}}}
}
