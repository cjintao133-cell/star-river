package anthropic

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

// defaultVersion anthropic-version 头默认值。Anthropic 要求每个请求显式携带该头。
const defaultVersion = "2023-06-01"

// Usage token 用量（T-M3-07 统计数据源）。
type Usage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// ChatRequest v1 最小请求。P1：model/max_tokens/stream 一律显式，不吃端点默认值。
// System 为独立系统提示字段（Anthropic 无 system role）；messages 里的 system 消息
// 也会被抽取合并进 System。MaxTokens Anthropic 强制必填。
type ChatRequest struct {
	Model        string
	System       string
	Messages     []types.Message
	MaxTokens    int
	NativeSearch bool // 启用模型原生网页搜索（Anthropic: tools web_search_20250305）
}

// ChatResult 一次成功调用的元数据。Model 为实际应答模型（中转掉包信号，T-M1-06 标记）。
// FinishReason 为 stop_reason（end_turn/max_tokens/tool_use/stop_sequence）。
// Degraded=true 表示 P3 降级：流式被明确拒绝后以 stream:false 重试成功。
type ChatResult struct {
	Model        string
	FinishReason string
	Usage        Usage
	Degraded     bool
	RequestID    string
	Sources      []Citation // 网页搜索引用来源（NativeSearch 时）
}

// Citation 一条搜索引用来源。
type Citation struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// webResultItem web_search_tool_result 的单条结果。
type webResultItem struct {
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

// collectWebResults 解析 web_search_tool_result.content（数组）追加去重来源。
func collectWebResults(dst []Citation, raw json.RawMessage) []Citation {
	if len(raw) == 0 {
		return dst
	}
	var items []webResultItem
	if json.Unmarshal(raw, &items) != nil {
		return dst
	}
	for _, it := range items {
		if it.URL == "" {
			continue
		}
		dup := false
		for _, c := range dst {
			if c.URL == it.URL {
				dup = true
				break
			}
		}
		if !dup {
			dst = append(dst, Citation{URL: it.URL, Title: it.Title})
		}
	}
	return dst
}

// Client 并发安全：字段构造后只读，http.Client 自身并发安全。
// BaseURL 形如 https://host/v1（尾斜杠自动去除），与 openai 适配器一致便于 T-M1-06 统一发现。
type Client struct {
	BaseURL     string
	HTTP        *http.Client
	MaxRetries  int
	BackoffBase time.Duration
	Version     string // anthropic-version，留空用 defaultVersion

	apiKey string
	sleep  func(ctx context.Context, d time.Duration) error
}

// New 默认 60s 整体超时。key 只经 x-api-key 头，绝不进 URL（红线 3）。
func New(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		HTTP:        &http.Client{Timeout: 60 * time.Second},
		MaxRetries:  2,
		BackoffBase: 500 * time.Millisecond,
		Version:     defaultVersion,
		apiKey:      apiKey,
		sleep:       sleepCtx,
	}
}

func (c *Client) version() string {
	if c.Version == "" {
		return defaultVersion
	}
	return c.Version
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

// wireOutBlock 请求侧内容块（按 type 使用可选字段）。
type wireOutBlock struct {
	Type      string           `json:"type"`
	Text      string           `json:"text,omitempty"`
	ID        string           `json:"id,omitempty"`          // tool_use
	Name      string           `json:"name,omitempty"`        // tool_use
	Input     json.RawMessage  `json:"input,omitempty"`       // tool_use
	ToolUseID string           `json:"tool_use_id,omitempty"` // tool_result
	Content   string           `json:"content,omitempty"`     // tool_result（字符串形态）
	IsError   bool             `json:"is_error,omitempty"`    // tool_result
	Source    *wireImageSource `json:"source,omitempty"`      // image
}

// wireImageSource Anthropic 图片源（base64）。
type wireImageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // image/png 等
	Data      string `json:"data"`       // base64
}

type wireOutMessage struct {
	Role    string         `json:"role"`
	Content []wireOutBlock `json:"content"`
}

// convertMessages 契约类型 → Anthropic 线格式。返回抽取出的 system 文本与消息列表。
// 语义差异（相对 openai）：
//   - system role 消息不进 messages，抽取合并到独立 system 字段；
//   - tool_result 块 → user 消息里的 tool_result 内容块（Anthropic 无 tool role）；
//   - tool_use 块 → assistant 消息里的 tool_use 内容块；
//   - 相邻同 role 消息合并（Anthropic 要求 user/assistant 交替）。
//
// v1 不支持的块（image/audio）显式报错，不静默丢弃。
func convertMessages(msgs []types.Message) (string, []wireOutMessage, error) {
	var systemParts []string
	out := make([]wireOutMessage, 0, len(msgs))
	for i, m := range msgs {
		if !m.Role.Valid() {
			return "", nil, &APIError{Kind: KindBadRequest, Message: fmt.Sprintf("messages[%d]: invalid role %q", i, m.Role)}
		}
		if m.Role == types.RoleSystem {
			for j, b := range m.Blocks {
				switch b.Type {
				case types.BlockText:
					systemParts = append(systemParts, b.Text)
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
			role = "assistant"
		}
		blocks := make([]wireOutBlock, 0, len(m.Blocks))
		for j, b := range m.Blocks {
			switch b.Type {
			case types.BlockText:
				blocks = append(blocks, wireOutBlock{Type: "text", Text: b.Text})
			case types.BlockReasoning:
				// 推理过程是端点产物，不回传。
			case types.BlockImage:
				if b.MIMEType == "" || len(b.Data) == 0 {
					return "", nil, &APIError{Kind: KindBadRequest, Message: fmt.Sprintf("messages[%d].blocks[%d]: image missing mime/data", i, j)}
				}
				blocks = append(blocks, wireOutBlock{Type: "image", Source: &wireImageSource{Type: "base64", MediaType: b.MIMEType, Data: base64.StdEncoding.EncodeToString(b.Data)}})
			case types.BlockToolUse:
				if b.Call == nil {
					return "", nil, &APIError{Kind: KindBadRequest, Message: fmt.Sprintf("messages[%d].blocks[%d]: tool_use without call", i, j)}
				}
				input := json.RawMessage(b.Call.Arguments)
				switch {
				case len(input) == 0:
					input = json.RawMessage("{}")
				case !json.Valid(input):
					return "", nil, &APIError{Kind: KindBadRequest, Message: fmt.Sprintf("messages[%d].blocks[%d]: tool_use arguments not valid JSON", i, j)}
				}
				blocks = append(blocks, wireOutBlock{Type: "tool_use", ID: b.Call.ID, Name: b.Call.Name, Input: input})
			case types.BlockToolResult:
				if b.CallID == "" {
					return "", nil, &APIError{Kind: KindBadRequest, Message: fmt.Sprintf("messages[%d].blocks[%d]: tool_result without call_id", i, j)}
				}
				blocks = append(blocks, wireOutBlock{Type: "tool_result", ToolUseID: b.CallID, Content: b.Text, IsError: b.IsError})
				role = "user"
			default:
				return "", nil, &APIError{Kind: KindBadRequest, Message: fmt.Sprintf("messages[%d].blocks[%d]: block type %q unsupported in v1", i, j, b.Type)}
			}
		}
		if len(blocks) == 0 {
			continue
		}
		if n := len(out); n > 0 && out[n-1].Role == role {
			out[n-1].Content = append(out[n-1].Content, blocks...)
		} else {
			out = append(out, wireOutMessage{Role: role, Content: blocks})
		}
	}
	var sys []string
	for _, s := range systemParts {
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

// buildBody P1：model/max_tokens/stream 全部显式序列化；system 仅在非空时携带。
func buildBody(req ChatRequest, stream bool) ([]byte, error) {
	sysFromMsgs, msgs, err := convertMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	body := struct {
		Model     string           `json:"model"`
		MaxTokens int              `json:"max_tokens"`
		System    string           `json:"system,omitempty"`
		Messages  []wireOutMessage `json:"messages"`
		Stream    bool             `json:"stream"`
		Tools     []map[string]any `json:"tools,omitempty"`
	}{Model: req.Model, MaxTokens: req.MaxTokens, System: joinSystem(req.System, sysFromMsgs), Messages: msgs, Stream: stream}
	if req.NativeSearch {
		body.Tools = append(body.Tools, map[string]any{"type": "web_search_20250305", "name": "web_search", "max_uses": 5})
	}
	return json.Marshal(body)
}

// doOnce 单次请求：非 200 读错误体分类后关体；200 把响应体交还调用方。
// 头契约：x-api-key + anthropic-version（无 Authorization Bearer）。
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
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", c.version())
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
	resp, err := c.do(ctx, http.MethodPost, "/messages", accept, body)
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

func usageFrom(w wireUsage) Usage {
	return Usage(w)
}

// consumeStream 消费 Anthropic 事件流（事件分帧）。
// 终止判定（增补⑤）：必须见到 message_stop 才算成功；EOF 前未见 → KindStream 截断失败。
// （Anthropic 无 [DONE] 哨兵，以 message_stop 事件为唯一成功终止信号。）
// tool_use 参数按 content block index 聚合（content_block_start 起始，input_json_delta 续段）。
func (c *Client) consumeStream(r io.Reader, rid string, emit func(types.StreamEvent)) (*ChatResult, error) {
	res := &ChatResult{RequestID: rid}
	calls := map[int]*types.ToolCall{}
	jsonBuf := map[int]*strings.Builder{}
	sawStop := false
	err := scanEvents(r, c.apiKey, func(ev wireEvent) error {
		switch ev.Type {
		case "message_start":
			if ev.Message != nil {
				if ev.Message.Model != "" {
					res.Model = ev.Message.Model
				}
				res.Usage = usageFrom(ev.Message.Usage)
			}
		case "content_block_start":
			if ev.ContentBlock != nil && ev.ContentBlock.Type == "tool_use" {
				calls[ev.Index] = &types.ToolCall{ID: ev.ContentBlock.ID, Name: ev.ContentBlock.Name}
				jsonBuf[ev.Index] = &strings.Builder{}
			}
			if ev.ContentBlock != nil && ev.ContentBlock.Type == "web_search_tool_result" {
				res.Sources = collectWebResults(res.Sources, ev.ContentBlock.Content)
			}
		case "content_block_delta":
			if ev.Delta != nil {
				switch ev.Delta.Type {
				case "text_delta":
					if ev.Delta.Text != "" {
						emit(types.StreamEvent{Kind: types.StreamDelta, Text: ev.Delta.Text})
					}
				case "thinking_delta":
					if ev.Delta.Thinking != "" {
						emit(types.StreamEvent{Kind: types.StreamReasoning, Text: ev.Delta.Thinking})
					}
				case "input_json_delta":
					if b := jsonBuf[ev.Index]; b != nil {
						b.WriteString(ev.Delta.PartialJSON)
					}
				}
			}
		case "message_delta":
			if ev.Delta != nil && ev.Delta.StopReason != "" {
				res.FinishReason = ev.Delta.StopReason
			}
			if ev.Usage != nil {
				res.Usage.OutputTokens = ev.Usage.OutputTokens
			}
		case "message_stop":
			sawStop = true
			return errStopScan
		case "ping":
			// 心跳，忽略。
		case "error":
			if ev.Error != nil {
				return &APIError{Kind: KindStream, Code: ev.Error.Type, RequestID: rid, Message: redact(ev.Error.Message, c.apiKey)}
			}
			return &APIError{Kind: KindStream, RequestID: rid, Message: "stream error event"}
		default:
			// content_block_stop 等无载荷事件忽略。
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !sawStop {
		return nil, &APIError{Kind: KindStream, RequestID: res.RequestID, Message: "stream ended without message_stop"}
	}
	idxs := make([]int, 0, len(calls))
	for i := range calls {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	for _, i := range idxs {
		call := calls[i]
		if b := jsonBuf[i]; b != nil {
			call.Arguments = b.String()
		}
		if err := call.Validate(); err != nil {
			return nil, &APIError{Kind: KindProtocol, RequestID: res.RequestID, Message: err.Error()}
		}
		emit(types.StreamEvent{Kind: types.StreamToolCall, Call: call, CallIndex: i})
	}
	emit(types.StreamEvent{Kind: types.StreamDone, Model: res.Model, RequestID: res.RequestID})
	return res, nil
}

// consumeJSON 非流式（P3 降级路径）。HTTP 200 带 error / 缺 stop_reason 均判失败，不许静默成功。
func (c *Client) consumeJSON(r io.Reader, rid string, emit func(types.StreamEvent)) (*ChatResult, error) {
	b, err := io.ReadAll(io.LimitReader(r, 8<<20))
	if err != nil {
		return nil, transportError(err, c.apiKey)
	}
	var w struct {
		Type       string             `json:"type"`
		Model      string             `json:"model"`
		Content    []wireContentBlock `json:"content"`
		StopReason string             `json:"stop_reason"`
		Usage      wireUsage          `json:"usage"`
		Error      *wireError         `json:"error"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return nil, &APIError{Kind: KindProtocol, RequestID: rid, Message: "response not JSON: " + redact(string(b), c.apiKey)}
	}
	if w.Type == "error" || w.Error != nil {
		code, msg := "", ""
		if w.Error != nil {
			code, msg = w.Error.Type, w.Error.Message
		}
		return nil, &APIError{Kind: KindProtocol, RequestID: rid, Code: code, Message: redact(msg, c.apiKey)}
	}
	if w.StopReason == "" {
		return nil, &APIError{Kind: KindProtocol, RequestID: rid, Message: "missing stop_reason"}
	}
	res := &ChatResult{Model: w.Model, FinishReason: w.StopReason, RequestID: rid, Usage: usageFrom(w.Usage)}
	toolIdx := 0
	for _, cb := range w.Content {
		switch cb.Type {
		case "text":
			if cb.Text != "" {
				emit(types.StreamEvent{Kind: types.StreamDelta, Text: cb.Text})
			}
		case "thinking":
			if cb.Thinking != "" {
				emit(types.StreamEvent{Kind: types.StreamReasoning, Text: cb.Thinking})
			}
		case "web_search_tool_result":
			res.Sources = collectWebResults(res.Sources, cb.Content)
		case "tool_use":
			args := ""
			if len(cb.Input) > 0 && string(cb.Input) != "null" {
				args = string(cb.Input)
			}
			call := &types.ToolCall{ID: cb.ID, Name: cb.Name, Arguments: args}
			if err := call.Validate(); err != nil {
				return nil, &APIError{Kind: KindProtocol, RequestID: rid, Message: err.Error()}
			}
			emit(types.StreamEvent{Kind: types.StreamToolCall, Call: call, CallIndex: toolIdx})
			toolIdx++
		}
	}
	emit(types.StreamEvent{Kind: types.StreamDone, Model: res.Model, RequestID: rid})
	return res, nil
}
