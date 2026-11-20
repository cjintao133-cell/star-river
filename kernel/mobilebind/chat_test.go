package mobilebind

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// recSink 记录回调序列（测试用）。
type recSink struct {
	deltas []string
	done   string
	err    string
}

func (s *recSink) OnDelta(t string) { s.deltas = append(s.deltas, t) }
func (s *recSink) OnDone(m string)  { s.done = m }
func (s *recSink) OnError(m string) { s.err = m }

// 空 endpoint/model → 仅一次 OnError，不触网。
func TestChatStreamValidatesConfig(t *testing.T) {
	s := &recSink{}
	ChatStream("", "k", "", "", "hi", s)
	if s.err == "" || s.done != "" || len(s.deltas) != 0 {
		t.Errorf("空配置应只 OnError: %+v", s)
	}
}

// key 不应出现在错误串里（红线 3）。
func TestChatStreamRedactsKey(t *testing.T) {
	// endpoint 非法但带 key 的场景：走 newTest? 用 build 失败的路径不便触网。
	// 直接验证 redactKey。
	if got := redactKey("boom sk-SECRET bad", "sk-SECRET"); got != "boom [REDACTED] bad" {
		t.Errorf("redactKey = %q", got)
	}
}

func TestJSONEscape(t *testing.T) {
	cases := map[string]string{
		`a"b`:   `a\"b`,
		`a\b`:   `a\\b`,
		"a\nb":  `a\nb`,
		"plain": "plain",
		"中文":    "中文",
	}
	for in, want := range cases {
		if got := jsonEscape(in); got != want {
			t.Errorf("jsonEscape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestListModelsEmptyEndpoint(t *testing.T) {
	if got := ListModels("", "k"); got != "ERR:endpoint 未配置" {
		t.Errorf("ListModels empty = %q", got)
	}
}

func TestChatStreamAgainstMockServer(t *testing.T) {
	sse := "data: {\"id\":\"x\",\"model\":\"mock-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"你\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"x\",\"model\":\"mock-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"好\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"x\",\"model\":\"mock-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(sse))
	}))
	defer srv.Close()
	s := &recSink{}
	ChatStream(srv.URL+"/v1", "k", "mock-model", "人设", "hi", s)
	if s.err != "" {
		t.Fatalf("unexpected err: %s", s.err)
	}
	if len(s.deltas) != 2 || s.deltas[0] != "你" || s.deltas[1] != "好" {
		t.Errorf("deltas = %v", s.deltas)
	}
	if s.done != "mock-model" {
		t.Errorf("done = %q", s.done)
	}
}

func TestChatStreamServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":{"message":"boom sk-KEY"}}`))
	}))
	defer srv.Close()
	s := &recSink{}
	ChatStream(srv.URL+"/v1", "sk-KEY", "m", "", "hi", s)
	if s.err == "" || s.done != "" {
		t.Errorf("应 OnError: %+v", s)
	}
	if contains(s.err, "sk-KEY") {
		t.Errorf("key 泄漏: %s", s.err)
	}
}

func TestListModelsMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"m1"},{"id":"m2"}]}`))
	}))
	defer srv.Close()
	got := ListModels(srv.URL+"/v1", "k")
	if got != `[{"id":"m1"},{"id":"m2"}]` {
		t.Errorf("ListModels = %q", got)
	}
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
