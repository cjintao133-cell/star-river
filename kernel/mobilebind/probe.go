package mobilebind

// T-M2-05：端点探测。设置页"校验"用——发一次极短请求确认端点/模型/key 可用，
// 失败按分类（auth/rate_limit/timeout/...）+ request id 回传（增补 P4/T-M2-05 追加），
// 让用户自助排障。返回 JSON 字符串。

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/cjintao133-cell/star-river/kernel/adapter/openai"
	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

// ProbeResult 端点探测结果（JSON 序列化给 UI）。
type ProbeResult struct {
	OK        bool   `json:"ok"`
	Model     string `json:"model,omitempty"`
	Category  string `json:"category,omitempty"`  // 失败分类（openai Kind）
	RequestID string `json:"requestId,omitempty"` // 排障 id
	Error     string `json:"error,omitempty"`     // 脱敏错误摘要
}

// ProbeEndpoint 发极短对话验证端点+模型+key。成功回 ok+实际模型；
// 失败回 ok=false + 分类 + request id + 脱敏错误。JSON 字符串。
func ProbeEndpoint(baseURL, apiKey, model string) string {
	res := ProbeResult{}
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(model) == "" {
		res.OK = false
		res.Category = "bad_request"
		res.Error = "endpoint/model 未配置"
		return mustJSON(res)
	}
	c := openai.New(baseURL, apiKey)
	c.MaxRetries = 0 // 校验不重试，快速反馈
	req := openai.ChatRequest{
		Model:     model,
		MaxTokens: 8,
		Messages:  []types.Message{{Role: types.RoleUser, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: "hi"}}}},
	}
	r, err := c.Chat(context.Background(), req, func(types.StreamEvent) {})
	if err != nil {
		res.OK = false
		res.Category = "unknown"
		var ae *openai.APIError
		if errors.As(err, &ae) {
			res.Category = ae.Kind
			res.RequestID = ae.RequestID
		}
		res.Error = redactKey(err.Error(), apiKey)
		return mustJSON(res)
	}
	res.OK = true
	res.Model = r.Model
	return mustJSON(res)
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"ok":false,"error":"encode"}`
	}
	return string(b)
}
