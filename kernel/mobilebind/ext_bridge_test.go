package mobilebind

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBrowserBridge(t *testing.T) {
	initTestKernel(t)
	if r := BrowserActionValidate(`{"kind":"navigate","url":"https://a"}`); r != "OK" {
		t.Fatalf("合法动作应 OK: %s", r)
	}
	if r := BrowserActionValidate(`{"kind":"click"}`); !strings.HasPrefix(r, "ERR") {
		t.Fatalf("缺 ref 应 ERR: %s", r)
	}
	if r := BrowserActionValidate(`{bad`); !strings.HasPrefix(r, "ERR") {
		t.Fatalf("坏 json 应 ERR")
	}
	page := `{"url":"u","tree":{"role":"root","children":[{"role":"button","name":"提交","ref":"e1"}]}}`
	out := BrowserCompress(page, 50)
	if !strings.Contains(out, `button "提交" [e1]`) {
		t.Fatalf("压缩不对: %s", out)
	}
}

func TestConnectorsBridge(t *testing.T) {
	initTestKernel(t)
	def := `{"id":"gmail","name":"Gmail","auth":{"type":"oauth2","auth_url":"https://accounts.google.com/o/oauth2/v2/auth","token_url":"https://oauth2.googleapis.com/token","scopes":["gmail.readonly"],"client_id":"cid"},"tools":[{"name":"list","description":"列出邮件"},{"name":"send","description":"发"}]}`
	if r := ConnectorsAdd(def); r != "OK" {
		t.Fatalf("add: %s", r)
	}
	if r := ConnectorsAdd(`{bad`); !strings.HasPrefix(r, "ERR") {
		t.Fatalf("坏 json 应 ERR")
	}
	// 持久化：重新读取列表应含 gmail（模拟重开库前同库读取）
	var list []map[string]any
	_ = json.Unmarshal([]byte(ConnectorsList()), &list)
	if len(list) != 1 || list[0]["id"] != "gmail" {
		t.Fatalf("列表不对: %+v", list)
	}
	var tools []map[string]any
	_ = json.Unmarshal([]byte(ConnectorsTools()), &tools)
	if len(tools) != 2 || tools[0]["name"] != "gmail_list" {
		t.Fatalf("工具派生不对: %+v", tools)
	}
	u := ConnectorsAuthURL("gmail", "https://app/cb", "st1")
	if !strings.Contains(u, "response_type=code") || !strings.Contains(u, "state=st1") {
		t.Fatalf("auth url 不对: %s", u)
	}
	if r := ConnectorsAuthURL("nope", "cb", ""); !strings.HasPrefix(r, "ERR") {
		t.Fatalf("不存在连接器应 ERR")
	}
	// 令牌引用
	if r := ConnectorsSaveToken(`{"connector_id":"gmail","secret_id":"gmail.token","scopes":["gmail.readonly"]}`); r != "OK" {
		t.Fatalf("save token: %s", r)
	}
	if tok := ConnectorsGetToken("gmail"); !strings.Contains(tok, "gmail.token") {
		t.Fatalf("get token 不对: %s", tok)
	}
	if tok := ConnectorsGetToken("nope"); !strings.HasPrefix(tok, "ERR") {
		t.Fatalf("无令牌应 ERR")
	}
}
