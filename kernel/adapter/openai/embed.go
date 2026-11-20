package openai

// Embeddings：POST {BaseURL}/embeddings，返回单条输入的向量（OpenAI 兼容）。
// key 只经 Authorization 头（红线3）；响应体上限受 maxErrBody 约束无关，用独立限流。

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

const maxEmbedBytes = 8 << 20 // 8MiB 上限

type embedReq struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embedResp struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Embed 返回输入文本的嵌入向量。失败返回 *APIError（已脱敏）。
func (c *Client) Embed(ctx context.Context, model, input string) ([]float32, error) {
	body, err := json.Marshal(embedReq{Model: model, Input: input})
	if err != nil {
		return nil, &APIError{Kind: KindBadRequest, Message: err.Error()}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, &APIError{Kind: KindBadRequest, Message: redact(err.Error(), c.apiKey)}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	for k, v := range c.extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, transportError(err, c.apiKey)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, maxEmbedBytes))
	if resp.StatusCode != http.StatusOK {
		return nil, apiErrorFromBody(resp.StatusCode, resp.Header, b, c.apiKey)
	}
	var out embedResp
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, &APIError{Kind: KindProtocol, Message: "bad embed json: " + err.Error()}
	}
	if out.Error != nil {
		return nil, &APIError{Kind: KindProtocol, Message: out.Error.Message}
	}
	if len(out.Data) == 0 || len(out.Data[0].Embedding) == 0 {
		return nil, &APIError{Kind: KindProtocol, Message: "empty embedding"}
	}
	return out.Data[0].Embedding, nil
}
