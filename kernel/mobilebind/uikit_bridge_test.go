package mobilebind

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUIKitBridge(t *testing.T) {
	initTestKernel(t)
	// 产物校验
	if r := UIArtifactValidate(`{"id":"a","kind":"markdown","content":"# hi"}`); r != "OK" {
		t.Fatalf("合法产物应 OK: %s", r)
	}
	if r := UIArtifactValidate(`{"id":"a","kind":"bogus"}`); !strings.HasPrefix(r, "ERR") {
		t.Fatalf("非法 kind 应 ERR: %s", r)
	}
	if r := UIArtifactValidate(`{bad`); !strings.HasPrefix(r, "ERR") {
		t.Fatalf("坏 json 应 ERR")
	}
	// 降级 markdown
	if md := UIArtifactToMarkdown(`{"id":"a","kind":"code","content":"x=1"}`); !strings.Contains(md, "```") {
		t.Fatalf("code 转 md 不对: %s", md)
	}
	// 组件树校验
	if r := UIRenderSpecValidate(`{"type":"card","children":[{"type":"text"}]}`); r != "OK" {
		t.Fatalf("合法树应 OK: %s", r)
	}
	if r := UIRenderSpecValidate(`{"type":"script"}`); !strings.HasPrefix(r, "ERR") {
		t.Fatalf("目录外组件应 ERR: %s", r)
	}
	// 目录
	var cat map[string]any
	_ = json.Unmarshal([]byte(UICatalogJSON()), &cat)
	if len(cat["types"].([]any)) < 10 {
		t.Fatalf("目录类型太少: %+v", cat)
	}
	// MCP 资源
	res := UIResourceForMcp(`{"id":"v1","kind":"html","title":"t","content":"<html><head></head><body>x</body></html>"}`, "")
	if !strings.Contains(res, "ui://v1") || !strings.Contains(res, "Content-Security-Policy") {
		t.Fatalf("MCP 资源不对: %s", res)
	}
	// 非 html 报错
	if r := UIResourceForMcp(`{"id":"m","kind":"markdown","content":"x"}`, ""); !strings.HasPrefix(r, "ERR") {
		t.Fatalf("非 html 应 ERR: %s", r)
	}
	// CSP
	if csp := UISandboxCSP(""); !strings.Contains(csp, "script-src 'none'") {
		t.Fatalf("默认 CSP 不对: %s", csp)
	}
}
