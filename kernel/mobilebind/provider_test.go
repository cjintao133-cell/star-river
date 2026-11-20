package mobilebind

// 多协议 provider 路由测试：切到 anthropic/gemini 时，chat 请求打到对应路径。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestSetChatProviderValidation(t *testing.T) {
	defer SetChatProvider("openai")
	if r := SetChatProvider("claude"); r != "OK" || ChatProvider() != "anthropic" {
		t.Fatalf("claude 应映射 anthropic，got %s/%s", r, ChatProvider())
	}
	if r := SetChatProvider("google"); r != "OK" || ChatProvider() != "gemini" {
		t.Fatalf("google 应映射 gemini")
	}
	if r := SetChatProvider("openai"); r != "OK" || ChatProvider() != "openai" {
		t.Fatalf("openai")
	}
	if r := SetChatProvider("bogus"); !strings.HasPrefix(r, "ERR:") {
		t.Fatalf("未知 provider 应报错")
	}
}

func TestProviderRoutingAnthropic(t *testing.T) {
	var mu sync.Mutex
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock(); path = r.URL.Path; mu.Unlock()
		w.WriteHeader(http.StatusBadRequest) // 触发错误即止，仅验证路径
		w.Write([]byte(`{"error":{"message":"mock"}}`))
	}))
	defer srv.Close()
	SetChatProvider("anthropic")
	defer SetChatProvider("openai")
	s := &recSink{}
	ChatStreamTools(srv.URL, "k", "claude-x", "", "hi", s)
	mu.Lock(); p := path; mu.Unlock()
	if !strings.HasSuffix(p, "/messages") {
		t.Fatalf("anthropic 应打 /messages，实际 %q", p)
	}
}

func TestProviderRoutingGemini(t *testing.T) {
	var mu sync.Mutex
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock(); path = r.URL.Path; mu.Unlock()
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"mock"}}`))
	}))
	defer srv.Close()
	SetChatProvider("gemini")
	defer SetChatProvider("openai")
	s := &recSink{}
	ChatStreamTools(srv.URL, "k", "gemini-x", "", "hi", s)
	mu.Lock(); p := path; mu.Unlock()
	if !strings.Contains(p, "generateContent") && !strings.Contains(p, "streamGenerateContent") {
		t.Fatalf("gemini 应打 generateContent，实际 %q", p)
	}
}
