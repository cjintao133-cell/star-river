package anthropic

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

	"star-river/kernel/core/types"
)

const testKey = "sk-ant-TESTKEY-0001"

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
	c := New(srv.URL+"/v1", testKey)
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
	w.Header().Set("Request-Id", "req-test")
	w.Write(b)
}

func writeJSON(w http.ResponseWriter, status int, b []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Request-Id", "req-test")
	w.WriteHeader(status)
	w.Write(b)
}

func textReq() ChatRequest {
	return ChatRequest{
		Model:     "claude-3-5-sonnet-20241022",
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
	// P1：model/max_tokens/stream/messages 全部显式出现在请求体。
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(st.body(0), &raw); err != nil {
		t.Fatalf("request body: %v", err)
	}
	for _, k := range []string{"model", "max_tokens", "stream", "messages"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("P1: request field %q missing", k)
		}
	}
	if string(raw["stream"]) != "true" || string(raw["max_tokens"]) != "64" || string(raw["model"]) != `"claude-3-5-sonnet-20241022"` {
		t.Errorf("P1 values: %s %s %s", raw["stream"], raw["max_tokens"], raw["model"])
	}
	// Header 契约：x-api-key + anthropic-version；无 Authorization Bearer。
	if got := st.hdr(0).Get("x-api-key"); got != testKey {
		t.Errorf("x-api-key = %q", got)
	}
	if got := st.hdr(0).Get("anthropic-version"); got != "2023-06-01" {
		t.Errorf("anthropic-version = %q", got)
	}
	if got := st.hdr(0).Get("Authorization"); got != "" {
		t.Errorf("Anthropic 不得用 Authorization 头, got %q", got)
	}
	if got := st.hdr(0).Get("Accept"); got != "text/event-stream" {
		t.Errorf("Accept = %q", got)
	}
	if st.path(0) != "/v1/messages" {
		t.Errorf("path = %q", st.path(0))
	}
	want := []types.StreamEvent{
		{Kind: types.StreamDelta, Text: "Hello"},
		{Kind: types.StreamDelta, Text: " world"},
		{Kind: types.StreamDone, Model: "claude-3-5-sonnet-20241022", RequestID: "req-test"},
	}
	if !reflect.DeepEqual(evs, want) {
		t.Errorf("events = %+v, want %+v", evs, want)
	}
	if res.Model != "claude-3-5-sonnet-20241022" || res.FinishReason != "end_turn" || res.Degraded || res.RequestID != "req-test" {
		t.Errorf("result = %+v", res)
	}
	if res.Usage != (Usage{InputTokens: 9, OutputTokens: 2}) {
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
		{Kind: types.StreamDone, Model: "claude-3-7-sonnet", RequestID: "req-test"},
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
		{Kind: types.StreamToolCall, CallIndex: 0, Call: &types.ToolCall{ID: "toolu_a1", Name: "get_weather", Arguments: `{"city":"Paris"}`}},
		{Kind: types.StreamToolCall, CallIndex: 1, Call: &types.ToolCall{ID: "toolu_b2", Name: "get_time", Arguments: `{}`}},
		{Kind: types.StreamDone, Model: "claude-3-5-sonnet-20241022", RequestID: "req-test"},
	}
	if !reflect.DeepEqual(evs, want) {
		t.Errorf("events = %+v, want %+v", evs, want)
	}
	if res.FinishReason != "tool_use" {
		t.Errorf("finish = %q", res.FinishReason)
	}
}

func TestChatStreamErrorMidP5(t *testing.T) {
	b := fixture(t, "stream_error_mid.sse")
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) { writeSSE(w, b) })
	var evs []types.StreamEvent
	_, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) { evs = append(evs, e) })
	ae := asAPIError(t, err)
	if ae.Kind != KindStream || ae.Code != "overloaded_error" {
		t.Errorf("增补⑤: kind=%s code=%s", ae.Kind, ae.Code)
	}
	if st.count() != 1 {
		t.Errorf("P6: 流内错误事件不得重试, requests=%d", st.count())
	}
	want := []types.StreamEvent{
		{Kind: types.StreamDelta, Text: "partial answer"},
		{Kind: types.StreamError, ErrCode: KindStream, RequestID: "req-test"},
	}
	if !reflect.DeepEqual(evs, want) {
		t.Errorf("events = %+v, want %+v", evs, want)
	}
}

func TestChatTruncatedNoMessageStopP5(t *testing.T) {
	b := fixture(t, "stream_truncated.sse")
	c, _ := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) { writeSSE(w, b) })
	var evs []types.StreamEvent
	_, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) { evs = append(evs, e) })
	ae := asAPIError(t, err)
	if ae.Kind != KindStream || !strings.Contains(ae.Message, "message_stop") {
		t.Errorf("增补⑤截断: kind=%s msg=%q", ae.Kind, ae.Message)
	}
	if n := len(evs); n == 0 || evs[n-1].Kind != types.StreamError {
		t.Errorf("last event must be error, evs=%+v", evs)
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

func TestChatCommentsAndPingTolerated(t *testing.T) {
	b := fixture(t, "stream_comments.sse")
	c, _ := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) { writeSSE(w, b) })
	var evs []types.StreamEvent
	if _, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) { evs = append(evs, e) }); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	want := []types.StreamEvent{
		{Kind: types.StreamDelta, Text: "Hi"},
		{Kind: types.StreamDone, Model: "claude-3-5-sonnet-20241022", RequestID: "req-test"},
	}
	if !reflect.DeepEqual(evs, want) {
		t.Errorf("events = %+v, want %+v", evs, want)
	}
}

// blockAfterReader 先吐出 data，之后永久阻塞（永不 EOF），模拟"发了 message_stop 但连接不关"。
type blockAfterReader struct {
	data []byte
}

func (b *blockAfterReader) Read(p []byte) (int, error) {
	if len(b.data) > 0 {
		n := copy(p, b.data)
		b.data = b.data[n:]
		return n, nil
	}
	select {} // 永久阻塞
}

// message_stop 收到即完成：即使连接不反回 EOF，也不应挂起/误判超时。
func TestConsumeStreamEarlyReturnOnMessageStop(t *testing.T) {
	sse := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\",\"model\":\"claude-x\",\"usage\":{\"input_tokens\":1}}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	c := New("http://127.0.0.1", testKey)
	r := &blockAfterReader{data: []byte(sse)}
	type ret struct {
		res *ChatResult
		err error
	}
	ch := make(chan ret, 1)
	go func() {
		res, err := c.consumeStream(r, "rid", func(types.StreamEvent) {})
		ch <- ret{res, err}
	}()
	select {
	case got := <-ch:
		if got.err != nil {
			t.Fatalf("应成功, got err=%v", got.err)
		}
		if got.res.FinishReason == "" && got.res.Model != "claude-x" {
			t.Logf("res=%+v", got.res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("message_stop 后未提前返回（仍在等 EOF）")
	}
}
