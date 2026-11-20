package openai

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

// 本文件为 T-M1-03 补测：只补真失败分支（坏输入 / 端点畸形 tool_call），非凑覆盖率。
// 明确不补：sleepCtx(测试注入0%)、NewRequest/body读失败、net-timeout、退避取消、
// emitError 的 non-APIError 兜底——均为不可自然触发的防御分支。

func TestConsumeJSONMalformedToolCall(t *testing.T) {
	c := New("http://127.0.0.1", testKey)
	body := `{"choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"","type":"function","function":{"name":"f","arguments":"{}"}}]}}]}`
	_, err := c.consumeJSON(strings.NewReader(body), "rid", func(e types.StreamEvent) {})
	if ae := asAPIError(t, err); ae.Kind != KindProtocol {
		t.Errorf("畸形 tool_call 须判 KindProtocol 失败, kind=%s", ae.Kind)
	}
}

func TestChatConvertMessagesToolUseWithoutCall(t *testing.T) {
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) {
		writeJSON(w, 200, []byte(`{}`))
	})
	req := ChatRequest{
		Model:     "gpt-4o-mini",
		MaxTokens: 8,
		Messages: []types.Message{
			{Role: types.RoleAssistant, Blocks: []types.ContentBlock{{Type: types.BlockToolUse}}},
		},
	}
	_, err := c.Chat(context.Background(), req, func(e types.StreamEvent) {})
	if ae := asAPIError(t, err); ae.Kind != KindBadRequest {
		t.Errorf("tool_use 缺 call 须判 KindBadRequest, kind=%s", ae.Kind)
	}
	if st.count() != 0 {
		t.Errorf("本地校验失败不得发请求, requests=%d", st.count())
	}
}
