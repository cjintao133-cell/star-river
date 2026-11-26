package mobilebind

// 网络代理设置：让内核全部出站请求（模型/搜索/语音/绘画/MCP 传输层，均经 netx.Client）
// 共享同一 HTTP/SOCKS 代理。空串=直连。App 侧在设置里填代理地址并在启动时下发。

import "github.com/cjintao133-cell/star-river/kernel/netx"

// SetNetworkProxy 设置出站代理（http://host:port / socks5://host:port）。空串=直连。
// 返回 "OK:proxy" / "OK:direct" / "ERR:<原因>"。
func SetNetworkProxy(proxy string) string {
	if err := netx.SetProxy(proxy); err != nil {
		return "ERR:" + err.Error()
	}
	if proxy == "" {
		return "OK:direct"
	}
	return "OK:proxy"
}

// GetNetworkProxy 返回当前代理串（空=直连）。
func GetNetworkProxy() string { return netx.CurrentProxy() }
