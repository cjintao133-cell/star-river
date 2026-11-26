// Package netx 统一出站 HTTP 客户端策略（安全红线 3/7 传输层）。
//
// 禁止跟随重定向：Go 的 http.Client 默认最多跟随 10 次跳转。跨 origin 302 时
// Go 只对 Authorization 头做跨域剥离，x-api-key / x-goog-api-key / 自定义订阅头
// 等不会被剥离，会被原样带到第二 origin——即 BYOK 真 key 可能被发往非预期主机。
// 模型/API 端点本不应发生重定向，故一律拒绝跟随（fail-closed），把 3xx 原样返回，
// 交上层按协议错误处理，且不在本地错误串里额外泄漏 URL。
//
// 代理：所有经 Client() 构造的客户端共享同一代理设置（SetProxy）。模型适配器/
// 搜索/语音/绘画/MCP 传输层都走 Client()，故一处设置全局生效；空串=直连（回退环境变量）。
package netx

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	proxyMu  sync.RWMutex
	proxyRaw string
)

// RedirectPolicy 拒绝一切重定向：返回 http.ErrUseLastResponse 让 Do 原样返回 3xx 响应，
// 不跟随、不把凭据发往新 origin。
func RedirectPolicy(req *http.Request, via []*http.Request) error {
	return http.ErrUseLastResponse
}

// SetProxy 设置出站 HTTP 代理（如 http://127.0.0.1:7890 或 socks5://127.0.0.1:1080）。
// 空串清除代理、回退直连（并尊重 HTTP_PROXY/HTTPS_PROXY 环境变量）。
func SetProxy(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		proxyMu.Lock()
		proxyRaw = ""
		proxyMu.Unlock()
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("proxy must include scheme and host, e.g. http://127.0.0.1:7890")
	}
	proxyMu.Lock()
	proxyRaw = raw
	proxyMu.Unlock()
	return nil
}

// CurrentProxy 返回当前代理串（空=直连）。
func CurrentProxy() string {
	proxyMu.RLock()
	defer proxyMu.RUnlock()
	return proxyRaw
}

func proxyFunc(req *http.Request) (*url.URL, error) {
	proxyMu.RLock()
	p := proxyRaw
	proxyMu.RUnlock()
	if p == "" {
		return http.ProxyFromEnvironment(req)
	}
	return url.Parse(p)
}

// Client 返回带「拒绝重定向」策略 + 代理支持的 http.Client。
func Client(timeout time.Duration) *http.Client {
	var tr *http.Transport
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		tr = base.Clone()
	} else {
		tr = &http.Transport{}
	}
	tr.Proxy = proxyFunc
	return &http.Client{Timeout: timeout, CheckRedirect: RedirectPolicy, Transport: tr}
}
