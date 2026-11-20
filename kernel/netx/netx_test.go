package netx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 核心断言：客户端不得跟随重定向到第二 origin（防凭据随 302 外泄）。
func TestClientRefusesRedirect(t *testing.T) {
	hit := false
	dst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer dst.Close()
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dst.URL, http.StatusFound)
	}))
	defer src.Close()

	c := Client(5 * time.Second)
	resp, err := c.Get(src.URL)
	if err != nil {
		t.Fatalf("request err: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Errorf("应原样返回 302 不跟随，实际 status=%d", resp.StatusCode)
	}
	if hit {
		t.Error("发生了跨 origin 重定向跟随——凭据有外泄风险！")
	}
}

// 正常直连不重定向时行为不变。
func TestClientFollowsNoRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	c := Client(5 * time.Second)
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatalf("request err: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("直连应 200，实际 %d", resp.StatusCode)
	}
}
