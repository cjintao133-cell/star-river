package tool

import (
	"context"
	"time"
)

// stdioDefaultCallTimeout stdio 单次请求默认超时。
const stdioDefaultCallTimeout = 30 * time.Second

// StdioClientOptions stdio 客户端配置。
type StdioClientOptions struct {
	CallTimeout     time.Duration // 单次请求超时（<=0 默认 30s）
	ClientName      string        // clientInfo.name（空用 "star-river"）
	ClientVersion   string        // clientInfo.version（空用 "0.1"）
	ProtocolVersion string        // 首选协议版本（空用 2025-06-18）
	NotifHandler    NotificationHandler
}

// StdioClient 单个 stdio MCP 服务器客户端（复用 JSON-RPC 信封与 Tool/CallResult 类型）。
// 并发安全由底层 StdioTransport 保证。
type StdioClient struct {
	tr          *StdioTransport
	callTimeout time.Duration
	reqVersion  string
	clientInfo  Implementation
	initialized bool
	initRes     InitResult
}

// NewStdioClient 建客户端（未拉起子进程；调用 Initialize 完成握手）。
func NewStdioClient(cfg StdioConfig, opts StdioClientOptions) *StdioClient {
	ct := opts.CallTimeout
	if ct <= 0 {
		ct = stdioDefaultCallTimeout
	}
	rv := opts.ProtocolVersion
	if rv == "" {
		rv = defaultProtocolVersion
	}
	name := opts.ClientName
	if name == "" {
		name = "star-river"
	}
	ver := opts.ClientVersion
	if ver == "" {
		ver = "0.1"
	}
	tr := NewStdioTransport(cfg)
	tr.SetNotificationHandler(opts.NotifHandler)
	return &StdioClient{tr: tr, callTimeout: ct, reqVersion: rv, clientInfo: Implementation{Name: name, Version: ver}}
}

// Transport 暴露底层传输（诊断/测试用）。
func (c *StdioClient) Transport() *StdioTransport { return c.tr }

// Alive 子进程是否存活。
func (c *StdioClient) Alive() bool { return c.tr.Alive() }

// Initialize 拉起子进程并完成 MCP 初始化握手。
func (c *StdioClient) Initialize(ctx context.Context) (InitResult, error) {
	if err := c.tr.Start(); err != nil {
		return InitResult{}, err
	}
	params := map[string]any{
		"protocolVersion": c.reqVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      c.clientInfo,
	}
	msg, err := c.call(ctx, "initialize", params)
	if err != nil {
		return InitResult{}, err
	}
	var res InitResult
	if err := decodeResult(msg, &res); err != nil {
		return InitResult{}, err
	}
	c.initRes = res
	c.initialized = true
	if err := c.tr.notify("notifications/initialized", nil); err != nil {
		return InitResult{}, err
	}
	return res, nil
}

// InitResult 最近一次初始化结果。
func (c *StdioClient) InitResult() InitResult { return c.initRes }

// Ping 心跳：发送 ping 并等待应答。
func (c *StdioClient) Ping(ctx context.Context) error {
	msg, err := c.call(ctx, "ping", nil)
	if err != nil {
		return err
	}
	return resultErr(msg)
}

// ListTools 发现全部工具（自动翻页）。
func (c *StdioClient) ListTools(ctx context.Context) ([]Tool, error) {
	if !c.initialized {
		return nil, &APIError{Kind: KindProtocol, Message: "ListTools before Initialize"}
	}
	var all []Tool
	cursor := ""
	for {
		var params any
		if cursor != "" {
			params = map[string]any{"cursor": cursor}
		}
		msg, err := c.call(ctx, "tools/list", params)
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

// CallTool 调用某工具。
func (c *StdioClient) CallTool(ctx context.Context, name string, args map[string]any) (*CallResult, error) {
	if !c.initialized {
		return nil, &APIError{Kind: KindProtocol, Message: "CallTool before Initialize"}
	}
	if name == "" {
		return nil, &APIError{Kind: KindProtocol, Message: "empty tool name"}
	}
	params := map[string]any{"name": name}
	if args != nil {
		params["arguments"] = args
	}
	msg, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}
	var res CallResult
	if err := decodeResult(msg, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// Close 关闭底层子进程。
func (c *StdioClient) Close() error { return c.tr.Close() }

// call 带超时的请求。
func (c *StdioClient) call(ctx context.Context, method string, params any) (*rpcMessage, error) {
	if c.callTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.callTimeout)
		defer cancel()
	}
	return c.tr.request(ctx, method, params)
}
