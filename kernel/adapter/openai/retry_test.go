package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"star-river/kernel/core/types"
)

func TestChatStreamUnsupportedDegradeP3(t *testing.T) {
	unsup := fixture(t, "err_stream_unsupported.json")
	okJSON := fixture(t, "json_ok.json")
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) {
		if n == 1 {
			writeJSON(w, 400, unsup)
			return
		}
		writeJSON(w, 200, okJSON)
	})
	var evs []types.StreamEvent
	res, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) { evs = append(evs, e) })
	if err != nil {
		t.Fatalf("P3 degrade: %v", err)
	}
	if !res.Degraded {
		t.Error("P3: Degraded must be true")
	}
	if st.count() != 2 {
		t.Fatalf("P3: requests = %d", st.count())
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(st.body(1), &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["stream"]) != "false" {
		t.Errorf("P3: retry stream = %s", raw["stream"])
	}
	if got := st.hdr(1).Get("Accept"); got != "application/json" {
		t.Errorf("P3 retry Accept = %q", got)
	}
	want := []types.StreamEvent{
		{Kind: types.StreamReasoning, Text: "Thinking."},
		{Kind: types.StreamDelta, Text: "Done."},
		{Kind: types.StreamToolCall, CallIndex: 0, Call: &types.ToolCall{ID: "call_j1", Name: "get_time", Arguments: `{}`}},
		{Kind: types.StreamDone, Model: "deepseek-reasoner", RequestID: "req-test"},
	}
	if !reflect.DeepEqual(evs, want) {
		t.Errorf("P3: 降级前不得发假警报事件, evs=%+v", evs)
	}
}

func TestChatDegradeFailureSingleErrorP3(t *testing.T) {
	unsup := fixture(t, "err_stream_unsupported.json")
	bad := fixture(t, "json_error200.json")
	c, _ := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) {
		if n == 1 {
			writeJSON(w, 400, unsup)
			return
		}
		writeJSON(w, 200, bad)
	})
	var evs []types.StreamEvent
	_, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) { evs = append(evs, e) })
	ae := asAPIError(t, err)
	if ae.Kind != KindProtocol || ae.Code != "503" {
		t.Errorf("增补⑤ 200带error: kind=%s code=%s", ae.Kind, ae.Code)
	}
	nerr := 0
	for _, e := range evs {
		if e.Kind == types.StreamError {
			nerr++
		}
	}
	if nerr != 1 {
		t.Errorf("error events = %d, want 1, evs=%+v", nerr, evs)
	}
}

func TestChat429RetryAfterP6(t *testing.T) {
	basic := fixture(t, "stream_basic.sse")
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) {
		if n == 1 {
			w.Header().Set("Retry-After", "2")
			writeJSON(w, 429, []byte(`{"error":{"message":"slow down"}}`))
			return
		}
		writeSSE(w, basic)
	})
	if _, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) {}); err != nil {
		t.Fatalf("P6 429 retry: %v", err)
	}
	if st.count() != 2 {
		t.Errorf("requests = %d", st.count())
	}
	if w := st.sleepWaits(); !reflect.DeepEqual(w, []time.Duration{2 * time.Second}) {
		t.Errorf("P6: Retry-After 必须优先于退避, waits=%v", w)
	}
}

func TestChat5xxBackoffP6(t *testing.T) {
	basic := fixture(t, "stream_basic.sse")
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) {
		if n <= 2 {
			writeJSON(w, 500, []byte(`{"error":{"message":"boom"}}`))
			return
		}
		writeSSE(w, basic)
	})
	if _, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) {}); err != nil {
		t.Fatalf("P6 5xx retry: %v", err)
	}
	if st.count() != 3 {
		t.Errorf("requests = %d", st.count())
	}
	if w := st.sleepWaits(); !reflect.DeepEqual(w, []time.Duration{time.Millisecond, 2 * time.Millisecond}) {
		t.Errorf("P6 exponential backoff waits=%v", w)
	}
}

func TestChatRetryExhaustedP6(t *testing.T) {
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) {
		writeJSON(w, 503, []byte(`{"error":{"message":"down"}}`))
	})
	var evs []types.StreamEvent
	_, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) { evs = append(evs, e) })
	if ae := asAPIError(t, err); ae.Kind != KindServer {
		t.Errorf("kind = %s", ae.Kind)
	}
	if st.count() != 3 {
		t.Errorf("P6 上限2: requests = %d, want 3", st.count())
	}
	if n := len(evs); n == 0 || evs[n-1].Kind != types.StreamError || evs[n-1].ErrCode != KindServer {
		t.Errorf("last event = %+v", evs)
	}
}

func TestChat401NoRetryRedactP4(t *testing.T) {
	body := []byte(`{"error":{"message":"key ` + testKey + ` rejected","code":"invalid_api_key"}}`)
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) { writeJSON(w, 401, body) })
	_, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) {})
	ae := asAPIError(t, err)
	if ae.Kind != KindAuth || ae.Status != 401 || ae.Code != "invalid_api_key" {
		t.Errorf("kind=%s status=%d code=%s", ae.Kind, ae.Status, ae.Code)
	}
	if ae.RequestID != "req-test" {
		t.Errorf("P4 request id = %q", ae.RequestID)
	}
	if strings.Contains(ae.Error(), testKey) || !strings.Contains(ae.Message, "[REDACTED]") {
		t.Errorf("红线3 脱敏失败: %q", ae.Error())
	}
	if st.count() != 1 || len(st.sleepWaits()) != 0 {
		t.Errorf("P6: 401 不得重试, requests=%d", st.count())
	}
}

func TestChat400NoRetry(t *testing.T) {
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) {
		writeJSON(w, 400, []byte(`{"error":{"message":"bad field foo"}}`))
	})
	_, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) {})
	if ae := asAPIError(t, err); ae.Kind != KindBadRequest {
		t.Errorf("kind = %s", ae.Kind)
	}
	if st.count() != 1 {
		t.Errorf("requests = %d", st.count())
	}
}

func TestChatTimeout(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) {
		time.Sleep(200 * time.Millisecond)
		writeJSON(w, 200, []byte(`{}`))
	})
	c.HTTP.Timeout = 50 * time.Millisecond
	c.MaxRetries = 0
	_, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) {})
	if ae := asAPIError(t, err); ae.Kind != KindTimeout {
		t.Errorf("kind = %s, err=%v", ae.Kind, err)
	}
}

func TestChatCanceled(t *testing.T) {
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) {
		writeJSON(w, 200, []byte(`{}`))
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Chat(ctx, textReq(), func(e types.StreamEvent) {})
	if ae := asAPIError(t, err); ae.Kind != KindCanceled {
		t.Errorf("kind = %s", ae.Kind)
	}
	if st.count() != 0 {
		t.Errorf("canceled 不得重试, requests=%d", st.count())
	}
}

func TestChatRequestValidation(t *testing.T) {
	cases := []struct {
		name string
		mut  func(r *ChatRequest)
	}{
		{"no model", func(r *ChatRequest) { r.Model = "" }},
		{"zero max_tokens", func(r *ChatRequest) { r.MaxTokens = 0 }},
		{"no messages", func(r *ChatRequest) { r.Messages = nil }},
		{"invalid role", func(r *ChatRequest) { r.Messages[0].Role = types.Role("bogus") }},
		{"two tool_results", func(r *ChatRequest) {
			r.Messages[0].Role = types.RoleTool
			r.Messages[0].Blocks = []types.ContentBlock{
				{Type: types.BlockToolResult, CallID: "c1", Text: "a"},
				{Type: types.BlockToolResult, CallID: "c2", Text: "b"},
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cl, st := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) {
				writeJSON(w, 200, []byte(`{}`))
			})
			req := textReq()
			c.mut(&req)
			_, err := cl.Chat(context.Background(), req, func(e types.StreamEvent) {})
			if ae := asAPIError(t, err); ae.Kind != KindBadRequest {
				t.Errorf("kind = %s", ae.Kind)
			}
			if st.count() != 0 {
				t.Errorf("本地校验失败不得发请求, requests=%d", st.count())
			}
		})
	}
}

func TestConvertMessagesWire(t *testing.T) {
	basic := fixture(t, "stream_basic.sse")
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) { writeSSE(w, basic) })
	req := ChatRequest{
		Model:     "gpt-4o-mini",
		MaxTokens: 64,
		Messages: []types.Message{
			{Role: types.RoleSystem, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: "sys"}}},
			{Role: types.RoleUser, Blocks: []types.ContentBlock{
				{Type: types.BlockReasoning, Text: "secret thoughts"},
				{Type: types.BlockText, Text: "question"},
			}},
			{Role: types.RoleAssistant, Blocks: []types.ContentBlock{
				{Type: types.BlockToolUse, Call: &types.ToolCall{ID: "call_x", Name: "search", Arguments: `{"q":1}`}},
			}},
			{Role: types.RoleTool, Blocks: []types.ContentBlock{
				{Type: types.BlockToolResult, CallID: "call_x", Text: "result"},
			}},
		},
	}
	if _, err := c.Chat(context.Background(), req, func(e types.StreamEvent) {}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if strings.Contains(string(st.body(0)), "secret thoughts") {
		t.Error("reasoning 块不得回传端点")
	}
	var w struct {
		Messages []wireOutMessage `json:"messages"`
	}
	if err := json.Unmarshal(st.body(0), &w); err != nil {
		t.Fatal(err)
	}
	if len(w.Messages) != 4 {
		t.Fatalf("messages = %d", len(w.Messages))
	}
	if w.Messages[1].Content != "question" {
		t.Errorf("user content = %q", w.Messages[1].Content)
	}
	tc := w.Messages[2].ToolCalls
	if len(tc) != 1 || tc[0].ID != "call_x" || tc[0].Type != "function" || tc[0].Function.Name != "search" || tc[0].Function.Arguments != `{"q":1}` {
		t.Errorf("tool_calls = %+v", tc)
	}
	if w.Messages[3].Role != "tool" || w.Messages[3].ToolCallID != "call_x" || w.Messages[3].Content != "result" {
		t.Errorf("tool message = %+v", w.Messages[3])
	}
}

func TestConsumeJSONFailuresP5(t *testing.T) {
	c := New("http://127.0.0.1", testKey)
	cases := []struct {
		name, body, want string
	}{
		{"no choices", string(fixture(t, "json_no_choices.json")), "no choices"},
		{"missing finish_reason", `{"choices":[{"index":0,"message":{"content":"x"}}]}`, "finish_reason"},
		{"not JSON", `<html>gateway</html>`, "not JSON"},
	}
	for _, cse := range cases {
		t.Run(cse.name, func(t *testing.T) {
			_, err := c.consumeJSON(strings.NewReader(cse.body), "rid", func(e types.StreamEvent) {})
			ae := asAPIError(t, err)
			if ae.Kind != KindProtocol || !strings.Contains(ae.Message, cse.want) {
				t.Errorf("kind=%s msg=%q", ae.Kind, ae.Message)
			}
		})
	}
}
