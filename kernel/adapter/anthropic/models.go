package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
)

// ModelInfo /v1/models 条目。id 为请求用模型名；display_name 供展示；
// created_at 为 RFC3339 字符串（Anthropic 原样，非 Unix 秒）；type 恒为 "model"。
type ModelInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
	CreatedAt   string `json:"created_at"`
}

// Models 拉取模型列表（GET /models?limit=1000）。超时由 Client.HTTP.Timeout 与 ctx
// 双重控制，超时/网络失败按 P6 有限重试后返回分类错误（T-M1-06 据此降级手动输入）。
// Anthropic 分页用 after_id/has_more；v1 单页拉满 1000，够 v1 全部模型，翻页留待需要。
func (c *Client) Models(ctx context.Context) ([]ModelInfo, error) {
	resp, err := c.do(ctx, http.MethodGet, "/models?limit=1000", "application/json", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, transportError(err, c.apiKey)
	}
	var w struct {
		Data []ModelInfo `json:"data"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return nil, &APIError{Kind: KindProtocol, RequestID: requestID(resp.Header), Message: "models response not JSON: " + redact(string(b), c.apiKey)}
	}
	if w.Data == nil {
		w.Data = []ModelInfo{}
	}
	return w.Data, nil
}
