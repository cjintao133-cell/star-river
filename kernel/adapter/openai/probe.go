package openai

import (
	"context"
	"encoding/json"
	"net/http"

	"star-river/kernel/adapter/probe"
)

// probePNG 1x1 透明 PNG（极短探测用）。
var probePNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

// SendProbe 实现对某模型的实测探测（OpenAI 兼容协议）。
// 发几次极短请求，汇总 probe.Signal：
//  1. stream:true 极短对话 → 连通 + 流式（失败=模型不可用→返回 err，致命）
//  2. 带 1x1 图 → 接受图片则 Vision（非致命）
//  3. 带一个空参工具定义 → 端点接受 tools 则 Tools（非致命）
//  4. /embeddings → Embedding（非致命）
//
// 注意：探测有真实成本，应仅在用户选中模型时懒调用，勿批量全探。
func (c *Client) SendProbe(ctx context.Context, modelID string) (probe.Signal, error) {
	var sig probe.Signal
	// 1) 连通 + 流式（致命：失败表示模型/端点不可用）
	if err := c.probeChat(ctx, modelID, false, &sig); err != nil {
		return sig, err
	}
	// 2) 图片（非致命）
	_ = c.probeChat(ctx, modelID, true, &sig)
	// 3) 工具（非致命）——接受 tools 参数即认为 Tools（不等同于会用）
	sig.AcceptedTools = c.probeTools(ctx, modelID)
	// 4) 向量（非致命）
	sig.EmbeddingOK = c.probeEmbedding(ctx, modelID)
	return sig, nil
}

// probeChat 发极短对话（可带图）。成功即 Streamed=true；带图成功即 AcceptedImage=true。
// withImage=false 时返回错误（致命连通性错误）。
func (c *Client) probeChat(ctx context.Context, modelID string, withImage bool, sig *probe.Signal) error {
	var content any = "hi"
	if withImage {
		content = []map[string]any{
			{"type": "text", "text": "what is this"},
			{"type": "image_url", "image_url": map[string]string{"url": "data:image/png;base64," + probePNG}},
		}
	}
	body, _ := json.Marshal(map[string]any{
		"model":      modelID,
		"max_tokens": 1,
		"stream":     true,
		"messages":   []map[string]any{{"role": "user", "content": content}},
	})
	resp, err := c.doOnce(ctx, http.MethodPost, "/chat/completions", "text/event-stream", body)
	if err != nil {
		if withImage {
			return nil // 带图失败视为不支持图片（非致命）
		}
		return err // 基础连通失败，致命
	}
	resp.Body.Close()
	sig.Streamed = true
	if withImage {
		sig.AcceptedImage = true
	}
	return nil
}

// probeTools 探端点是否接受 tools 参数（200 即接受）。
func (c *Client) probeTools(ctx context.Context, modelID string) bool {
	body, _ := json.Marshal(map[string]any{
		"model":      modelID,
		"max_tokens": 1,
		"messages":   []map[string]any{{"role": "user", "content": "hi"}},
		"tools": []map[string]any{{
			"type": "function",
			"function": map[string]any{
				"name": "noop", "description": "no-op",
				"parameters": map[string]any{"type": "object", "properties": map[string]any{}},
			},
		}},
	})
	resp, err := c.doOnce(ctx, http.MethodPost, "/chat/completions", "application/json", body)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

// probeEmbedding 探 /embeddings 是否可用。
func (c *Client) probeEmbedding(ctx context.Context, modelID string) bool {
	body, _ := json.Marshal(map[string]any{"model": modelID, "input": "hi"})
	resp, err := c.doOnce(ctx, http.MethodPost, "/embeddings", "application/json", body)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

// 编译期保证 *Client 可作为探测发送器（经 probe.SenderFunc 适配）。
var _ = probe.SenderFunc((*Client)(nil).SendProbe)
