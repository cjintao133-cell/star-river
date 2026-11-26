package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStructuredToolOutput(t *testing.T) {
	s := New(Config{
		Name: "sr", Version: "1",
		Tools: []ToolDef{{Name: "stat", Description: "统计"}},
		Structured: func(name string, args map[string]any) (string, any, bool, error) {
			if name != "stat" {
				return "", nil, true, nil
			}
			return "共 3 条", map[string]any{"count": 3, "items": []string{"a", "b", "c"}}, false, nil
		},
	}, nil)
	resp, _ := s.Handle(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"stat","arguments":{}}}`)
	var m map[string]any
	_ = json.Unmarshal([]byte(resp), &m)
	res := m["result"].(map[string]any)
	sc, ok := res["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("应含 structuredContent: %s", resp)
	}
	if sc["count"].(float64) != 3 {
		t.Fatalf("structured 内容不对: %+v", sc)
	}
	// content 里仍有文本
	content := res["content"].([]any)[0].(map[string]any)
	if content["text"] != "共 3 条" {
		t.Fatalf("文本应保留: %+v", content)
	}
}

func TestStructuredErrorPath(t *testing.T) {
	s := New(Config{
		Tools: []ToolDef{{Name: "x"}},
		Structured: func(name string, args map[string]any) (string, any, bool, error) {
			return "", nil, true, nil
		},
	}, nil)
	resp, _ := s.Handle(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"x","arguments":{}}}`)
	if !strings.Contains(resp, `"isError":true`) {
		t.Fatalf("应 isError true: %s", resp)
	}
	if strings.Contains(resp, "structuredContent") {
		t.Fatalf("nil structured 不应有字段: %s", resp)
	}
}
