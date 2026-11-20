package mobilebind

// 应用请求头（T-新增）：给发往用户自配端点的每个请求带上小小的应用标识头。
// 默认恒带 X-Star-River-Client: star-river-kernel/<version>；用户可再设 X-Star-River-Tag 等自定义头。
// 红线3：头值不得含密钥；保留头（Authorization/Content-Type/Accept）由适配器拒绝覆盖。

import (
	"encoding/json"
	"strings"
	"sync"
)

var (
	reqHdrMu  sync.RWMutex
	reqHdrs = map[string]string{}
)

// SetRequestHeaders 设应用额外请求头（JSON 对象 {"k":"v"}）。空串=仅用默认标识头。
func SetRequestHeaders(jsonStr string) string {
	m := map[string]string{}
	if strings.TrimSpace(jsonStr) != "" {
		if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
			return "ERR:bad header json: " + err.Error()
		}
	}
	reqHdrMu.Lock()
	reqHdrs = m
	reqHdrMu.Unlock()
	return "OK"
}

// currentHeaders 返回含默认标识头的头集合（拷贝，供适配器使用）。
func currentHeaders() map[string]string {
	out := map[string]string{"X-Star-River-Client": "star-river-kernel/" + kernelVersion}
	reqHdrMu.RLock()
	for k, v := range reqHdrs {
		out[k] = v
	}
	reqHdrMu.RUnlock()
	return out
}
