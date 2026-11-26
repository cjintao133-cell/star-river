package netx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSetProxyAffectsClient(t *testing.T) {
	defer SetProxy("")

	if err := SetProxy("http://127.0.0.1:7890"); err != nil {
		t.Fatalf("set proxy: %v", err)
	}
	if CurrentProxy() != "http://127.0.0.1:7890" {
		t.Fatalf("current proxy = %q", CurrentProxy())
	}
	c := Client(time.Second)
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport not *http.Transport: %T", c.Transport)
	}
	req := httptest.NewRequest("GET", "https://api.openai.com/v1/models", nil)
	u, err := tr.Proxy(req)
	if err != nil {
		t.Fatalf("proxy func err: %v", err)
	}
	if u == nil || u.Host != "127.0.0.1:7890" {
		t.Fatalf("expected proxy host 127.0.0.1:7890, got %v", u)
	}
	// 仍拒绝重定向
	if c.CheckRedirect == nil {
		t.Fatal("redirect policy missing")
	}
}

func TestSetProxyInvalid(t *testing.T) {
	defer SetProxy("")
	if err := SetProxy("not-a-url"); err == nil {
		t.Fatal("expected error for url without scheme/host")
	}
	if err := SetProxy("http://hostonly"); err != nil {
		// host-only with scheme is acceptable
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestSetProxyClear(t *testing.T) {
	if err := SetProxy("http://127.0.0.1:1"); err != nil {
		t.Fatal(err)
	}
	if err := SetProxy(""); err != nil {
		t.Fatal(err)
	}
	if CurrentProxy() != "" {
		t.Fatalf("expected cleared, got %q", CurrentProxy())
	}
}
