package openai

import (
	"strings"
	"testing"

	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

func nsReq(native bool) ChatRequest {
	return ChatRequest{Model: "m", MaxTokens: 10, NativeSearch: native, Messages: []types.Message{{
		ID: "u", SessionID: "s", Role: types.RoleUser, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: "今天的新闻"}},
	}}}
}

func TestNativeSearchInject(t *testing.T) {
	b, err := buildBody(nsReq(true), false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"web_search_options"`) {
		t.Errorf("开启原生搜索应注入 web_search_options: %s", b)
	}
	b2, _ := buildBody(nsReq(false), false)
	if strings.Contains(string(b2), "web_search_options") {
		t.Errorf("关闭时不应注入: %s", b2)
	}
}
