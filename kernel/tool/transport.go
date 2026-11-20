package tool

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"star-river/kernel/netx"
)

// maxBodyBytes 单次响应体读取上限（JSON 分支）。
const maxBodyBytes = 8 << 20 // 8MiB

// maxSSELineBytes 单行 SSE 上限。
const maxSSELineBytes = 4 << 20 // 4MiB

// defaultHTTPTimeout 默认单请求超时。
const defaultHTTPTimeout = 60 * time.Second

// StreamableHTTPTransport 实现 MCP 的 Streamable HTTP 传输。
// 并发安全：sessionID/protocolVersion 由互斥量保护。
type StreamableHTTPTransport struct {
	Endpoint string       // MCP 端点（单一 POST/GET/DELETE 路径）
	Client   *http.Client // 可为 nil（用带默认超时的 client）
	Header   http.Header  // 额外请求头（如 Authorization），会复制进每个请求

	mu              sync.Mutex
	sessionID       string
	protocolVersion string
}

// NewStreamableHTTPTransport 建传输层。extra 头会逐请求附带（不改动调用方 Header）。
func NewStreamableHTTPTransport(endpoint string, client *http.Client, extra http.Header) *StreamableHTTPTransport {
	if client == nil {
		client = netx.Client(defaultHTTPTimeout)
	}
	return &StreamableHTTPTransport{
		Endpoint: strings.TrimSpace(endpoint),
		Client:   client,
		Header:   extra.Clone(),
	}
}

// setSession 初始化后设置会话 id 与协商版本。
func (t *StreamableHTTPTransport) setSession(id, protocolVersion string) {
	t.mu.Lock()
	t.sessionID = id
	if protocolVersion != "" {
		t.protocolVersion = protocolVersion
	}
	t.mu.Unlock()
}

// SessionID 当前会话 id（未初始化为空）。
func (t *StreamableHTTPTransport) SessionID() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sessionID
}

// ProtocolVersion 当前协议版本。
func (t *StreamableHTTPTransport) ProtocolVersion() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.protocolVersion
}

// newRequest 构造带公共头的请求。
func (t *StreamableHTTPTransport) newRequest(ctx context.Context, method string, body []byte) (*http.Request, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, t.Endpoint, rdr)
	if err != nil {
		return nil, &APIError{Kind: KindProtocol, Message: "build request: " + err.Error()}
	}
	for k, vs := range t.Header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	t.mu.Lock()
	sid, ver := t.sessionID, t.protocolVersion
	t.mu.Unlock()
	if sid != "" {
		req.Header.Set("Mcp-Session-Id", sid)
	}
	if ver != "" {
		req.Header.Set("MCP-Protocol-Version", ver)
	}
	return req, nil
}

// Post 发送 JSON-RPC 消息，返回原始响应（调用方负责关闭 body）。
// 每次 POST 新请求，符合 Streamable HTTP「每条消息独立 POST」。
func (t *StreamableHTTPTransport) Post(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := t.newRequest(ctx, http.MethodPost, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := t.Client.Do(req)
	if err != nil {
		return nil, transportError(err)
	}
	return resp, nil
}

// Delete 显式结束会话（尽力而为；405 表示服务器不支持，忽略）。
func (t *StreamableHTTPTransport) Delete(ctx context.Context) error {
	if t.SessionID() == "" {
		return nil
	}
	req, err := t.newRequest(ctx, http.MethodDelete, nil)
	if err != nil {
		return err
	}
	resp, err := t.Client.Do(req)
	if err != nil {
		return transportError(err)
	}
	defer resp.Body.Close()
	if (resp.StatusCode >= 200 && resp.StatusCode < 300) || resp.StatusCode == http.StatusMethodNotAllowed {
		return nil
	}
	return &APIError{Kind: KindHTTP, HTTPStatus: resp.StatusCode, Message: "session delete failed"}
}

// transportError 归类传输层错误。
func transportError(err error) *APIError {
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
	return &APIError{Kind: kind, Message: err.Error()}
}

// readResponse 解析一次 POST 的响应，返回匹配 wantID 的 JSON-RPC 应答。
// 支持 application/json 与 text/event-stream 两种 Content-Type。
func readResponse(resp *http.Response, wantID int64) (*rpcMessage, error) {
	defer resp.Body.Close()
	// 会话失效：404 → 需重开会话。
	if resp.StatusCode == http.StatusNotFound {
		return nil, &APIError{Kind: KindSessionExpired, HTTPStatus: resp.StatusCode, Message: "session not found (404)"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, &APIError{Kind: KindHTTP, HTTPStatus: resp.StatusCode, Message: strings.TrimSpace(string(body))}
	}
	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "text/event-stream") {
		return readSSEResponse(resp.Body, wantID)
	}
	// 默认按 JSON。
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, transportError(err)
	}
	var msg rpcMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, &APIError{Kind: KindProtocol, Message: "response not JSON: " + err.Error()}
	}
	return &msg, nil
}

// readSSEResponse 从 SSE 流里读取第一个 id==wantID 的 JSON-RPC 应答。
// 途中出现的服务器通知/请求（id 不匹配或无 id）忽略（v1 不处理服务端发起请求）。
func readSSEResponse(r io.Reader, wantID int64) (*rpcMessage, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), maxSSELineBytes)
	var data []string
	dispatch := func() (*rpcMessage, bool, error) {
		if len(data) == 0 {
			return nil, false, nil
		}
		payload := strings.Join(data, "\n")
		data = data[:0]
		var msg rpcMessage
		if err := json.Unmarshal([]byte(payload), &msg); err != nil {
			return nil, false, &APIError{Kind: KindProtocol, Message: "sse event not JSON: " + err.Error()}
		}
		if msg.ID != nil && *msg.ID == wantID {
			return &msg, true, nil
		}
		return nil, false, nil // 非本次应答，忽略
	}
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if m, done, err := dispatch(); err != nil {
				return nil, err
			} else if done {
				return m, nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "id:") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			v := strings.TrimPrefix(line, "data:")
			v = strings.TrimPrefix(v, " ") // SSE 规范：冒号后单个空格被去掉
			data = append(data, v)
		}
	}
	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return nil, &APIError{Kind: KindProtocol, Message: "sse line too long"}
		}
		return nil, transportError(err)
	}
	// 流结束前最后一事件可能无结尾空行，再 flush 一次。
	if m, done, err := dispatch(); err != nil {
		return nil, err
	} else if done {
		return m, nil
	}
	return nil, &APIError{Kind: KindProtocol, Message: "sse stream closed without matching response"}
}
