package uikit

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestArtifactValidate(t *testing.T) {
	ok := Artifact{ID: "a1", Kind: KindMarkdown, Content: "# hi"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("应通过: %v", err)
	}
	if err := (Artifact{Kind: KindMarkdown, Content: "x"}).Validate(); err == nil {
		t.Fatal("缺 id 应报错")
	}
	if err := (Artifact{ID: "x", Kind: "bogus"}).Validate(); err == nil {
		t.Fatal("未知 kind 应报错")
	}
	if err := (Artifact{ID: "x", Kind: KindChart}).Validate(); err == nil {
		t.Fatal("chart 缺 data 应报错")
	}
	if err := (Artifact{ID: "x", Kind: KindChart, Data: json.RawMessage("{bad")}).Validate(); err == nil {
		t.Fatal("chart 坏 json 应报错")
	}
	if err := (Artifact{ID: "x", Kind: KindTable, Data: json.RawMessage(`[["a","b"]]`)}).Validate(); err != nil {
		t.Fatalf("合法 table 应通过: %v", err)
	}
}

func TestCatalogValidate(t *testing.T) {
	c := DefaultCatalog()
	good := Component{Type: "card", Children: []Component{
		{Type: "text"}, {Type: "button"}, {Type: "row", Children: []Component{{Type: "image"}}},
	}}
	if err := c.Validate(good); err != nil {
		t.Fatalf("合法树应通过: %v", err)
	}
	bad := Component{Type: "script"} // 不在目录
	if err := c.Validate(bad); err == nil {
		t.Fatal("目录外组件应拒绝")
	}
	// 深度超限
	deep := Component{Type: "card"}
	cur := &deep
	for i := 0; i < 30; i++ {
		cur.Children = []Component{{Type: "column"}}
		cur = &cur.Children[0]
	}
	if err := c.Validate(deep); err == nil {
		t.Fatal("超深应拒绝")
	}
}

func TestCSPAndSandbox(t *testing.T) {
	p := SecureDefault()
	csp := p.CSP()
	if !strings.Contains(csp, "script-src 'none'") || !strings.Contains(csp, "connect-src 'none'") {
		t.Fatalf("安全默认应禁脚本/网络: %s", csp)
	}
	if p.SandboxAttrs() != "" {
		t.Fatalf("默认不应 allow-scripts")
	}
	p2 := SandboxPolicy{AllowScripts: true, AllowNetwork: true, AllowedHosts: []string{"https://api.x.com"}}
	csp2 := p2.CSP()
	if !strings.Contains(csp2, "script-src 'unsafe-inline'") || !strings.Contains(csp2, "https://api.x.com") {
		t.Fatalf("放宽后 CSP 不对: %s", csp2)
	}
	if p2.SandboxAttrs() != "allow-scripts" {
		t.Fatalf("sandbox attrs 不对")
	}
}

func TestToMCPResource(t *testing.T) {
	p := SecureDefault()
	html := Artifact{ID: "view1", Kind: KindHTML, Title: "报表", Content: "<html><head></head><body>hi</body></html>"}
	res, err := ToMCPResource(html, p)
	if err != nil {
		t.Fatal(err)
	}
	if res["uri"] != "ui://view1" || res["mimeType"] != "text/html" {
		t.Fatalf("资源元信息不对: %+v", res)
	}
	text := res["text"].(string)
	if !strings.Contains(text, "Content-Security-Policy") {
		t.Fatalf("应注入 CSP meta: %s", text)
	}
	meta := res["meta"].(map[string]any)
	if meta["sandbox"] != "" {
		t.Fatalf("安全默认 sandbox 应空: %+v", meta)
	}
	// 非 html 报错
	if _, err := ToMCPResource(Artifact{ID: "m", Kind: KindMarkdown, Content: "x"}, p); err == nil {
		t.Fatal("非 html 应报错")
	}
}

func TestHasScriptAndMarkdown(t *testing.T) {
	if !HasScript(`<div onclick="x()">a</div>`) {
		t.Fatal("应检出 onclick")
	}
	if HasScript("plain text") {
		t.Fatal("纯文本不应误判")
	}
	if ToMarkdown(Artifact{Kind: KindCode, Content: "x=1"}) != "```\nx=1\n```" {
		t.Fatal("code 转 markdown 不对")
	}
	if !strings.Contains(ToMarkdown(Artifact{Kind: KindTable, Title: "t", Data: json.RawMessage(`[1]`)}), "```json") {
		t.Fatal("table 转 markdown 不对")
	}
	if ToMarkdown(Artifact{Kind: KindMarkdown, Content: "# md"}) != "# md" {
		t.Fatal("markdown 应原样")
	}
}
