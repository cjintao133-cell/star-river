package openai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

// T-M7-03 异常注入矩阵（内核侧）：断网 / 模型超时 / key 失效 / 余额不足 / 返回格式错误 五场景，
// 全部满足：不崩溃（不 panic）+ 明确错误分类（*APIError.Kind 且 emit StreamError.ErrCode）+ 可恢复。

func newFaultClient(baseURL string) *Client {
	c := New(baseURL, "sk-secret-key")
	c.MaxRetries = 0 // 加速：不重试
	c.BackoffBase = time.Millisecond
	c.HTTP = &http.Client{Timeout: 3 * time.Second}
	return c
}

func okReq() ChatRequest {
	return ChatRequest{
		Model:     "m",
		MaxTokens: 16,
		Messages:  []types.Message{{ID: "u1", SessionID: "s", Role: types.RoleUser, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: "hi"}}}},
	}
}

// okSSE 一段能成功消费的流式响应。
const okSSE = "data: {\"model\":\"m\",\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\ndata: [DONE]\n\n"

// callCatchingPanic 调用 Chat 并捕获 panic（显式验证"不崩溃"）。error 作为最后一个返回值。
func callCatchingPanic(c *Client, req ChatRequest) (res *ChatResult, panicked bool, events []types.StreamEvent, err error) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
		}
	}()
	emit := func(e types.StreamEvent) { events = append(events, e) }
	res, err = c.Chat(context.Background(), req, emit)
	return res, panicked, events, err
}

// expectFault 断言：不 panic + 返回期望 Kind + emit 中有一条 StreamError{ErrCode: wantKind}。
func expectFault(t *testing.T, c *Client, wantKind string) {
	t.Helper()
	_, panicked, events, err := callCatchingPanic(c, okReq())
	if panicked {
		t.Fatalf("异常场景不应 panic")
	}
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("应返回 *APIError, got %v", err)
	}
	if ae.Kind != wantKind {
		t.Fatalf("错误分类应为 %s, got %s (%v)", wantKind, ae.Kind, err)
	}
	// 明确用户提示通道：emit 了一条 StreamError，ErrCode = Kind。
	found := false
	for _, e := range events {
		if e.Kind == types.StreamError && e.ErrCode == wantKind {
			found = true
		}
	}
	if !found {
		t.Errorf("应向 UI emit StreamError{ErrCode=%s}, 实得 %+v", wantKind, events)
	}
}

// expectRecovered 断言：故障消除后同一客户端可正常恢复。
func expectRecovered(t *testing.T, c *Client) {
	t.Helper()
	res, panicked, _, err := callCatchingPanic(c, okReq())
	if panicked {
		t.Fatal("恢复调用不应 panic")
	}
	if err != nil {
		t.Fatalf("故障消除后应恢复成功, got %v", err)
	}
	if res == nil || res.FinishReason != "stop" {
		t.Fatalf("恢复结果不对: %+v", res)
	}
}

// modeServer 可切换故障模式的 mock 端点。
type modeServer struct {
	mu   sync.Mutex
	mode string // ok|auth|quota|server|protocol|truncated|slow
}

func (m *modeServer) set(mode string) { m.mu.Lock(); m.mode = mode; m.mu.Unlock() }
func (m *modeServer) get() string     { m.mu.Lock(); defer m.mu.Unlock(); return m.mode }

func (m *modeServer) handler(w http.ResponseWriter, r *http.Request) {
	switch m.get() {
	case "auth":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key","code":"invalid_api_key"}}`))
	case "quota":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":{"message":"insufficient quota","code":"insufficient_quota"}}`))
	case "server":
		w.WriteHeader(500)
		_, _ = w.Write([]byte("internal error"))
	case "protocol":
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("data: {this is not json\n\n"))
	case "truncated":
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")) // 无 finish/[DONE]
	case "slow":
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(okSSE))
	default: // ok
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(okSSE))
	}
}

func TestAbnormalKeyInvalid(t *testing.T) {
	ms := &modeServer{mode: "auth"}
	srv := httptest.NewServer(http.HandlerFunc(ms.handler))
	defer srv.Close()
	c := newFaultClient(srv.URL)
	expectFault(t, c, KindAuth)
	// 恢复：换有效 key（模拟用户重填）后正常
	ms.set("ok")
	c2 := newFaultClient(srv.URL)
	c2.apiKey = "sk-new-valid"
	expectRecovered(t, c2)
}

func TestAbnormalQuota(t *testing.T) {
	ms := &modeServer{mode: "quota"}
	srv := httptest.NewServer(http.HandlerFunc(ms.handler))
	defer srv.Close()
	c := newFaultClient(srv.URL)
	expectFault(t, c, KindRateLimit)
	ms.set("ok")
	expectRecovered(t, c)
}

func TestAbnormalServerError(t *testing.T) {
	ms := &modeServer{mode: "server"}
	srv := httptest.NewServer(http.HandlerFunc(ms.handler))
	defer srv.Close()
	c := newFaultClient(srv.URL)
	expectFault(t, c, KindServer)
	ms.set("ok")
	expectRecovered(t, c)
}

func TestAbnormalProtocolBadFormat(t *testing.T) {
	ms := &modeServer{mode: "protocol"}
	srv := httptest.NewServer(http.HandlerFunc(ms.handler))
	defer srv.Close()
	c := newFaultClient(srv.URL)
	expectFault(t, c, KindProtocol)
	ms.set("ok")
	expectRecovered(t, c)
}

func TestAbnormalStreamTruncated(t *testing.T) {
	ms := &modeServer{mode: "truncated"}
	srv := httptest.NewServer(http.HandlerFunc(ms.handler))
	defer srv.Close()
	c := newFaultClient(srv.URL)
	expectFault(t, c, KindStream)
	ms.set("ok")
	expectRecovered(t, c)
}

func TestAbnormalTimeout(t *testing.T) {
	ms := &modeServer{mode: "slow"}
	srv := httptest.NewServer(http.HandlerFunc(ms.handler))
	defer srv.Close()
	c := newFaultClient(srv.URL)
	c.HTTP = &http.Client{Timeout: 40 * time.Millisecond} // 短于服务端 300ms
	expectFault(t, c, KindTimeout)
	// 恢复：放宽超时
	ms.set("ok")
	c.HTTP = &http.Client{Timeout: 3 * time.Second}
	expectRecovered(t, c)
}

func TestAbnormalNetworkDown(t *testing.T) {
	// 指向关闭的端口 → 连接失败（断网/服务不可达）。
	c := newFaultClient("http://127.0.0.1:1")
	expectFault(t, c, KindNetwork)
	// 恢复：切到健康端点（模拟网络恢复/换线路）。
	ms := &modeServer{mode: "ok"}
	srv := httptest.NewServer(http.HandlerFunc(ms.handler))
	defer srv.Close()
	c2 := newFaultClient(srv.URL)
	expectRecovered(t, c2)
}

func TestAbnormalValidationNoPanic(t *testing.T) {
	// 入参非法（缺 model / max_tokens）在联网前即返回分类错误，不 panic。
	c := newFaultClient("http://127.0.0.1:1")
	cases := []ChatRequest{
		{MaxTokens: 1, Messages: okReq().Messages},
		{Model: "m", MaxTokens: 0, Messages: okReq().Messages},
		{Model: "m", MaxTokens: 1},
	}
	for i, req := range cases {
		_, panicked, _, err := callCatchingPanic(c, req)
		if panicked {
			t.Fatalf("case %d 不应 panic", i)
		}
		var ae *APIError
		if !errors.As(err, &ae) || ae.Kind != KindBadRequest {
			t.Errorf("case %d 应为 KindBadRequest, got %v", i, err)
		}
	}
}

func TestAbnormalErrorNoKeyLeak(t *testing.T) {
	// 错误信息不得泄漏 key（红线 3）。
	ms := &modeServer{mode: "server"}
	srv := httptest.NewServer(http.HandlerFunc(ms.handler))
	defer srv.Close()
	c := newFaultClient(srv.URL)
	_, _, _, err := callCatchingPanic(c, okReq())
	if err != nil && strings.Contains(err.Error(), "sk-secret-key") {
		t.Error("错误信息泄漏了 key")
	}
}
