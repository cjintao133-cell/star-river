package tool

import (
	"encoding/json"
	"errors"
	"fmt"
)

// jsonrpcVersion 固定为 "2.0"。
const jsonrpcVersion = "2.0"

// ErrKind 错误分类（调用方用 errors.As 取 *APIError 再判 Kind）。
type ErrKind string

const (
	KindNetwork        ErrKind = "network"         // 连接/读失败
	KindTimeout        ErrKind = "timeout"         // 超时
	KindCanceled       ErrKind = "canceled"        // 主动取消
	KindHTTP           ErrKind = "http"            // 非 2xx 状态
	KindProtocol       ErrKind = "protocol"        // JSON/信封/SSE 结构非法
	KindRPC            ErrKind = "rpc"             // JSON-RPC error 对象
	KindSessionExpired ErrKind = "session_expired" // 404，需重开会话
)

// APIError 本包统一错误类型。Message 已脱敏（不含密钥/完整敏感 URL）。
type APIError struct {
	Kind       ErrKind `json:"kind"`
	HTTPStatus int     `json:"http_status,omitempty"`
	Code       int     `json:"code,omitempty"` // JSON-RPC error.code
	Message    string  `json:"message"`
}

// Error 实现 error。
func (e *APIError) Error() string {
	switch {
	case e.HTTPStatus != 0 && e.Code != 0:
		return fmt.Sprintf("mcp: %s (http %d, rpc %d): %s", e.Kind, e.HTTPStatus, e.Code, e.Message)
	case e.HTTPStatus != 0:
		return fmt.Sprintf("mcp: %s (http %d): %s", e.Kind, e.HTTPStatus, e.Message)
	case e.Code != 0:
		return fmt.Sprintf("mcp: %s (rpc %d): %s", e.Kind, e.Code, e.Message)
	default:
		return fmt.Sprintf("mcp: %s: %s", e.Kind, e.Message)
	}
}

// rpcError JSON-RPC 2.0 错误对象。
type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// rpcMessage JSON-RPC 2.0 消息信封（请求/响应/通知共用）。
// ID 为 nil 表示通知（notifications/*）。
type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// encodeRequest 编码一个 JSON-RPC 请求。
func encodeRequest(id int64, method string, params any) ([]byte, error) {
	raw, err := marshalParams(params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(rpcMessage{JSONRPC: jsonrpcVersion, ID: &id, Method: method, Params: raw})
}

// encodeNotification 编码一个 JSON-RPC 通知（无 id）。
func encodeNotification(method string, params any) ([]byte, error) {
	raw, err := marshalParams(params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(rpcMessage{JSONRPC: jsonrpcVersion, Method: method, Params: raw})
}

// marshalParams nil → nil（省略 params 字段）。
func marshalParams(params any) (json.RawMessage, error) {
	if params == nil {
		return nil, nil
	}
	return json.Marshal(params)
}

// resultErr 把响应信封里的 error 转为 *APIError；无 error 返回 nil。
func resultErr(msg *rpcMessage) error {
	if msg == nil {
		return &APIError{Kind: KindProtocol, Message: "nil message"}
	}
	if msg.Error != nil {
		return &APIError{Kind: KindRPC, Code: msg.Error.Code, Message: msg.Error.Message}
	}
	return nil
}

// decodeResult 校验信封无 error 后把 result 解到 out。
func decodeResult(msg *rpcMessage, out any) error {
	if err := resultErr(msg); err != nil {
		return err
	}
	if len(msg.Result) == 0 {
		return &APIError{Kind: KindProtocol, Message: "response missing result"}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(msg.Result, out); err != nil {
		return &APIError{Kind: KindProtocol, Message: "result not decodable: " + err.Error()}
	}
	return nil
}

// asAPIError 尽力把任意 error 归类为 *APIError（已是则原样返回）。
func asAPIError(err error) *APIError {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae
	}
	return &APIError{Kind: KindNetwork, Message: err.Error()}
}
