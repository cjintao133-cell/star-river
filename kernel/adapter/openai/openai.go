package openai

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
	"sort"
	"strings"
	"time"

	"star-river/kernel/core/types"
)

// Usage token 用量（P4 / T-M3-07 统计数据源）。
type Usage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

// ChatRequest v1 最小请求。P1：stream/model/max_tokens 一律显式，不吃端点默认值。
// 请求侧 tools 定义推迟到 T-M6-01 接线；响应侧 tool_calls 解析本任务已实现。
type ChatRequest struct {
	Model        string
	Messages     []types.Message
	MaxTokens    int
	NativeSearch bool // 启用模型原生网页搜索（OpenAI: web_search_options + search-preview 模型）
}

// ChatResult 一次成功调用的元数据。Model 为实际应答模型（中转掉包信号，
// 与请求不一致时由 T-M1-06 在 UI 标记，本层只如实透出）。
// Degraded=true 表示 P3 降级：流式被明确拒绝后以 stream:false 重试成功，
// 调用方应把能力位 Streaming 记 false。
type ChatResult struct {
	Model        string
	FinishReason string
	Usage        Usage
	Degraded     bool
	RequestID    string
}

// Client 并发安全：字段构造后只读，http.Client 自身并发安全。
type Client struct {
	BaseURL     string        // 形如 https://host/v1，尾斜杠自动去除
	HTTP        *http.Client  // 超时在此配置（/models 与 chat 共用）
	MaxRetries  int           // 网络/超时/5xx/429 重试上限（P6，默认 2）
	BackoffBase time.Duration // 指数退避基数（默认 500ms，第 n 次重试等 base*2^n）

	apiKey string
	sleep  func(ctx context.Context, d time.Duration) error // 测试注入点
}

// New 默认 60s 整体超时。key 只经 Authorization 头，绝不进 URL（红线 3）。
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

// wireOutMessage 请求侧消息。reasoning 块不回传端点。
type wireOutMessage struct {
	Role       string            `json:"role"`
	Content    any               `json:"content"`
	ToolCalls  []wireOutToolCall `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
}

type wireOutToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// openAIOutPart OpenAI 多模态 content 分片（文本/图片）。
type openAIOutPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *openAIImageURL `json:"image_url,omitempty"`
}

type openAIImageURL struct {
	URL string `json:"url"`
}

// dataURI 把图片字节编码为 data URI（OpenAI image_url.url 用）。
func dataURI(mime string, data []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// convertMessages 契约类型 → 请求线格式。v1 不支持的块显式报错，不静默丢弃。
func convertMessages(msgs []types.Message) ([]wireOutMessage, error) {
	out := make([]wireOutMessage, 0, len(msgs))
	for i, m := range msgs {
		if !m.Role.Valid() {
			return nil, &APIError{Kind: KindBadRequest, Message: fmt.Sprintf("messages[%d]: invalid role %q", i, m.Role)}
		}
		w := wireOutMessage{Role: string(m.Role)}
		var text strings.Builder
		var images []openAIOutPart
		for j, b := range m.Blocks {
			switch b.Type {
			case types.BlockText:
				text.WriteString(b.Text)
			case types.BlockReasoning:
				// 推理过程是端点产物，不回传。
			case types.BlockImage:
				// 多模态视觉：图片 → content 数组里的 image_url(data URI)。
				if b.MIMEType == "" || len(b.Data) == 0 {
					return nil, &APIError{Kind: KindBadRequest, Message: fmt.Sprintf("messages[%d].blocks[%d]: image missing mime/data", i, j)}
				}
				images = append(images, openAIOutPart{Type: "image_url", ImageURL: &openAIImageURL{URL: dataURI(b.MIMEType, b.Data)}})
			case types.BlockToolUse:
				if b.Call == nil {
					return nil, &APIError{Kind: KindBadRequest, Message: fmt.Sprintf("messages[%d].blocks[%d]: tool_use without call", i, j)}
				}
				tc := wireOutToolCall{ID: b.Call.ID, Type: "function"}
				tc.Function.Name = b.Call.Name
				tc.Function.Arguments = b.Call.Arguments
				w.ToolCalls = append(w.ToolCalls, tc)
			case types.BlockToolResult:
				if w.ToolCallID != "" {
					return nil, &APIError{Kind: KindBadRequest, Message: fmt.Sprintf("messages[%d]: one tool_result per message", i)}
				}
				w.ToolCallID = b.CallID
				text.WriteString(b.Text)
			default:
				return nil, &APIError{Kind: KindBadRequest, Message: fmt.Sprintf("messages[%d].blocks[%d]: block type %q unsupported in v1", i, j, b.Type)}
			}
		}
		if len(images) > 0 {
			// 有图：content 用分片数组（文本段在前，图片随后）。
			arr := make([]openAIOutPart, 0, len(images)+1)
			if text.Len() > 0 {
				arr = append(arr, openAIOutPart{Type: "text", Text: text.String()})
			}
			arr = append(arr, images...)
			w.Content = arr
		} else {
			w.Content = text.String()
		}
		out = append(out, w)
	}
	return out, nil
}

// buildBody P1：四个关键字段全部显式序列化，无 omitempty。
func buildBody(req ChatRequest, stream bool) ([]byte, error) {
	msgs, err := convertMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	body := struct {
		Model            string           `json:"model"`
		Messages         []wireOutMessage `json:"messages"`
		MaxTokens        int              `json:"max_tokens"`
		Stream           bool             `json:"stream"`
		WebSearchOptions *struct{}        `json:"web_search_options,omitempty"`
	}{Model: req.Model, Messages: msgs, MaxTokens: req.MaxTokens, Stream: stream}
	if req.NativeSearch {
		body.WebSearchOptions = &struct{}{}
	}
	return json.Marshal(body)
}

// doOnce 单次请求：非 200 读错误体分类后关体；200 把响应体交还调用方。
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
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
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
// 并返回 *APIError（不许静默成功）。P3：可判定的不支持流式 → stream:false
// 重试一次，成功则 ChatResult.Degraded=true。
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
	body, err := buildBody(req, stream)
	if err != nil {
		return nil, err
	}
	accept := "application/json"
	if stream {
		accept = "text/event-stream"
	}
	resp, err := c.do(ctx, http.MethodPost, "/chat/completions", accept, body)
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
		res, err = c.consumeStream(resp.Body, rid, emit)
	} else {
		res, err = c.consumeJSON(resp.Body, rid, emit)
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

func usageFrom(w *wireUsage) Usage {
	return Usage{PromptTokens: w.PromptTokens, CompletionTokens: w.CompletionTokens, TotalTokens: w.TotalTokens}
}

// consumeStream 消费 SSE。终止判定（增补⑤）：见过 [DONE] 或 finish_reason
// 才算成功；EOF 而两者皆无（含空流）判 KindStream 截断失败。
func (c *Client) consumeStream(r io.Reader, rid string, emit func(types.StreamEvent)) (*ChatResult, error) {
	res := &ChatResult{RequestID: rid}
	calls := map[int]*types.ToolCall{}
	sawFinish := false
	done, err := scanSSE(r, c.apiKey, func(ch wireChunk) error {
		if ch.Model != "" {
			res.Model = ch.Model
		}
		if ch.Usage != nil {
			res.Usage = usageFrom(ch.Usage)
		}
		for _, choice := range ch.Choices {
			d := choice.Delta
			if d.ReasoningContent != "" {
				emit(types.StreamEvent{Kind: types.StreamReasoning, Text: d.ReasoningContent})
			}
			if d.Content != "" {
				emit(types.StreamEvent{Kind: types.StreamDelta, Text: d.Content})
			}
			for _, tc := range d.ToolCalls {
				cur := calls[tc.Index]
				if cur == nil {
					cur = &types.ToolCall{}
					calls[tc.Index] = cur
				}
				if tc.ID != "" {
					cur.ID = tc.ID
				}
				cur.Name += tc.Function.Name
				cur.Arguments += tc.Function.Arguments
			}
			if choice.FinishReason != "" {
				sawFinish = true
				res.FinishReason = choice.FinishReason
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !done && !sawFinish {
		return nil, &APIError{Kind: KindStream, RequestID: rid, Message: "stream ended without [DONE] or finish_reason"}
	}
	idxs := make([]int, 0, len(calls))
	for i := range calls {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	for _, i := range idxs {
		call := calls[i]
		if err := call.Validate(); err != nil {
			return nil, &APIError{Kind: KindProtocol, RequestID: rid, Message: err.Error()}
		}
		emit(types.StreamEvent{Kind: types.StreamToolCall, Call: call, CallIndex: i})
	}
	emit(types.StreamEvent{Kind: types.StreamDone, Model: res.Model, RequestID: rid})
	return res, nil
}

// consumeJSON 非流式（P3 降级路径）。HTTP 200 带 error / 缺 choices /
// 缺 finish_reason 均判 KindProtocol 失败，不许静默成功。
func (c *Client) consumeJSON(r io.Reader, rid string, emit func(types.StreamEvent)) (*ChatResult, error) {
	b, err := io.ReadAll(io.LimitReader(r, 8<<20))
	if err != nil {
		return nil, transportError(err, c.apiKey)
	}
	var ch wireChunk
	if err := json.Unmarshal(b, &ch); err != nil {
		return nil, &APIError{Kind: KindProtocol, RequestID: rid, Message: "response not JSON: " + redact(string(b), c.apiKey)}
	}
	if ch.Error != nil {
		return nil, &APIError{Kind: KindProtocol, RequestID: rid, Code: ch.Error.CodeString(), Message: redact(ch.Error.Message, c.apiKey)}
	}
	if len(ch.Choices) == 0 {
		return nil, &APIError{Kind: KindProtocol, RequestID: rid, Message: "response has no choices"}
	}
	choice := ch.Choices[0]
	if choice.FinishReason == "" {
		return nil, &APIError{Kind: KindProtocol, RequestID: rid, Message: "missing finish_reason"}
	}
	res := &ChatResult{Model: ch.Model, FinishReason: choice.FinishReason, RequestID: rid}
	if ch.Usage != nil {
		res.Usage = usageFrom(ch.Usage)
	}
	m := choice.Message
	if m.ReasoningContent != "" {
		emit(types.StreamEvent{Kind: types.StreamReasoning, Text: m.ReasoningContent})
	}
	if m.Content != "" {
		emit(types.StreamEvent{Kind: types.StreamDelta, Text: m.Content})
	}
	for i, tc := range m.ToolCalls {
		call := &types.ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments}
		if err := call.Validate(); err != nil {
			return nil, &APIError{Kind: KindProtocol, RequestID: rid, Message: err.Error()}
		}
		emit(types.StreamEvent{Kind: types.StreamToolCall, Call: call, CallIndex: i})
	}
	emit(types.StreamEvent{Kind: types.StreamDone, Model: res.Model, RequestID: rid})
	return res, nil
}
