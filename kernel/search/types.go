package search

import (
	"context"
	"errors"
	"fmt"
)

// 错误分类。
var (
	ErrNoProvider  = errors.New("search: no provider configured")
	ErrRateLimited = errors.New("search: rate limited")
)

// ErrorKind 错误种类（供 UI/路由分类）。
type ErrorKind string

const (
	KindNetwork   ErrorKind = "network"
	KindTimeout   ErrorKind = "timeout"
	KindHTTP      ErrorKind = "http"       // 非 2xx
	KindProtocol  ErrorKind = "protocol"   // 响应结构非法
	KindAuth      ErrorKind = "auth"       // 401/403
	KindRateLimit ErrorKind = "rate_limit" // 429
	KindDisabled  ErrorKind = "disabled"   // 未配置/停用
)

// APIError 统一错误。
type APIError struct {
	Kind    ErrorKind `json:"kind"`
	Status  int       `json:"status,omitempty"`
	Message string    `json:"message"`
}

// Error 实现 error。
func (e *APIError) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("search: %s (http %d): %s", e.Kind, e.Status, e.Message)
	}
	return fmt.Sprintf("search: %s: %s", e.Kind, e.Message)
}

// Query 一次搜索请求。
type Query struct {
	Text    string // 关键词
	Lang    string // 语言（如 zh-CN / en）
	Region  string // 地区
	Count   int    // 期望条数（<=0 用默认）
	Recency string // 时效过滤：空/pd(24h)/pw(周)/pm(月)/py(年)
}

// Result 统一搜索结果条目。
type Result struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Snippet     string `json:"snippet"`
	Thumbnail   string `json:"thumbnail,omitempty"` // 缩略图 URL（可空）
	Favicon     string `json:"favicon,omitempty"`   // 站点图标 URL（可空）
	PublishedAt string `json:"published_at,omitempty"`
	Source      string `json:"source,omitempty"` // 来源后端名（填充用，便于排障）
}

// Capabilities 后端能力声明（供路由决策）。
type Capabilities struct {
	HasThumbnail    bool // 是否自带缩略图
	SupportsRecency bool // 是否支持时效过滤
	SupportsChinese bool // 是否偏好中文结果（国内后端）
}

// Provider 搜索后端接口。
type Provider interface {
	// Name 后端名（用于缓存键/排障/主备排序）。
	Name() string
	// Caps 能力声明。
	Caps() Capabilities
	// Search 执行一次搜索，返回统一结果。
	Search(ctx context.Context, q Query) ([]Result, error)
	// Healthy 健康检查（连通性/额度）。返回非 nil 视为不可用。
	Healthy(ctx context.Context) error
}

// normalizeCount 归一默认条数。
func normalizeCount(n int) int {
	if n <= 0 {
		return 10
	}
	if n > 50 {
		return 50
	}
	return n
}

// classifyStatus 由 HTTP 状态码归类。
func classifyStatus(status int, body string) *APIError {
	switch {
	case status == 401 || status == 403:
		return &APIError{Kind: KindAuth, Status: status, Message: truncate(body, 200)}
	case status == 429:
		return &APIError{Kind: KindRateLimit, Status: status, Message: truncate(body, 200)}
	case status >= 500:
		return &APIError{Kind: KindHTTP, Status: status, Message: truncate(body, 200)}
	default:
		return &APIError{Kind: KindHTTP, Status: status, Message: truncate(body, 200)}
	}
}

// truncate 截断字符串（脱敏展示）。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
