package connectors

import (
	"strings"
	"testing"
)

func sampleDef() ConnectorDef {
	return ConnectorDef{
		ID: "gmail", Name: "Gmail",
		Auth:  AuthSpec{Type: AuthOAuth2, AuthURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: "https://oauth2.googleapis.com/token", Scopes: []string{"gmail.readonly"}, ClientID: "cid"},
		Tools: []ToolDef{{Name: "list", Description: "列出邮件"}, {Name: "send", Description: "发邮件"}},
	}
}

func TestRegisterAndTools(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(sampleDef()); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(ConnectorDef{Name: "无id"}); err == nil {
		t.Fatal("缺 id 应报错")
	}
	if len(r.List()) != 1 {
		t.Fatalf("应 1 连接器")
	}
	tools := r.AllTools()
	if len(tools) != 2 || tools[0].Name != "gmail_list" || tools[1].Name != "gmail_send" {
		t.Fatalf("工具派生不对: %+v", tools)
	}
	if d, ok := r.Get("gmail"); !ok || d.Name != "Gmail" {
		t.Fatalf("Get 不对")
	}
}

func TestBuildAuthURL(t *testing.T) {
	u, err := BuildAuthURL(sampleDef(), "https://app/cb", "xyz")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"response_type=code", "client_id=cid", "redirect_uri=", "scope=gmail.readonly", "state=xyz"} {
		if !strings.Contains(u, want) {
			t.Errorf("URL 缺 %q: %s", want, u)
		}
	}
	// 非 oauth2
	api := ConnectorDef{ID: "k", Auth: AuthSpec{Type: AuthAPIKey}}
	if _, err := BuildAuthURL(api, "cb", ""); err == nil {
		t.Fatal("非 oauth2 应报错")
	}
}

func TestNewStateUnique(t *testing.T) {
	a, b := NewState(), NewState()
	if a == b || len(a) < 16 {
		t.Fatalf("state 应唯一且够长: %q %q", a, b)
	}
}

func TestTokenStore(t *testing.T) {
	s := NewMemStore()
	if _, ok, _ := s.Get("x"); ok {
		t.Fatal("初始应无")
	}
	_ = s.Save(TokenRef{ConnectorID: "gmail", SecretID: "gmail.token"})
	if tr, ok, _ := s.Get("gmail"); !ok || tr.SecretID != "gmail.token" {
		t.Fatalf("Get 不对: %+v", tr)
	}
	l, _ := s.List()
	if len(l) != 1 {
		t.Fatalf("List 不对")
	}
	_ = s.Delete("gmail")
	if _, ok, _ := s.Get("gmail"); ok {
		t.Fatal("删除后应无")
	}
}
