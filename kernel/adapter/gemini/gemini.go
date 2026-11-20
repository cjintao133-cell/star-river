package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"star-river/kernel/core/types"
)

// Usage token 用量（T-M3-07 统计数据源）。与 openai 适配器同名，便于上层统一映射。
type Usage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

// ChatRequest v1 最小请求。P1：model/maxOutputTokens 显式；流式与否由端点方法
// （streamGenerateContent + alt=sse）决定，非请求体字段。System 为独立系统指令字段。
type ChatRequest struct {
	Model        string
	System       string
	Messages     []types.Message
	MaxTokens    int       // → generationConfig.maxOutputTokens，Gemini 必填
	NativeSearch bool      // 启用模型原生网页搜索（Gemini: tools googleSearch）
	Tools        []ToolDef // 客户端工具定义（functionDeclarations）
}

// ToolDef 客户端工具定义。Parameters 为 JSON Schema。
type ToolDef struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// ChatResult 一次成功调用的元数据。Model 为请求模型（Gemini 响应不回显 model，
// 无法做掉包检测；本层如实回填请求值）。FinishReason 为 Gemini 原生 stop 枚举
// （STOP/MAX_TOKENS/SAFETY/RECITATION/OTHER），原样透出。Degraded=true 表示 P3 降级。
type ChatResult struct {
	Model        string
	FinishReason string
	Usage        Usage
	Degraded     bool
	RequestID    string
	Sources      []Citation // 原生搜索引用来源（NativeSearch 时）
}

// Citation 一条搜索引用来源。
type Citation struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// collectGrounding 从 groundingMetadata 追加去重来源。
func collectGrounding(dst []Citation, gm *wireGroundingMetadata) []Citation {
	if gm == nil {
		return dst
	}
	for _, gc := range gm.GroundingChunks {
		if gc.Web.URI == "" {
			continue
		}
		dup := false
		for _, c := range dst {
			if c.URL == gc.Web.URI {
				dup = true
				break
			}
		}
		if !dup {
			dst = append(dst, Citation{URL: gc.Web.URI, Title: gc.Web.Title})
		}
	}
	return dst
}

// Client 并发安全：字段构造后只读，http.Client 自身并发安全。
// BaseURL 形如 https://generativelanguage.googleapis.com/v1beta（尾斜杠自动去除）。
type Client struct {
	BaseURL     string
	HTTP        *http.Client
	MaxRetries  int
	BackoffBase time.Duration

	apiKey string
	sleep  func(ctx context.Context, d time.Duration) error
}

// New 默认 60s 整体超时。key 只经 x-goog-api-key 头，绝不进 URL（红线 3）。
func New(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		HTTP:        &http.Client{Timeout: 60 * time.Second},
		MaxRetries:  2,
		BackoffBase: 500 * time.Millisecond,
		apiKey:      apiKey,
		sleep:       sleepCtx,
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// transportError 传输层错误分类；Message 脱敏（err 原文可能含 URL）。
func transportError(err error, apiKey string) *APIError {
	kind := KindNetwork
	var ne net.Error
	switch {
	case errors.Is(err, context.Canceled):
		kind = KindCanceled
	case errors.Is(err, context.DeadlineExceeded):
		kind = KindTimeout
	case errors.As(err, &ne) && ne.Timeout():
		kind = KindTimeout
	}
	return &APIError{Kind: kind, Message: redact(err.Error(), apiKey)}
}

func retryableKind(k string) bool {
	return k == KindNetwork || k == KindTimeout || k == KindServer || k == KindRateLimit
}

// 请求侧线格式。
type wireOutPart struct {
	Text             string            `json:"text,omitempty"`
	FunctionCall     *wireFunctionCall `json:"functionCall,omitempty"`
	FunctionResponse *wireFunctionResp `json:"functionResponse,omitempty"`
	InlineData       *wireInlineData   `json:"inlineData,omitempty"`
}

// wireInlineData Gemini 内联图片（base64）。
type wireInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type wireFunctionResp struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type wireOutContent struct {
	Role  string        `json:"role,omitempty"`
	Parts []wireOutPart `json:"parts"`
}

// toolResponse 把内部 tool_result 文本转成 Gemini functionResponse.response 对象。
// 文本本身是 JSON 对象则原样用；否则包成 {"result": text}（或 isError 时 {"error": text}）。
func toolResponse(text string, isError bool) json.RawMessage {
	if text != "" && json.Valid([]byte(text)) {
		var probe map[string]json.RawMessage
		if json.Unmarshal([]byte(text), &probe) == nil {
			return json.RawMessage(text)
		}
	}
	key := "result"
	if isError {
		key = "error"
	}
	b, _ := json.Marshal(map[string]string{key: text})
	return b
}

// convertMessages 契约类型 → Gemini 线格式。返回系统指令文本与 contents。
// 语义差异（相对 openai/anthropic）：
//   - system role 消息不进 contents，抽取合并到 systemInstruction；
//   - assistant → role "model"；user/tool → role "user"；
//   - tool_use → functionCall part；tool_result → functionResponse part（必须 user turn，
//     且需按 call_id 回查函数名——Gemini functionResponse 要 name，而契约 tool_result
//     只带 call_id，故本包按出现顺序维护 call_id→name 映射）；
//   - 相邻同 role 合并（Gemini 要求 user/model 交替）。
//
// v1 不支持的块（image/audio）显式报错，不静默丢弃。
func convertMessages(msgs []types.Message) (string, []wireOutContent, error) {
	var sysParts []string
	names := map[string]string{} // call_id → function name
	out := make([]wireOutContent, 0, len(msgs))
	for i, m := range msgs {
		if !m.Role.Valid() {
			return "", nil, &APIError{Kind: KindBadRequest, Message: fmt.Sprintf("messages[%d]: invalid role %q", i, m.Role)}
		}
		if m.Role == types.RoleSystem {
			for j, b := range m.Blocks {
				switch b.Type {
				case types.BlockText:
					sysParts = append(sysParts, b.Text)
				case types.BlockReasoning:
					// 推理过程不回传。
				default:
					return "", nil, &APIError{Kind: KindBadRequest, Message: fmt.Sprintf("messages[%d].blocks[%d]: system block type %q unsupported", i, j, b.Type)}
				}
			}
			continue
		}
		role := "user"
		if m.Role == types.RoleAssistant {
			role = "model"
		}
		parts := make([]wireOutPart, 0, len(m.Blocks))
		for j, b := range m.Blocks {
			switch b.Type {
			case types.BlockText:
				parts = append(parts, wireOutPart{Text: b.Text})
			case types.BlockReasoning:
				// 推理过程是端点产物，不回传。
			case types.BlockImage:
				if b.MIMEType == "" || len(b.Data) == 0 {
					return "", nil, &APIError{Kind: KindBadRequest, Message: fmt.Sprintf("messages[%d].blocks[%d]: image missing mime/data", i, j)}
				}
				parts = append(parts, wireOutPart{InlineData: &wireInlineData{MimeType: b.MIMEType, Data: base64.StdEncoding.EncodeToString(b.Data)}})
			case types.BlockToolUse:
				if b.Call == nil {
					return "", nil, &APIError{Kind: KindBadRequest, Message: fmt.Sprintf("messages[%d].blocks[%d]: tool_use without call", i, j)}
				}
				args := json.RawMessage(b.Call.Arguments)
				switch {
				case len(args) == 0:
					args = json.RawMessage("{}")
				case !json.Valid(args):
					return "", nil, &APIError{Kind: KindBadRequest, Message: fmt.Sprintf("messages[%d].blocks[%d]: tool_use arguments not valid JSON", i, j)}
				}
				parts = append(parts, wireOutPart{FunctionCall: &wireFunctionCall{Name: b.Call.Name, Args: args}})
				names[b.Call.ID] = b.Call.Name
			case types.BlockToolResult:
				if b.CallID == "" {
					return "", nil, &APIError{Kind: KindBadRequest, Message: fmt.Sprintf("messages[%d].blocks[%d]: tool_result without call_id", i, j)}
				}
				name, ok := names[b.CallID]
				if !ok {
					return "", nil, &APIError{Kind: KindBadRequest, Message: fmt.Sprintf("messages[%d].blocks[%d]: tool_result references unknown call_id %q", i, j, b.CallID)}
				}
				parts = append(parts, wireOutPart{FunctionResponse: &wireFunctionResp{Name: name, Response: toolResponse(b.Text, b.IsError)}})
				role = "user"
			default:
				return "", nil, &APIError{Kind: KindBadRequest, Message: fmt.Sprintf("messages[%d].blocks[%d]: block type %q unsupported in v1", i, j, b.Type)}
			}
		}
		if len(parts) == 0 {
			continue
		}
		if n := len(out); n > 0 && out[n-1].Role == role {
			out[n-1].Parts = append(out[n-1].Parts, parts...)
		} else {
			out = append(out, wireOutContent{Role: role, Parts: parts})
		}
	}
	var sys []string
	for _, s := range sysParts {
		if strings.TrimSpace(s) != "" {
			sys = append(sys, s)
		}
	}
	return strings.Join(sys, "\n"), out, nil
}

func joinSystem(a, b string) string {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "\n" + b
	}
}

// buildBody P1：generationConfig.maxOutputTokens 显式序列化；contents 始终在位。
func buildBody(req ChatRequest) ([]byte, error) {
	sysFromMsgs, contents, err := convertMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	body := struct {
		Contents          []wireOutContent `json:"contents"`
		SystemInstruction *wireContentIn   `json:"systemInstruction,omitempty"`
		GenerationConfig  struct {
			MaxOutputTokens int `json:"maxOutputTokens"`
		} `json:"generationConfig"`
		Tools []map[string]any `json:"tools,omitempty"`
	}{Contents: contents}
	body.GenerationConfig.MaxOutputTokens = req.MaxTokens
	if req.NativeSearch {
		body.Tools = append(body.Tools, map[string]any{"googleSearch": map[string]any{}})
	}
	if len(req.Tools) > 0 {
		decls := make([]map[string]any, 0, len(req.Tools))
		for _, td := range req.Tools {
			if td.Name == "" {
				continue
			}
			params := td.Parameters
			if len(params) == 0 {
				params = json.RawMessage(`{"type":"object","properties":{}}`)
			}
			decls = append(decls, map[string]any{"name": td.Name, "description": td.Description, "parameters": params})
		}
		if len(decls) > 0 {
			body.Tools = append(body.Tools, map[string]any{"functionDeclarations": decls})
		}
	}
	if sys := joinSystem(req.System, sysFromMsgs); sys != "" {
		body.SystemInstruction = &wireContentIn{Parts: []wireOutPart{{Text: sys}}}
	}
	return json.Marshal(body)
}

// wireContentIn systemInstruction 外壳（无 role）。
type wireContentIn struct {
	Parts []wireOutPart `json:"parts"`
}

// chatPath 依模型与流式与否拼路径。model 可带 "models/" 前缀，统一剥除。
// 流式：/models/{m}:streamGenerateContent?alt=sse；非流式：/models/{m}:generateContent。
// key 不进 URL（走头），故查询串只有 alt=sse。
func (c *Client) chatPath(model string, stream bool) string {
	m := url.PathEscape(strings.TrimPrefix(model, "models/"))
	if stream {
		return "/models/" + m + ":streamGenerateContent?alt=sse"
	}
	return "/models/" + m + ":generateContent"
}

// doOnce 单次请求：非 200 读错误体分类后关体；200 把响应体交还调用方。
// 头契约：x-goog-api-key 鉴权（无 Authorization）。
func (c *Client) doOnce(ctx context.Context, method, path, accept string, body []byte) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return nil, &APIError{Kind: KindBadRequest, Message: redact(err.Error(), c.apiKey)}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("x-goog-api-key", c.apiKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, transportError(err, c.apiKey)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2*maxErrBody))
		return nil, apiErrorFromBody(resp.StatusCode, resp.Header, b, c.apiKey)
	}
	return resp, nil
}

// do 带 P6 重试：只覆盖"响应体开始前"的失败（网络/超时/状态码），
// 流中途失败不在此层重试（避免向 UI 重复吐字）。429 尊重 Retry-After。
func (c *Client) do(ctx context.Context, method, path, accept string, body []byte) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		resp, err := c.doOnce(ctx, method, path, accept, body)
		if err == nil {
			return resp, nil
		}
		var ae *APIError
		if !errors.As(err, &ae) || attempt >= c.MaxRetries || !retryableKind(ae.Kind) {
			return nil, err
		}
		wait := c.BackoffBase << attempt
		if ae.RetryAfter > wait {
			wait = ae.RetryAfter
		}
		if serr := c.sleep(ctx, wait); serr != nil {
			return nil, err
		}
	}
}

// Chat 流式对话。emit 收归一化 StreamEvent；失败时必发 StreamError 事件
// 并返回 *APIError（不许静默成功）。P3：可判定的不支持流式 → 非流式重试一次，
// 成功则 ChatResult.Degraded=true。
func (c *Client) Chat(ctx context.Context, req ChatRequest, emit func(types.StreamEvent)) (*ChatResult, error) {
	switch {
	case req.Model == "":
		return nil, &APIError{Kind: KindBadRequest, Message: "model required"}
	case req.MaxTokens <= 0:
		return nil, &APIError{Kind: KindBadRequest, Message: "max_tokens must be positive"}
	case len(req.Messages) == 0:
		return nil, &APIError{Kind: KindBadRequest, Message: "messages required"}
	}
	res, err := c.chatOnce(ctx, req, true, emit)
	if err == nil {
		return res, nil
	}
	var ae *APIError
	if errors.As(err, &ae) && ae.Kind == KindStreamUnsupported {
		res, err = c.chatOnce(ctx, req, false, emit)
		if err == nil {
			res.Degraded = true
			return res, nil
		}
	}
	return nil, err
}

// chatOnce 一趟请求+消费。stream_unsupported 且还将降级时不发 error 事件
// （避免 UI 先见错误后见成功的假警报）；其余失败路径必发。
func (c *Client) chatOnce(ctx context.Context, req ChatRequest, stream bool, emit func(types.StreamEvent)) (*ChatResult, error) {
	body, err := buildBody(req)
	if err != nil {
		return nil, err
	}
	accept := "application/json"
	if stream {
		accept = "text/event-stream"
	}
	resp, err := c.do(ctx, http.MethodPost, c.chatPath(req.Model, stream), accept, body)
	if err != nil {
		var ae *APIError
		if errors.As(err, &ae) && ae.Kind == KindStreamUnsupported && stream {
			return nil, ae
		}
		c.emitError(emit, err)
		return nil, err
	}
	defer resp.Body.Close()
	rid := requestID(resp.Header)
	var res *ChatResult
	if stream {
		res, err = c.consumeStream(resp.Body, rid, req.Model, emit)
	} else {
		res, err = c.consumeJSON(resp.Body, rid, req.Model, emit)
	}
	if err != nil {
		var ae *APIError
		if !errors.As(err, &ae) {
			ae = transportError(err, c.apiKey)
		}
		if ae.RequestID == "" {
			ae.RequestID = rid
		}
		c.emitError(emit, ae)
		return nil, ae
	}
	return res, nil
}

func (c *Client) emitError(emit func(types.StreamEvent), err error) {
	var ae *APIError
	if !errors.As(err, &ae) {
		ae = &APIError{Kind: KindProtocol, Message: redact(err.Error(), c.apiKey)}
	}
	emit(types.StreamEvent{Kind: types.StreamError, ErrCode: ae.Kind, RequestID: ae.RequestID})
}

func usageFrom(w *wireUsageMetadata) Usage {
	if w == nil {
		return Usage{}
	}
	return Usage{PromptTokens: w.PromptTokenCount, CompletionTokens: w.CandidatesTokenCount, TotalTokens: w.TotalTokenCount}
}

// consumeStream 消费 Gemini alt=sse 流。终止判定（增补⑤）：必须见到 finishReason 才算成功；
// EOF 前未见 → 若为 promptFeedback 拦截则 KindProtocol，否则 KindStream 截断失败。
// thought=true 的 part 归 reasoning 事件；functionCall part 合成稳定 tool_call id。
func (c *Client) consumeStream(r io.Reader, rid, model string, emit func(types.StreamEvent)) (*ChatResult, error) {
	res := &ChatResult{RequestID: rid, Model: model}
	calls := make([]*types.ToolCall, 0)
	sawFinish := false
	blocked := ""
	err := scanSSE(r, c.apiKey, func(ch wireGenerateResponse) error {
		if ch.UsageMetadata != nil {
			res.Usage = usageFrom(ch.UsageMetadata)
		}
		if ch.PromptFeedback != nil && ch.PromptFeedback.BlockReason != "" {
			blocked = ch.PromptFeedback.BlockReason
		}
		if ch.GroundingMetadata != nil {
			res.Sources = collectGrounding(res.Sources, ch.GroundingMetadata)
		}
		for _, cand := range ch.Candidates {
			for _, p := range cand.Content.Parts {
				if p.FunctionCall != nil {
					args := "{}"
					if len(p.FunctionCall.Args) > 0 && string(p.FunctionCall.Args) != "null" {
						args = string(p.FunctionCall.Args)
					}
					calls = append(calls, &types.ToolCall{
						ID:        fmt.Sprintf("gemini-call-%d", len(calls)),
						Name:      p.FunctionCall.Name,
						Arguments: args,
					})
					continue
				}
				if p.Text != "" {
					if p.Thought {
						emit(types.StreamEvent{Kind: types.StreamReasoning, Text: p.Text})
					} else {
						emit(types.StreamEvent{Kind: types.StreamDelta, Text: p.Text})
					}
				}
			}
			if cand.FinishReason != "" {
				sawFinish = true
				res.FinishReason = cand.FinishReason
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !sawFinish {
		if blocked != "" {
			return nil, &APIError{Kind: KindProtocol, RequestID: res.RequestID, Code: blocked, Message: "prompt blocked: " + blocked}
		}
		return nil, &APIError{Kind: KindStream, RequestID: res.RequestID, Message: "stream ended without finishReason"}
	}
	for i, call := range calls {
		if err := call.Validate(); err != nil {
			return nil, &APIError{Kind: KindProtocol, RequestID: res.RequestID, Message: err.Error()}
		}
		emit(types.StreamEvent{Kind: types.StreamToolCall, Call: call, CallIndex: i})
	}
	emit(types.StreamEvent{Kind: types.StreamDone, Model: res.Model, RequestID: res.RequestID})
	return res, nil
}

// consumeJSON 非流式（P3 降级路径）。HTTP 200 带 error / 被拦截 / 缺 candidates /
// 缺 finishReason 均判失败，不许静默成功。
func (c *Client) consumeJSON(r io.Reader, rid, model string, emit func(types.StreamEvent)) (*ChatResult, error) {
	b, err := io.ReadAll(io.LimitReader(r, 8<<20))
	if err != nil {
		return nil, transportError(err, c.apiKey)
	}
	var w wireGenerateResponse
	if err := json.Unmarshal(b, &w); err != nil {
		return nil, &APIError{Kind: KindProtocol, RequestID: rid, Message: "response not JSON: " + redact(string(b), c.apiKey)}
	}
	if w.Error != nil {
		return nil, &APIError{Kind: KindProtocol, RequestID: rid, Code: w.Error.Status, Message: redact(w.Error.Message, c.apiKey)}
	}
	if len(w.Candidates) == 0 {
		if w.PromptFeedback != nil && w.PromptFeedback.BlockReason != "" {
			return nil, &APIError{Kind: KindProtocol, RequestID: rid, Code: w.PromptFeedback.BlockReason, Message: "prompt blocked: " + w.PromptFeedback.BlockReason}
		}
		return nil, &APIError{Kind: KindProtocol, RequestID: rid, Message: "response has no candidates"}
	}
	cand := w.Candidates[0]
	if cand.FinishReason == "" {
		return nil, &APIError{Kind: KindProtocol, RequestID: rid, Message: "missing finishReason"}
	}
	res := &ChatResult{Model: model, FinishReason: cand.FinishReason, RequestID: rid, Usage: usageFrom(w.UsageMetadata)}
	res.Sources = collectGrounding(res.Sources, w.GroundingMetadata)
	toolIdx := 0
	for _, p := range cand.Content.Parts {
		if p.FunctionCall != nil {
			args := "{}"
			if len(p.FunctionCall.Args) > 0 && string(p.FunctionCall.Args) != "null" {
				args = string(p.FunctionCall.Args)
			}
			call := &types.ToolCall{ID: fmt.Sprintf("gemini-call-%d", toolIdx), Name: p.FunctionCall.Name, Arguments: args}
			if err := call.Validate(); err != nil {
				return nil, &APIError{Kind: KindProtocol, RequestID: rid, Message: err.Error()}
			}
			emit(types.StreamEvent{Kind: types.StreamToolCall, Call: call, CallIndex: toolIdx})
			toolIdx++
			continue
		}
		if p.Text != "" {
			if p.Thought {
				emit(types.StreamEvent{Kind: types.StreamReasoning, Text: p.Text})
			} else {
				emit(types.StreamEvent{Kind: types.StreamDelta, Text: p.Text})
			}
		}
	}
	emit(types.StreamEvent{Kind: types.StreamDone, Model: res.Model, RequestID: rid})
	return res, nil
}
