package tool

import (
	"context"
	"encoding/json"
	"sort"
	"sync"

	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

// —— 内置工具（T-M6-03）——
//
// 内置高频工具三件：memory_query（查自身记忆）、mood_report（情绪自述）、visual_request（发起生成）。
// 它们运行在进程内（区别于远程/stdio MCP），供"主脑"按需调用。
//
// 红线 2（依赖方向）：本包（tool）为内核底座的一部分，**不得 import 能力插件包**
// （hippocampus/endocrine/visual）。因此记忆/情绪/视觉能力一律以**接口注入**方式接入，
// 由应用装配层用各插件包实现这些接口后注入；tool 只认中性类型。

// MemoryHit 一条记忆命中（中性类型，避免依赖 hippocampus.Memory）。
type MemoryHit struct {
	ID    string  `json:"id"`
	Text  string  `json:"text"`
	Score float64 `json:"score"`
}

// MemorySource 记忆只读查询能力（应用层用 hippocampus 适配后注入）。
type MemorySource interface {
	// Query 返回与 query 相关的记忆命中（降序）；limit<=0 用实现默认。
	Query(query string, limit int) []MemoryHit
}

// MoodSource 情绪快照只读能力（应用层用 endocrine 适配后注入）。
// 返回维度 key→值（如 valence/arousal/intimacy/fatigue/curiosity）。
type MoodSource interface {
	Snapshot() map[string]float64
}

// VisualRequester 图像生成发起能力（应用层用 visual/生成适配器实现后注入）。
// 异步：返回任务 id。参数 params 为工具入参里的可调参数（可为 nil）。
type VisualRequester interface {
	Request(ctx context.Context, prompt string, params map[string]any) (jobID string, err error)
}

// BuiltinTool 进程内内置工具。
type BuiltinTool interface {
	Name() string
	Definition() Tool
	Execute(ctx context.Context, args json.RawMessage) (*CallResult, error)
}

// Registry 内置工具注册表 + 主脑调用分发器。并发安全。
type Registry struct {
	mu    sync.RWMutex
	tools map[string]BuiltinTool
	order []string
}

// NewRegistry 空注册表。
func NewRegistry() *Registry { return &Registry{tools: make(map[string]BuiltinTool)} }

// Register 注册内置工具（同名覆盖定义，不重复占位）。
func (r *Registry) Register(t BuiltinTool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tools[t.Name()]; !ok {
		r.order = append(r.order, t.Name())
	}
	r.tools[t.Name()] = t
}

// Definitions 返回全部工具定义（供拼入模型请求的 tools 列表；按注册序）。
func (r *Registry) Definitions() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tool, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, r.tools[n].Definition())
	}
	return out
}

// Names 返回已注册工具名（按注册序）。
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]string(nil), r.order...)
}

// Has 报告工具是否注册。
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.tools[name]
	return ok
}

// Dispatch 执行一次模型发起的工具调用（types.ToolCall），返回可回填给模型的 tool_result 内容块。
// 未知工具/参数非法/执行错误都转成 IsError=true 的结果块（协议成功、业务失败），不 panic。
func (r *Registry) Dispatch(ctx context.Context, call types.ToolCall) types.ContentBlock {
	if err := call.Validate(); err != nil {
		return toolResultBlock(call.ID, "invalid tool call: "+err.Error(), true)
	}
	r.mu.RLock()
	t := r.tools[call.Name]
	r.mu.RUnlock()
	if t == nil {
		return toolResultBlock(call.ID, "unknown tool: "+call.Name, true)
	}
	var raw json.RawMessage
	if call.Arguments != "" {
		raw = json.RawMessage(call.Arguments)
	}
	res, err := t.Execute(ctx, raw)
	if err != nil {
		return toolResultBlock(call.ID, err.Error(), true)
	}
	return toolResultBlock(call.ID, contentText(res), res != nil && res.IsError)
}

// DispatchAll 批量分发（按顺序）。
func (r *Registry) DispatchAll(ctx context.Context, calls []types.ToolCall) []types.ContentBlock {
	out := make([]types.ContentBlock, 0, len(calls))
	for _, c := range calls {
		out = append(out, r.Dispatch(ctx, c))
	}
	return out
}

// toolResultBlock 构造 tool_result 内容块（types 契约）。
func toolResultBlock(callID, text string, isErr bool) types.ContentBlock {
	return types.ContentBlock{Type: types.BlockToolResult, CallID: callID, Text: text, IsError: isErr}
}

// contentText 把 CallResult 的文本内容项拼接为单段文本（内置工具均为文本）。
func contentText(res *CallResult) string {
	if res == nil {
		return ""
	}
	out := ""
	for i, c := range res.Content {
		if c.Text == "" {
			continue
		}
		if i > 0 && out != "" {
			out += "\n"
		}
		out += c.Text
	}
	return out
}

// 便于测试/展示：排序 map key。
func sortedKeys(m map[string]float64) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
