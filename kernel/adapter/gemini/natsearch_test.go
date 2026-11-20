package gemini

import (
	"strings"
	"testing"

	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

func TestNativeSearchInject(t *testing.T) {
	mk := func(native bool) ChatRequest {
		return ChatRequest{Model: "m", MaxTokens: 10, NativeSearch: native, Messages: []types.Message{{
			ID: "u", SessionID: "s", Role: types.RoleUser, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: "q"}},
		}}}
	}
	b, err := buildBody(mk(true))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "googleSearch") {
		t.Errorf("应注入 googleSearch 工具: %s", b)
	}
	b2, _ := buildBody(mk(false))
	if strings.Contains(string(b2), "googleSearch") {
		t.Errorf("关闭时不应注入: %s", b2)
	}
}
