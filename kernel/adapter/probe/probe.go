// Package probe 是模型能力的「实测探针」信号层（协议无关）。
//
// 各协议适配器实现 Sender：发几次极短探测请求，回报 Signal（是否流式/收图/收工具/向量/推理）。
// 本包只把 Signal 映射为 types.ModelCapabilities；不含 HTTP 细节、不依赖任何协议包或 discovery，
// 便于上层（integration）把探测结果喂给 discovery.BuildCard 或经 Prober 落台账。
package probe

import (
	"context"

	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

// Signal 一次探测得到的原始信号。
type Signal struct {
	Streamed      bool // 流式可用（SSE）
	AcceptedImage bool // 接受图片输入（未明确拒绝图）
	AcceptedTools bool // 接受 tools 参数（能传工具≠会用，仅"接受"）
	EmbeddingOK   bool // /embeddings 可用
	SawReasoning  bool // 响应含推理片段（reasoning_content/thinking）
}

// Sender 发探测请求并回报信号。各协议适配器实现。
type Sender interface {
	Send(ctx context.Context, modelID string) (Signal, error)
}

// SenderFunc 函数适配。
type SenderFunc func(ctx context.Context, modelID string) (Signal, error)

// Send 实现 Sender。
func (f SenderFunc) Send(ctx context.Context, modelID string) (Signal, error) { return f(ctx, modelID) }

// Caps 把探测信号映射为能力位。
func Caps(modelID string, s Signal) types.ModelCapabilities {
	return types.ModelCapabilities{
		ModelID:   modelID,
		Vision:    s.AcceptedImage,
		Tools:     s.AcceptedTools,
		Reasoning: s.SawReasoning,
		Embedding: s.EmbeddingOK,
		Streaming: s.Streamed,
	}
}

// AsProbeFn 把 Sender 包成 `func(ctx,model)(types.ModelCapabilities,error)`，
// 可直接用于 discovery.ProberFunc(sender) 落台账。不 import discovery，避免包环。
func AsProbeFn(s Sender) func(ctx context.Context, modelID string) (types.ModelCapabilities, error) {
	return func(ctx context.Context, modelID string) (types.ModelCapabilities, error) {
		sig, err := s.Send(ctx, modelID)
		if err != nil {
			return types.ModelCapabilities{}, err
		}
		return Caps(modelID, sig), nil
	}
}
