package mobilebind

// 多协议统一入口（T-新增）：让聊天/工具循环能在 openai / anthropic / gemini 三家间切换。
// 三家 adapter 的 Chat 签名一致（ctx, ChatRequest, emit）→ (*ChatResult, error)，
// 此处用统一 sender 抽象屏蔽各自 ChatRequest/ToolDef 差异。

import (
	"context"
	"strings"
	"sync"

	"star-river/kernel/adapter/anthropic"
	"star-river/kernel/adapter/gemini"
	"star-river/kernel/adapter/openai"
	"star-river/kernel/core/types"
)

var (
	provMu    sync.RWMutex
	provName  = "openai" // 默认 openai 兼容
	kernelVer = kernelVersion
)

// SetChatProvider 设置聊天 provider："openai"(默认) | "anthropic" | "gemini"。返回 "OK"/"ERR:..."。
func SetChatProvider(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "openai", "openai-compatible", "":
		n = "openai"
	case "anthropic", "claude":
		n = "anthropic"
	case "gemini", "google":
		n = "gemini"
	default:
		return "ERR:unknown provider: " + name
	}
	provMu.Lock()
	provName = n
	provMu.Unlock()
	return "OK"
}

// ChatProvider 当前 provider 名。
func ChatProvider() string {
	provMu.RLock()
	defer provMu.RUnlock()
	return provName
}

// chatSend 统一发送函数：入参用 openai.ToolDef 作规范工具定义；返回应答模型名。
type chatSend func(ctx context.Context, model string, msgs []types.Message, defs []openai.ToolDef, emit func(types.StreamEvent)) (string, error)

// buildSender 依当前 provider 构造 sender。
func buildSender(baseURL, apiKey string) chatSend {
	switch ChatProvider() {
	case "anthropic":
		c := anthropic.New(baseURL, apiKey)
		return func(ctx context.Context, model string, msgs []types.Message, defs []openai.ToolDef, emit func(types.StreamEvent)) (string, error) {
			tds := make([]anthropic.ToolDef, 0, len(defs))
			for _, d := range defs {
				tds = append(tds, anthropic.ToolDef{Name: d.Name, Description: d.Description, Parameters: d.Parameters})
			}
			res, err := c.Chat(ctx, anthropic.ChatRequest{Model: model, Messages: msgs, MaxTokens: 1024, Tools: tds}, emit)
			if err != nil {
				return "", err
			}
			return res.Model, nil
		}
	case "gemini":
		c := gemini.New(baseURL, apiKey)
		return func(ctx context.Context, model string, msgs []types.Message, defs []openai.ToolDef, emit func(types.StreamEvent)) (string, error) {
			tds := make([]gemini.ToolDef, 0, len(defs))
			for _, d := range defs {
				tds = append(tds, gemini.ToolDef{Name: d.Name, Description: d.Description, Parameters: d.Parameters})
			}
			res, err := c.Chat(ctx, gemini.ChatRequest{Model: model, Messages: msgs, MaxTokens: 1024, Tools: tds}, emit)
			if err != nil {
				return "", err
			}
			return res.Model, nil
		}
	default: // openai
		c := openai.NewWithHeaders(baseURL, apiKey, currentHeaders())
		return func(ctx context.Context, model string, msgs []types.Message, defs []openai.ToolDef, emit func(types.StreamEvent)) (string, error) {
			res, err := c.Chat(ctx, openai.ChatRequest{Model: model, Messages: msgs, MaxTokens: 1024, Tools: defs}, emit)
			if err != nil {
				return "", err
			}
			return res.Model, nil
		}
	}
}
