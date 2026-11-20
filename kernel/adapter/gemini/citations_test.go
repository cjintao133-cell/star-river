package gemini

import (
	"context"
	"net/http"
	"testing"

	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

func TestCollectGrounding(t *testing.T) {
	gm := &wireGroundingMetadata{GroundingChunks: []struct {
		Web struct {
			URI   string `json:"uri"`
			Title string `json:"title"`
		} `json:"web"`
	}{
		{Web: struct {
			URI   string `json:"uri"`
			Title string `json:"title"`
		}{URI: "https://a.com", Title: "A"}},
		{Web: struct {
			URI   string `json:"uri"`
			Title string `json:"title"`
		}{URI: "https://a.com", Title: "A2"}},
		{Web: struct {
			URI   string `json:"uri"`
			Title string `json:"title"`
		}{URI: "https://b.com", Title: "B"}},
	}}
	got := collectGrounding(nil, gm)
	if len(got) != 2 || got[0].URL != "https://a.com" || got[1].Title != "B" {
		t.Fatalf("grounding 解析/去重不对: %+v", got)
	}
	if collectGrounding(nil, nil) != nil {
		t.Error("nil 应返回 nil")
	}
}

func TestCitationsNonStream(t *testing.T) {
	resp := `{"candidates":[{"content":{"parts":[{"text":"答"}]},"finishReason":"STOP"}],
		"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2},
		"groundingMetadata":{"groundingChunks":[{"web":{"uri":"https://x.com","title":"X"}}]}}`
	cl, _ := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) { writeJSON(w, 200, []byte(resp)) })
	res, err := cl.chatOnce(context.Background(), ChatRequest{Model: "m", MaxTokens: 10, Messages: textReq().Messages}, false, func(types.StreamEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sources) != 1 || res.Sources[0].URL != "https://x.com" {
		t.Fatalf("grounding 引用未解析: %+v", res.Sources)
	}
}
