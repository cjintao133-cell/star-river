// Package anthropic 手写 Anthropic Messages API HTTP 客户端（红线 1：不引第三方 SDK）。
// T-M1-04 + 健壮性增补 P1/P3/P4/P6（docs/api-pitfalls-audit.md）。
// key 只走 x-api-key 头；错误对象与日志不含 key 与完整 URL（红线 3）。
package anthropic

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// 错误分类（T-M2-05 按类型展示；P4 保留排障线索）。与 openai 包同一套 Kind 语义，
// 保证上层按 Kind 分支处理无需区分厂商。
const (
	KindAuth              = "auth"               // 401/403
	KindRateLimit         = "rate_limit"         // 429
	KindServer            = "server"             // 5xx（含 529 overloaded_error）
	KindNetwork           = "network"            // 连接/DNS/重置
	KindTimeout           = "timeout"            // 客户端或上下文超时
	KindCanceled          = "canceled"           // 调用方取消，不重试
	KindBadRequest        = "bad_request"        // 其余 4xx（400/404/413）
	KindStreamUnsupported = "stream_unsupported" // 可判定的不支持流式（P3）
	KindStream            = "stream"             // 流内错误/无 message_stop 截断（增补⑤）
	KindProtocol          = "protocol"           // 响应体不符合契约
)

// maxErrBody 错误体留存上限（P4：脱敏后保留原始错误体片段）。
const maxErrBody = 2048

// APIError 分类错误。Message 已脱敏截断；不含 URL 与 key（红线 3）。
type APIError struct {
	Kind       string
	Status     int
	RequestID  string
	Code       string
	Message    string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	return fmt.Sprintf("anthropic: %s status=%d code=%s request_id=%s: %s",
		e.Kind, e.Status, e.Code, e.RequestID, e.Message)
}

// redact 去 key、截断并修齐 UTF-8 边界（红线 3）。
func redact(s, apiKey string) string {
	if apiKey != "" {
		s = strings.ReplaceAll(s, apiKey, "[REDACTED]")
	}
	s = scrubURL(s)
	if len(s) > maxErrBody {
		s = strings.ToValidUTF8(s[:maxErrBody], "")
	}
	return s
}

// wireError Anthropic 错误体：{"type":"error","error":{"type":"...","message":"..."}}。
// 注意与 openai 不同：错误类型在 error.type（如 invalid_request_error/overloaded_error），
// 不在 error.code。这里 Type 承载分类字符串，Message 承载可读信息。
type wireError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func kindForStatus(status int) string {
	switch {
	case status == 401 || status == 403:
		return KindAuth
	case status == 429:
		return KindRateLimit
	case status >= 500: // 含 500 api_error 与 529 overloaded_error
		return KindServer
	default:
		return KindBadRequest
	}
}

// streamUnsupported 判定"可判定的不支持流式"（P3：只对明确信号降级，不盲目重试）。
// Anthropic 官方全模型支持流式；此路径为兼容中转端点偶发拒绝而保留。
// 条件：400/422/501 且错误体同时含 stream 与明确的不支持措辞。
func streamUnsupported(status int, body string) bool {
	if status != 400 && status != 422 && status != 501 {
		return false
	}
	b := strings.ToLower(body)
	if !strings.Contains(b, "stream") {
		return false
	}
	for _, sig := range []string{"not support", "unsupported", "does not support", "disabled", "not allowed", "not enabled"} {
		if strings.Contains(b, sig) {
			return true
		}
	}
	return false
}

// parseRetryAfter 支持 delta-seconds 与 HTTP-date；缺失/非法/过去时间返回 0。
func parseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil {
		if n < 0 {
			return 0
		}
		return time.Duration(n) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := t.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}

// requestID 提取端点排障 id（P4）。Anthropic 用 request-id 响应头（大小写不敏感）。
func requestID(h http.Header) string {
	for _, k := range []string{"Request-Id", "X-Request-Id", "Anthropic-Request-Id"} {
		if v := h.Get(k); v != "" {
			return v
		}
	}
	return ""
}

// apiErrorFromBody 由状态码+响应体构造分类错误（P4）。
func apiErrorFromBody(status int, header http.Header, body []byte, apiKey string) *APIError {
	e := &APIError{
		Kind:       kindForStatus(status),
		Status:     status,
		RequestID:  requestID(header),
		RetryAfter: parseRetryAfter(header.Get("Retry-After"), time.Now()),
		Message:    redact(string(body), apiKey),
	}
	var w struct {
		Error wireError `json:"error"`
	}
	if json.Unmarshal(body, &w) == nil && w.Error.Message != "" {
		e.Code = w.Error.Type
		e.Message = redact(w.Error.Message, apiKey)
	}
	if streamUnsupported(status, string(body)) {
		e.Kind = KindStreamUnsupported
	}
	return e
}

// urlRe 匹配 http(s) URL；http.Client.Do 的错误常含完整请求 URL（红线3 泄漏点）。
var urlRe = regexp.MustCompile(`https?://[^\s"']+`)

// scrubURL 把错误串里的 URL 替换为占位（下发/落盘前脱敏）。
func scrubURL(s string) string {
	if !strings.Contains(s, "://") {
		return s
	}
	return urlRe.ReplaceAllString(s, "[REDACTED_URL]")
}
