package gemini

import (
	"context"
	"net/http"
	"testing"

	"star-river/kernel/core/types"
)

// 本文件为 T-M1-05 补测：只补真行为/真失败分支（非为凑覆盖率数字）。
// 覆盖 gemini.go toolResponse 的 isError/透传/空文本/非对象分支、usageFrom nil、
// models.go ChatModels 错误传播、sse.go scanSSE 的 data: 空行与 [DONE] 跳过。

func TestToolResponse(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		isError bool
		want    string
	}{
		{"valid object passthrough", `{"a":1}`, false, `{"a":1}`},
		{"plain text wrapped result", `hello`, false, `{"result":"hello"}`},
		{"plain text wrapped error", `boom`, true, `{"error":"boom"}`},
		{"empty text", ``, false, `{"result":""}`},
		{"json array is not object", `[1,2]`, false, `{"result":"[1,2]"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := string(toolResponse(c.text, c.isError))
			if got != c.want {
				t.Errorf("toolResponse(%q,%v) = %s, want %s", c.text, c.isError, got, c.want)
			}
		})
	}
}

func TestUsageFromNil(t *testing.T) {
	if got := usageFrom(nil); got != (Usage{}) {
		t.Errorf("usageFrom(nil) = %+v, want zero", got)
	}
	w := &wireUsageMetadata{PromptTokenCount: 3, CandidatesTokenCount: 4, TotalTokenCount: 7}
	if got := usageFrom(w); got != (Usage{PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7}) {
		t.Errorf("usageFrom = %+v", got)
	}
}

func TestChatModelsPropagatesError(t *testing.T) {
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) {
		writeJSON(w, 500, []byte(`{"error":{"code":500,"status":"INTERNAL","message":"boom"}}`))
	})
	_, err := c.ChatModels(context.Background())
	ae := asAPIError(t, err)
	if ae.Kind != KindServer {
		t.Errorf("ChatModels 应透传 Models 错误, kind=%s", ae.Kind)
	}
	if st.count() != 3 { // MaxRetries=2 → 3 次
		t.Errorf("requests = %d", st.count())
	}
}

// scanSSE 必须跳过：空 data 行、[DONE] 行、非 data 行（event:），只处理真 chunk。
func TestScanSSESkipsEmptyDoneAndNonData(t *testing.T) {
	body := ": comment ignored\n" +
		"event: message\n" +
		"data:\n" +
		"data: [DONE]\n" +
		"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"x\"}],\"role\":\"model\"},\"finishReason\":\"STOP\",\"index\":0}]}\n"
	c, _ := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Goog-Request-Id", "req-test")
		w.Write([]byte(body))
	})
	var evs []types.StreamEvent
	res, err := c.Chat(context.Background(), textReq(), func(e types.StreamEvent) { evs = append(evs, e) })
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	want := []types.StreamEvent{
		{Kind: types.StreamDelta, Text: "x"},
		{Kind: types.StreamDone, Model: "gemini-1.5-pro", RequestID: "req-test"},
	}
	if len(evs) != len(want) {
		t.Fatalf("events = %+v, want %+v", evs, want)
	}
	for i := range want {
		if evs[i].Kind != want[i].Kind || evs[i].Text != want[i].Text {
			t.Errorf("evs[%d] = %+v, want %+v", i, evs[i], want[i])
		}
	}
	if res.FinishReason != "STOP" {
		t.Errorf("finish = %q", res.FinishReason)
	}
}
