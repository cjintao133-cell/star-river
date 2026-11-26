package mobilebind

// 浏览器自动化桥（纯逻辑部分）：动作校验 + 无障碍树压缩。真实驱动由入口层注入。

import (
	"encoding/json"

	"github.com/cjintao133-cell/star-river/kernel/browser"
)

// BrowserActionValidate 校验浏览器动作 JSON，返回 "OK"/"ERR:..."。
func BrowserActionValidate(actionJSON string) string {
	var a browser.Action
	if err := json.Unmarshal([]byte(actionJSON), &a); err != nil {
		return "ERR:bad json"
	}
	if err := a.Valid(); err != nil {
		return "ERR:" + err.Error()
	}
	return "OK"
}

// BrowserCompress 把页面快照 JSON（browser.Page）压成 token 友好文本。
func BrowserCompress(pageJSON string, maxLines int) string {
	var p browser.Page
	if err := json.Unmarshal([]byte(pageJSON), &p); err != nil {
		return "ERR:bad json"
	}
	return browser.Compress(p, maxLines)
}
