package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"star-river/kernel/core/types"
)

func TestBuildBodyIncludesTools(t *testing.T) {
	req := ChatRequest{
		Model: "claude-x", MaxTokens: 256,
		Messages: []types.Message{{Role: types.RoleUser, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: "hi"}}}},
		Tools:    []ToolDef{{Name: "weather", Description: "查天气", Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)}},
	}
	b, err := buildBody(req, true)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"name":"weather"`) || !strings.Contains(s, `"input_schema"`) {
		t.Fatalf("body 应含工具定义(input_schema): %s", s)
	}
}
