package tool

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// defaultProtocolVersion 本客户端首选协议版本（2025-06-18 Streamable HTTP）。
const defaultProtocolVersion = "2025-06-18"

// defaultCallTimeout 单次请求默认超时。
const defaultCallTimeout = 30 * time.Second

// Tool MCP 工具定义（tools/list 条目）。
type Tool struct {
	Name         string          `json:"name"`
	Title        string          `json:"title,omitempty"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"inputSchema,omitempty"`
	OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
	Annotations  json.RawMessage `json:"annotations,omitempty"`
}

// Content 工具结果内容项（文本/图像/音频/资源链接等；按 Type 取用对应字段）。
type Content struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MIMEType string `json:"mimeType,omitempty"`
	URI      string `json:"uri,omitempty"`
	Name     string `json:"name,omitempty"`
}

// CallResult tools/call 的结果。IsError=true 表示工具执行错误（协议成功、业务失败）。
type CallResult struct {
	Content           []Content       `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
	IsError           bool            `json:"isError,omitempty"`
}

// Implementation 客户端/服务器实现信息。
type Implementation struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version,omitempty"`
}

// InitResult initialize 的服务器应答。
type InitResult struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Capabilities    json.RawMessage `json:"capabilities"`
	ServerInfo      Implementation  `json:"serverInfo"`
	Instructions    string          `json:"instructions,omitempty"`
}

// Options 客户端配置。
type Options struct {
	HTTPClient      *http.Client  // 可为 nil
	Header          http.Header   // 额外头（如 Authorization）
	CallTimeout     time.Duration // 单次请求超时（<=0 用默认 30s）
	ProtocolVersion string        // 首选协议版本（空用 2025-06-18）
	ClientName      string        // clientInfo.name（空用 "star-river"）
	ClientVersion   string        // clientInfo.version（空用 "0.1"）
}

// Client 单个 MCP 服务器客户端。并发安全。
type Client struct {
	tr          *StreamableHTTPTransport
	callTimeout time.Duration
	reqVersion  string
	clientInfo  Implementation

	mu          sync.Mutex
	nextID      int64
	initialized bool
	initRes     InitResult
}

// NewClient 建客户端（尚未初始化，调用 Initialize 完成握手）。
func NewClient(endpoint string, opts Options) *Client {
	rt := opts.ProtocolVersion
	if rt == "" {
		rt = defaultProtocolVersion
	}
	ct := opts.CallTimeout
	if ct <= 0 {
		ct = defaultCallTimeout
	}
	name := opts.ClientName
	if name == "" {
		name = "star-river"
	}
	ver := opts.ClientVersion
	if ver == "" {
		ver = "0.1"
	}
	return &Client{
		tr:          NewStreamableHTTPTransport(endpoint, opts.HTTPClient, opts.Header),
		callTimeout: ct,
		reqVersion:  rt,
		clientInfo:  Implementation{Name: name, Version: ver},
	}
}

// nextIDv 自增请求 id。
func (c *Client) nextIDv() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	return c.nextID
}

// SessionID 当前会话 id（测试/排障用）。
func (c *Client) SessionID() string { return c.tr.SessionID() }

// ProtocolVersion 协商后的协议版本。
func (c *Client) ProtocolVersion() string { return c.tr.ProtocolVersion() }

// Initialize 执行 MCP 初始化握手（initialize 请求 + initialized 通知）。
// 捕获 Mcp-Session-Id 与协商协议版本，供后续请求使用。
func (c *Client) Initialize(ctx context.Context) (InitResult, error) {
	params := map[string]any{
		"protocolVersion": c.reqVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      c.clientInfo,
	}
	msg, hdr, err := c.callAttempt(ctx, "initialize", params, false)
	if err != nil {
		return InitResult{}, err
	}
	var res InitResult
	if err := decodeResult(msg, &res); err != nil {
		return InitResult{}, err
	}
	// 采纳服务器版本（版本协商：不支持则本客户端仍按返回版本继续；严格客户端可断开）。
	sid := ""
	if hdr != nil {
		sid = hdr.Get("Mcp-Session-Id")
	}
	c.tr.setSession(sid, res.ProtocolVersion)
	c.mu.Lock()
	c.initRes = res
	c.initialized = true
	c.mu.Unlock()
	// 发送 initialized 通知（无 id，服务器应回 202）。
	if err := c.notify(ctx, "notifications/initialized", nil); err != nil {
		return InitResult{}, err
	}
	return res, nil
}

// InitResult 最近一次初始化结果（未初始化返回零值）。
func (c *Client) InitResult() InitResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.initRes
}

// Ping 发送 ping 请求（可用于健康检查）。
func (c *Client) Ping(ctx context.Context) error {
	msg, _, err := c.callAttempt(ctx, "ping", nil, true)
	if err != nil {
		return err
	}
	return resultErr(msg)
}

// ListTools 发现全部工具（自动翻页）。需先 Initialize。
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	if !c.isInitialized() {
		return nil, &APIError{Kind: KindProtocol, Message: "ListTools before Initialize"}
	}
	var all []Tool
	cursor := ""
	for {
		var params any
		if cursor != "" {
			params = map[string]any{"cursor": cursor}
		}
		msg, _, err := c.callAttempt(ctx, "tools/list", params, true)
		if err != nil {
			return nil, err
		}
		var res struct {
			Tools      []Tool `json:"tools"`
			NextCursor string `json:"nextCursor"`
		}
		if err := decodeResult(msg, &res); err != nil {
			return nil, err
		}
		all = append(all, res.Tools...)
		if res.NextCursor == "" {
			break
		}
		cursor = res.NextCursor
	}
	return all, nil
}

// CallTool 调用某工具。args 为结构化参数（nil 表示无参）。需先 Initialize。
// 每请求受 CallTimeout/ctx 约束；ctx 取消时尽力发 notifications/cancelled。
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (*CallResult, error) {
	if !c.isInitialized() {
		return nil, &APIError{Kind: KindProtocol, Message: "CallTool before Initialize"}
	}
	if name == "" {
		return nil, &APIError{Kind: KindProtocol, Message: "empty tool name"}
	}
	params := map[string]any{"name": name}
	if args != nil {
		params["arguments"] = args
	}
	msg, _, err := c.callAttempt(ctx, "tools/call", params, true)
	if err != nil {
		return nil, err
	}
	var res CallResult
	if err := decodeResult(msg, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// Close 显式结束会话（DELETE；405 忽略）。
func (c *Client) Close(ctx context.Context) error { return c.tr.Delete(ctx) }

func (c *Client) isInitialized() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.initialized
}

// callAttempt 发送一次请求并解析应答。allowReinit：遇会话失效(404)时自动重开会话并重试一次。
func (c *Client) callAttempt(ctx context.Context, method string, params any, allowReinit bool) (*rpcMessage, http.Header, error) {
	if c.callTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.callTimeout)
		defer cancel()
	}
	id := c.nextIDv()
	body, err := encodeRequest(id, method, params)
	if err != nil {
		return nil, nil, &APIError{Kind: KindProtocol, Message: "encode request: " + err.Error()}
	}
	resp, err := c.tr.Post(ctx, body)
	if err != nil {
		ae := asAPIError(err)
		if ae.Kind == KindCanceled || ae.Kind == KindTimeout {
			c.notifyCancelled(id, string(ae.Kind))
		}
		return nil, nil, err
	}
	hdr := resp.Header
	msg, err := readResponse(resp, id)
	if err != nil {
		var ae *APIError
		if ok := asAPIErrorErr(err, &ae); ok && ae.Kind == KindSessionExpired && allowReinit && method != "initialize" && c.isInitialized() {
			if rerr := c.reinitialize(ctx); rerr == nil {
				return c.callAttempt(ctx, method, params, false)
			}
		}
		return nil, hdr, err
	}
	return msg, hdr, nil
}

// reinitialize 会话失效后重新 Initialize。
func (c *Client) reinitialize(ctx context.Context) error {
	_, err := c.Initialize(ctx)
	return err
}

// notify 发送 JSON-RPC 通知（无 id），期望 2xx（通常 202，无 body）。
func (c *Client) notify(ctx context.Context, method string, params any) error {
	body, err := encodeNotification(method, params)
	if err != nil {
		return &APIError{Kind: KindProtocol, Message: "encode notification: " + err.Error()}
	}
	resp, err := c.tr.Post(ctx, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{Kind: KindHTTP, HTTPStatus: resp.StatusCode, Message: "notification rejected"}
	}
	return nil
}

// notifyCancelled 尽力发送 notifications/cancelled（用独立短超时，避免被已取消的 ctx 拖累）。
func (c *Client) notifyCancelled(requestID int64, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = c.notify(ctx, "notifications/cancelled", map[string]any{
		"requestId": requestID,
		"reason":    reason,
	})
}

// asAPIErrorErr errors.As 风格但目标是 **APIError（供内部判断 Kind）。
func asAPIErrorErr(err error, target **APIError) bool {
	if err == nil {
		return false
	}
	if ae, ok := err.(*APIError); ok {
		*target = ae
		return true
	}
	return false
}
