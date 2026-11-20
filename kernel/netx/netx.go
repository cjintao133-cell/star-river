// Package netx 统一出站 HTTP 客户端策略（安全红线 3/7 传输层）。
//
// 禁止跟随重定向：Go 的 http.Client 默认最多跟随 10 次跳转。跨 origin 302 时
// Go 只对 Authorization 头做跨域剥离，x-api-key / x-goog-api-key / 自定义订阅头
// 等不会被剥离，会被原样带到第二 origin——即 BYOK 真 key 可能被发往非预期主机。
// 模型/API 端点本不应发生重定向，故一律拒绝跟随（fail-closed），把 3xx 原样返回，
// 交上层按协议错误处理，且不在本地错误串里额外泄漏 URL。
package netx

import (
	"net/http"
	"time"
)

// RedirectPolicy 拒绝一切重定向：返回 http.ErrUseLastResponse 让 Do 原样返回 3xx 响应，
// 不跟随、不把凭据发往新 origin。
func RedirectPolicy(req *http.Request, via []*http.Request) error {
	return http.ErrUseLastResponse
}

// Client 返回带「拒绝重定向」策略的 http.Client。
func Client(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: RedirectPolicy}
}
