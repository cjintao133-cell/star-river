package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

func TestCollectWebResults(t *testing.T) {
	raw := json.RawMessage(`[
		{"type":"web_search_result","url":"https://a.com","title":"A"},
		{"type":"web_search_result","url":"https://b.com","title":"B"},
		{"type":"web_search_result","url":"https://a.com","title":"A-dup"}
	]`)
	got := collectWebResults(nil, raw)
	if len(got) != 2 || got[0].URL != "https://a.com" || got[1].Title != "B" {
		t.Fatalf("来源解析/去重不对: %+v", got)
	}
	if collectWebResults(nil, nil) != nil {
		t.Error("nil 应返回 nil")
	}
	if got := collectWebResults(nil, json.RawMessage(`{bad`)); len(got) != 0 {
		t.Error("坏 JSON 应忽略")
	}
}

func TestCitationsNonStream(t *testing.T) {
	resp := `{"type":"message","model":"m","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1},"content":[
		{"type":"web_search_tool_result","content":[{"type":"web_search_result","url":"https://x.com","title":"X"}]},
		{"type":"text","text":"答"}
	]}`
	cl, _ := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) { writeJSON(w, 200, []byte(resp)) })
	res, err := cl.chatOnce(context.Background(), ChatRequest{Model: "m", MaxTokens: 10, Messages: textReq().Messages}, false, func(types.StreamEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sources) != 1 || res.Sources[0].URL != "https://x.com" {
		t.Fatalf("引用未解析: %+v", res.Sources)
	}
}
