package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

const testKey = "sk-TEST-KEY-0001"

// mockState 记录 mock server 收到的请求与注入 sleep 的等待值（并发安全）。
type mockState struct {
	mu      sync.Mutex
	bodies  [][]byte
	hdrs    []http.Header
	methods []string
	paths   []string
	waits   []time.Duration
}

func (m *mockState) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.bodies)
}

func (m *mockState) body(i int) []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bodies[i]
}

func (m *mockState) hdr(i int) http.Header {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hdrs[i]
}

func (m *mockState) method(i int) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.methods[i]
}

func (m *mockState) path(i int) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.paths[i]
}

func (m *mockState) sleepWaits() []time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]time.Duration(nil), m.waits...)
}

// newTestClient httptest 仅监听 127.0.0.1（网络纪律）。sleep 注入免真实等待。
func newTestClient(t *testing.T, h func(w http.ResponseWriter, n int, body []byte)) (*Client, *mockState) {
	t.Helper()
	st := &mockState{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		st.mu.Lock()
		st.bodies = append(st.bodies, b)
		st.hdrs = append(st.hdrs, r.Header.Clone())
		st.methods = append(st.methods, r.Method)
		st.paths = append(st.paths, r.URL.Path)
		n := len(st.bodies)
		st.mu.Unlock()
		h(w, n, b)
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL+"/v1/", testKey)
	c.BackoffBase = time.Millisecond
	c.sleep = func(ctx context.Context, d time.Duration) error {
		st.mu.Lock()
		st.waits = append(st.waits, d)
		st.mu.Unlock()
		return ctx.Err()
	}
	return c, st
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return b
}

func writeSSE(w http.ResponseWriter, b []byte) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Request-Id", "req-test")
	w.Write(b)
}

func writeJSON(w http.ResponseWriter, status int, b []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-Id", "req-test")
	w.WriteHeader(status)
	w.Write(b)
}

func textReq() ChatRequest {
	return ChatRequest{
		Model:     "gpt-4o-mini",
		MaxTokens: 64,
		Messages: []types.Message{
			{Role: types.RoleUser, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: "hi"}}},
		},
	}
}

func asAPIError(t *testing.T, err error) *APIError {
	t.Helper()
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *APIError, got %T: %v", err, err)
	}
	return ae
}

func TestChatP1ExplicitFieldsAndBasicStream(t *testing.T) {
	basic := fixture(t, "stream_basic.sse")
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) { writeSSE(w, basic) })
	var evs []types.StreamEvent
	res, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) { evs = append(evs, e) })
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	// P1：stream/model/max_tokens/messages 全部显式出现在请求体。
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(st.body(0), &raw); err != nil {
		t.Fatalf("request body: %v", err)
	}
	for _, k := range []string{"model", "max_tokens", "stream", "messages"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("P1: request field %q missing", k)
		}
	}
	if string(raw["stream"]) != "true" || string(raw["max_tokens"]) != "64" || string(raw["model"]) != `"gpt-4o-mini"` {
		t.Errorf("P1 values: %s %s %s", raw["stream"], raw["max_tokens"], raw["model"])
	}
	if got := st.hdr(0).Get("Authorization"); got != "Bearer "+testKey {
		t.Errorf("Authorization header wrong")
	}
	if got := st.hdr(0).Get("Accept"); got != "text/event-stream" {
		t.Errorf("Accept = %q", got)
	}
	if st.path(0) != "/v1/chat/completions" {
		t.Errorf("path = %q", st.path(0))
	}
	want := []types.StreamEvent{
		{Kind: types.StreamDelta, Text: "Hello"},
		{Kind: types.StreamDelta, Text: " world"},
		{Kind: types.StreamDone, Model: "gpt-4o-mini", RequestID: "req-test"},
	}
	if !reflect.DeepEqual(evs, want) {
		t.Errorf("events = %+v, want %+v", evs, want)
	}
	if res.Model != "gpt-4o-mini" || res.FinishReason != "stop" || res.Degraded || res.RequestID != "req-test" {
		t.Errorf("result = %+v", res)
	}
	if res.Usage != (Usage{PromptTokens: 9, CompletionTokens: 2, TotalTokens: 11}) {
		t.Errorf("usage = %+v", res.Usage)
	}
}

func TestChatReasoningEvents(t *testing.T) {
	b := fixture(t, "stream_reasoning.sse")
	c, _ := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) { writeSSE(w, b) })
	var evs []types.StreamEvent
	if _, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) { evs = append(evs, e) }); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	want := []types.StreamEvent{
		{Kind: types.StreamReasoning, Text: "Let me "},
		{Kind: types.StreamReasoning, Text: "think."},
		{Kind: types.StreamDelta, Text: "42"},
		{Kind: types.StreamDone, Model: "deepseek-reasoner", RequestID: "req-test"},
	}
	if !reflect.DeepEqual(evs, want) {
		t.Errorf("events = %+v, want %+v", evs, want)
	}
}

func TestChatToolCallAggregation(t *testing.T) {
	b := fixture(t, "stream_toolcalls.sse")
	c, _ := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) { writeSSE(w, b) })
	var evs []types.StreamEvent
	res, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) { evs = append(evs, e) })
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	want := []types.StreamEvent{
		{Kind: types.StreamToolCall, CallIndex: 0, Call: &types.ToolCall{ID: "call_a1", Name: "get_weather", Arguments: `{"city":"Paris"}`}},
		{Kind: types.StreamToolCall, CallIndex: 1, Call: &types.ToolCall{ID: "call_b2", Name: "get_time", Arguments: `{}`}},
		{Kind: types.StreamDone, Model: "gpt-4o-mini", RequestID: "req-test"},
	}
	if !reflect.DeepEqual(evs, want) {
		t.Errorf("events = %+v, want %+v", evs, want)
	}
	if res.FinishReason != "tool_calls" {
		t.Errorf("finish = %q", res.FinishReason)
	}
}

func TestChatStreamErrorMidP5(t *testing.T) {
	b := fixture(t, "stream_error_mid.sse")
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) { writeSSE(w, b) })
	var evs []types.StreamEvent
	_, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) { evs = append(evs, e) })
	ae := asAPIError(t, err)
	if ae.Kind != KindStream || ae.Code != "bad_gateway" {
		t.Errorf("增补⑤: kind=%s code=%s", ae.Kind, ae.Code)
	}
	if st.count() != 1 {
		t.Errorf("P6: 流中途失败不得重试, requests=%d", st.count())
	}
	want := []types.StreamEvent{
		{Kind: types.StreamDelta, Text: "partial answer"},
		{Kind: types.StreamError, ErrCode: KindStream, RequestID: "req-test"},
	}
	if !reflect.DeepEqual(evs, want) {
		t.Errorf("events = %+v, want %+v", evs, want)
	}
}

func TestChatTruncatedStreamP5(t *testing.T) {
	b := fixture(t, "stream_truncated.sse")
	c, _ := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) { writeSSE(w, b) })
	var evs []types.StreamEvent
	_, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) { evs = append(evs, e) })
	ae := asAPIError(t, err)
	if ae.Kind != KindStream || !strings.Contains(ae.Message, "without") {
		t.Errorf("增补⑤截断: kind=%s msg=%q", ae.Kind, ae.Message)
	}
	if n := len(evs); n == 0 || evs[n-1].Kind != types.StreamError {
		t.Errorf("last event must be error, evs=%+v", evs)
	}
}

func TestChatNoDoneButFinishOK(t *testing.T) {
	b := fixture(t, "stream_no_done.sse")
	c, _ := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) { writeSSE(w, b) })
	res, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) {})
	if err != nil {
		t.Fatalf("缺[DONE]但有finish_reason应成功: %v", err)
	}
	if res.FinishReason != "stop" {
		t.Errorf("finish = %q", res.FinishReason)
	}
}

func TestChatBadJSONP5(t *testing.T) {
	b := fixture(t, "stream_badjson.sse")
	c, _ := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) { writeSSE(w, b) })
	_, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) {})
	if ae := asAPIError(t, err); ae.Kind != KindProtocol {
		t.Errorf("kind = %s", ae.Kind)
	}
}

func TestChatOversizeLineP5(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: "+strings.Repeat("x", 2<<20)+"\n\n")
	})
	_, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) {})
	ae := asAPIError(t, err)
	if ae.Kind != KindProtocol || !strings.Contains(ae.Message, "1MiB") {
		t.Errorf("kind=%s msg=%q", ae.Kind, ae.Message)
	}
}

func TestChatCommentsTolerated(t *testing.T) {
	b := fixture(t, "stream_comments.sse")
	c, _ := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) { writeSSE(w, b) })
	var evs []types.StreamEvent
	if _, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) { evs = append(evs, e) }); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	want := []types.StreamEvent{
		{Kind: types.StreamDelta, Text: "Hi"},
		{Kind: types.StreamDone, Model: "gpt-4o-mini", RequestID: "req-test"},
	}
	if !reflect.DeepEqual(evs, want) {
		t.Errorf("events = %+v, want %+v", evs, want)
	}
}
