package mobilebind

// 多协议统一入口（T-新增）：让聊天/工具循环能在 openai / anthropic / gemini 三家间切换。
// 三家 adapter 的 Chat 签名一致（ctx, ChatRequest, emit）→ (*ChatResult, error)，
// 此处用统一 sender 抽象屏蔽各自 ChatRequest/ToolDef 差异。

import (
	"context"
	"strings"
	"sync"

	"star-river/kernel/adapter/openai"
	"star-river/kernel/core/types"
)

var (
	provMu   sync.RWMutex
	provName = "openai" // 默认 openai 兼容
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

// buildSender 构造 sender：内部统一走 dispatch，并叠加 router 主备失败切换。
func buildSender(baseURL, apiKey string) chatSend {
	return func(ctx context.Context, model string, msgs []types.Message, defs []openai.ToolDef, emit func(types.StreamEvent)) (string, error) {
		return sendWithFailover(ctx, baseURL, apiKey, model, msgs, defs, emit)
	}
}
