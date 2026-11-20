package gemini

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

func TestChatP3Degrade(t *testing.T) {
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
	// 降级走非流式端点：:generateContent、无 alt=sse、Accept=json。
	if got := st.path(1); !strings.HasSuffix(got, ":generateContent") {
		t.Errorf("P3 retry path = %q", got)
	}
	if got := st.rawQuery(1); got != "" {
		t.Errorf("P3 retry query = %q, want empty", got)
	}
	if got := st.hdr(1).Get("Accept"); got != "application/json" {
		t.Errorf("P3 retry Accept = %q", got)
	}
	want := []types.StreamEvent{
		{Kind: types.StreamDelta, Text: "Done."},
		{Kind: types.StreamToolCall, CallIndex: 0, Call: &types.ToolCall{ID: "gemini-call-0", Name: "get_time", Arguments: `{}`}},
		{Kind: types.StreamDone, Model: "gemini-1.5-pro", RequestID: "req-test"},
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
	if ae.Kind != KindProtocol || ae.Code != "RESOURCE_EXHAUSTED" {
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
			writeJSON(w, 429, []byte(`{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"slow down"}}`))
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
			writeJSON(w, 500, []byte(`{"error":{"code":500,"status":"INTERNAL","message":"boom"}}`))
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
		writeJSON(w, 503, []byte(`{"error":{"code":503,"status":"UNAVAILABLE","message":"down"}}`))
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
	body := []byte(`{"error":{"code":401,"status":"UNAUTHENTICATED","message":"key ` + testKey + ` rejected"}}`)
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) { writeJSON(w, 401, body) })
	_, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) {})
	ae := asAPIError(t, err)
	if ae.Kind != KindAuth || ae.Status != 401 || ae.Code != "UNAUTHENTICATED" {
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
		writeJSON(w, 400, []byte(`{"error":{"code":400,"status":"INVALID_ARGUMENT","message":"bad field foo"}}`))
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
		{"tool_use without call", func(r *ChatRequest) {
			r.Messages[0].Blocks = []types.ContentBlock{{Type: types.BlockToolUse}}
		}},
		{"tool_use bad arguments", func(r *ChatRequest) {
			r.Messages[0].Blocks = []types.ContentBlock{{Type: types.BlockToolUse, Call: &types.ToolCall{ID: "c1", Name: "f", Arguments: "not json"}}}
		}},
		{"tool_result unknown call id", func(r *ChatRequest) {
			r.Messages[0].Blocks = []types.ContentBlock{{Type: types.BlockToolResult, CallID: "ghost", Text: "x"}}
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
		Model:     "gemini-1.5-pro",
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
		Contents          []wireOutContent `json:"contents"`
		SystemInstruction *wireContentIn   `json:"systemInstruction"`
	}
	if err := json.Unmarshal(st.body(0), &w); err != nil {
		t.Fatal(err)
	}
	if w.SystemInstruction == nil || len(w.SystemInstruction.Parts) != 1 || w.SystemInstruction.Parts[0].Text != "be helpful\nsys" {
		t.Errorf("systemInstruction = %+v", w.SystemInstruction)
	}
	if len(w.Contents) != 3 {
		t.Fatalf("contents = %d, want 3: %+v", len(w.Contents), w.Contents)
	}
	if w.Contents[0].Role != "user" || len(w.Contents[0].Parts) != 1 || w.Contents[0].Parts[0].Text != "question" {
		t.Errorf("contents[0] = %+v", w.Contents[0])
	}
	m1 := w.Contents[1]
	if m1.Role != "model" || len(m1.Parts) != 1 || m1.Parts[0].FunctionCall == nil || m1.Parts[0].FunctionCall.Name != "search" || string(m1.Parts[0].FunctionCall.Args) != `{"q":1}` {
		t.Errorf("contents[1] = %+v", m1)
	}
	m2 := w.Contents[2]
	if m2.Role != "user" || len(m2.Parts) != 1 || m2.Parts[0].FunctionResponse == nil || m2.Parts[0].FunctionResponse.Name != "search" {
		t.Errorf("contents[2] = %+v", m2)
	}
	if !strings.Contains(string(m2.Parts[0].FunctionResponse.Response), "result") {
		t.Errorf("functionResponse.response = %s", m2.Parts[0].FunctionResponse.Response)
	}
}

func TestConsumeJSONFailuresP5(t *testing.T) {
	c := New("http://127.0.0.1", testKey)
	cases := []struct {
		name, body, wantCode, wantMsg string
	}{
		{"no candidates", `{"usageMetadata":{"promptTokenCount":1}}`, "", "no candidates"},
		{"missing finishReason", `{"candidates":[{"content":{"parts":[{"text":"x"}]}}]}`, "", "finishReason"},
		{"error in 200", string(fixture(t, "json_error200.json")), "RESOURCE_EXHAUSTED", ""},
		{"blocked", `{"promptFeedback":{"blockReason":"SAFETY"}}`, "SAFETY", "blocked"},
		{"not JSON", `<html>gateway</html>`, "", "not JSON"},
	}
	for _, cse := range cases {
		t.Run(cse.name, func(t *testing.T) {
			_, err := c.consumeJSON(strings.NewReader(cse.body), "rid", "gemini-1.5-pro", func(e types.StreamEvent) {})
			ae := asAPIError(t, err)
			if ae.Kind != KindProtocol {
				t.Errorf("kind = %s", ae.Kind)
			}
			if cse.wantCode != "" && ae.Code != cse.wantCode {
				t.Errorf("code = %s, want %s", ae.Code, cse.wantCode)
			}
			if cse.wantMsg != "" && !strings.Contains(ae.Message, cse.wantMsg) {
				t.Errorf("msg = %q, want contains %q", ae.Message, cse.wantMsg)
			}
		})
	}
}
