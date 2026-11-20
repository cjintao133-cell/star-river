package gemini

import (
	"encoding/json"
	"strings"
	"testing"

	"star-river/kernel/core/types"
)

func TestBuildBodyIncludesFunctionDeclarations(t *testing.T) {
	req := ChatRequest{
		Model: "gemini-x", MaxTokens: 256,
		Messages: []types.Message{{Role: types.RoleUser, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: "hi"}}}},
		Tools: []ToolDef{{Name: "weather", Description: "查天气", Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)}},
	}
	b, err := buildBody(req)
	if err != nil { t.Fatal(err) }
	s := string(b)
	if !strings.Contains(s, `"functionDeclarations"`) || !strings.Contains(s, `"weather"`) {
		t.Fatalf("body 应含 functionDeclarations: %s", s)
	}
}
