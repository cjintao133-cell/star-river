package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
)

// ModelInfo /models 条目。created 为端点原样 Unix 秒（非本内核 UnixMilli 契约，
// 仅透传展示，入库时由 T-M1-06 换算）。
type ModelInfo struct {
	ID      string `json:"id"`
	OwnedBy string `json:"owned_by"`
	Created int64  `json:"created"`
}

// Models 拉取模型列表。超时由 Client.HTTP.Timeout 与 ctx 双重控制，
// 超时/网络失败按 P6 有限重试后返回分类错误（T-M1-06 据此降级手动输入）。
func (c *Client) Models(ctx context.Context) ([]ModelInfo, error) {
	resp, err := c.do(ctx, http.MethodGet, "/models", "application/json", nil)
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
