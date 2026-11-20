package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"star-river/kernel/core/types"
)

// 非流式：message.annotations 里带 url_citation，应解析进 ChatResult.Sources。
func TestCitationsJSON(t *testing.T) {
	resp := `{"model":"m","choices":[{"message":{"role":"assistant","content":"答","annotations":[{"type":"url_citation","url_citation":{"url":"https://a.com","title":"A"}},{"type":"url_citation","url_citation":{"url":"https://b.com","title":"B"}}]},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	cl, _ := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) { writeJSON(w, 200, []byte(resp)) })
	res, err := cl.chatOnce(context.Background(), ChatRequest{Model: "m", MaxTokens: 10, Messages: textReq().Messages}, false, func(types.StreamEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sources) != 2 || res.Sources[0].URL != "https://a.com" || res.Sources[1].Title != "B" {
		t.Fatalf("引用解析不对: %+v", res.Sources)
	}
}

func TestCitationsDedup(t *testing.T) {
	var anns []wireAnnotation
	_ = json.Unmarshal([]byte(`[{"type":"url_citation","url_citation":{"url":"https://a.com","title":"A"}},{"type":"url_citation","url_citation":{"url":"https://a.com","title":"A2"}},{"type":"other"}]`), &anns)
	got := addSources(nil, anns)
	if len(got) != 1 || got[0].URL != "https://a.com" {
		t.Errorf("重复 URL 应去重且忽略非 url_citation, got %+v", got)
	}
}
