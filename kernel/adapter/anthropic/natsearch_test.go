package anthropic

import (
	"strings"
	"testing"

	"star-river/kernel/core/types"
)

func TestNativeSearchInject(t *testing.T) {
	mk := func(native bool) ChatRequest {
		return ChatRequest{Model: "m", MaxTokens: 10, NativeSearch: native, Messages: []types.Message{{
			ID: "u", SessionID: "s", Role: types.RoleUser, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: "q"}},
		}}}
	}
	b, err := buildBody(mk(true), false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "web_search_20250305") || !strings.Contains(string(b), `"web_search"`) {
		t.Errorf("应注入 web_search 工具: %s", b)
	}
	b2, _ := buildBody(mk(false), false)
	if strings.Contains(string(b2), "web_search") {
		t.Errorf("关闭时不应注入: %s", b2)
	}
}
