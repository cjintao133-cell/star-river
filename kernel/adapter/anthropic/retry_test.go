package anthropic

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

func TestChatP1AndP3Degrade(t *testing.T) {
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
		{Kind: types.StreamToolCall, CallIndex: 0, Call: &types.ToolCall{ID: "toolu_j1", Name: "get_time", Arguments: `{}`}},
		{Kind: types.StreamDone, Model: "claude-3-7-sonnet", RequestID: "req-test"},
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
	if ae.Kind != KindProtocol || ae.Code != "overloaded_error" {
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
			writeJSON(w, 429, []byte(`{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`))
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
			writeJSON(w, 500, []byte(`{"type":"error","error":{"type":"api_error","message":"boom"}}`))
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
		writeJSON(w, 503, []byte(`{"type":"error","error":{"type":"api_error","message":"down"}}`))
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

func TestChat529OverloadedIsServer(t *testing.T) {
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) {
		writeJSON(w, 529, []byte(`{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`))
	})
	c.MaxRetries = 0
	_, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) {})
	if ae := asAPIError(t, err); ae.Kind != KindServer || ae.Code != "overloaded_error" {
		t.Errorf("529 → kind=%s code=%s", ae.Kind, ae.Code)
	}
	if st.count() != 1 {
		t.Errorf("MaxRetries=0 requests=%d", st.count())
	}
}

func TestChat401NoRetryRedactP4(t *testing.T) {
	body := []byte(`{"type":"error","error":{"type":"authentication_error","message":"key ` + testKey + ` rejected"}}`)
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) { writeJSON(w, 401, body) })
	_, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) {})
	ae := asAPIError(t, err)
	if ae.Kind != KindAuth || ae.Status != 401 || ae.Code != "authentication_error" {
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
		writeJSON(w, 400, []byte(`{"type":"error","error":{"type":"invalid_request_error","message":"bad field foo"}}`))
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
		{"image block", func(r *ChatRequest) {
			r.Messages[0].Blocks = []types.ContentBlock{{Type: types.BlockImage, MIMEType: "image/png", Data: []byte{1}}}
		}},
		{"invalid role", func(r *ChatRequest) { r.Messages[0].Role = types.Role("bogus") }},
		{"tool_use without call", func(r *ChatRequest) {
			r.Messages[0].Blocks = []types.ContentBlock{{Type: types.BlockToolUse}}
		}},
		{"tool_use bad arguments", func(r *ChatRequest) {
			r.Messages[0].Blocks = []types.ContentBlock{{Type: types.BlockToolUse, Call: &types.ToolCall{ID: "c1", Name: "f", Arguments: "not json"}}}
		}},
	}
	for _, cse := range cases {
		t.Run(cse.name, func(t *testing.T) {
			cl, st := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) {
				writeJSON(w, 200, []byte(`{}`))
			})
			req := textReq()
			cse.mut(&req)
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
		Model:     "claude-3-5-sonnet-20241022",
		System:    "be helpful",
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
		System   string           `json:"system"`
		Messages []wireOutMessage `json:"messages"`
	}
	if err := json.Unmarshal(st.body(0), &w); err != nil {
		t.Fatal(err)
	}
	// system = req.System + system 消息文本，合并进独立字段。
	if w.System != "be helpful\nsys" {
		t.Errorf("system = %q", w.System)
	}
	// system 消息不进 messages：user(reasoning drops→text) / assistant(tool_use) / user(tool_result)。
	if len(w.Messages) != 3 {
		t.Fatalf("messages = %d, want 3: %+v", len(w.Messages), w.Messages)
	}
	if w.Messages[0].Role != "user" || len(w.Messages[0].Content) != 1 || w.Messages[0].Content[0].Type != "text" || w.Messages[0].Content[0].Text != "question" {
		t.Errorf("messages[0] = %+v", w.Messages[0])
	}
	m1 := w.Messages[1]
	if m1.Role != "assistant" || len(m1.Content) != 1 || m1.Content[0].Type != "tool_use" || m1.Content[0].ID != "call_x" || m1.Content[0].Name != "search" || string(m1.Content[0].Input) != `{"q":1}` {
		t.Errorf("messages[1] = %+v", m1)
	}
	m2 := w.Messages[2]
	if m2.Role != "user" || len(m2.Content) != 1 || m2.Content[0].Type != "tool_result" || m2.Content[0].ToolUseID != "call_x" || m2.Content[0].Content != "result" {
		t.Errorf("messages[2] = %+v", m2)
	}
}

func TestConsumeJSONFailuresP5(t *testing.T) {
	c := New("http://127.0.0.1", testKey)
	t.Run("missing stop_reason", func(t *testing.T) {
		_, err := c.consumeJSON(strings.NewReader(`{"type":"message","model":"m","content":[{"type":"text","text":"x"}],"usage":{}}`), "rid", func(e types.StreamEvent) {})
		ae := asAPIError(t, err)
		if ae.Kind != KindProtocol || !strings.Contains(ae.Message, "stop_reason") {
			t.Errorf("kind=%s msg=%q", ae.Kind, ae.Message)
		}
	})
	t.Run("error in 200", func(t *testing.T) {
		_, err := c.consumeJSON(strings.NewReader(string(fixture(t, "json_error200.json"))), "rid", func(e types.StreamEvent) {})
		ae := asAPIError(t, err)
		if ae.Kind != KindProtocol || ae.Code != "overloaded_error" {
			t.Errorf("kind=%s code=%s", ae.Kind, ae.Code)
		}
	})
	t.Run("not JSON", func(t *testing.T) {
		_, err := c.consumeJSON(strings.NewReader(`<html>gateway</html>`), "rid", func(e types.StreamEvent) {})
		ae := asAPIError(t, err)
		if ae.Kind != KindProtocol || !strings.Contains(ae.Message, "not JSON") {
			t.Errorf("kind=%s msg=%q", ae.Kind, ae.Message)
		}
	})
}
