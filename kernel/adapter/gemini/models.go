package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// ModelInfo /v1beta/models 条目。Name 形如 "models/gemini-1.5-pro"；
// SupportedGenerationMethods 决定能力（含 generateContent 才是聊天模型）。
type ModelInfo struct {
	Name                       string   `json:"name"`
	DisplayName                string   `json:"displayName"`
	Description                string   `json:"description"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
}

// ID 返回去掉 "models/" 前缀的模型名（请求直接可用）。
func (m ModelInfo) ID() string {
	return strings.TrimPrefix(m.Name, "models/")
}

// SupportsGenerateContent 报告该模型是否支持聊天生成（supportedGenerationMethods 含 generateContent）。
func (m ModelInfo) SupportsGenerateContent() bool {
	for _, s := range m.SupportedGenerationMethods {
		if s == "generateContent" || s == "streamGenerateContent" {
			return true
		}
	}
	return false
}

// Models 拉取全部模型（GET /models）。翻页只取首页（pageSize=1000 覆盖 v1 全部模型）。
// 超时由 Client.HTTP.Timeout 与 ctx 双重控制，超时/网络失败按 P6 有限重试后返回分类错误。
func (c *Client) Models(ctx context.Context) ([]ModelInfo, error) {
	resp, err := c.do(ctx, http.MethodGet, "/models?pageSize=1000", "application/json", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, transportError(err, c.apiKey)
	}
	var w struct {
		Models []ModelInfo `json:"models"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return nil, &APIError{Kind: KindProtocol, RequestID: requestID(resp.Header), Message: "models response not JSON: " + redact(string(b), c.apiKey)}
	}
	if w.Models == nil {
		w.Models = []ModelInfo{}
	}
	return w.Models, nil
}

// ChatModels 拉取模型并只保留支持聊天生成的（supportedGenerationMethods 含 generateContent）。
// T-M1-05 验收："按 supportedGenerationMethods 过滤聊天模型"。
func (c *Client) ChatModels(ctx context.Context) ([]ModelInfo, error) {
	all, err := c.Models(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ModelInfo, 0, len(all))
	for _, m := range all {
		if m.SupportsGenerateContent() {
			out = append(out, m)
		}
	}
	return out, nil
}
