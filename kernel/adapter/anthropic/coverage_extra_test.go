package anthropic

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

// 本文件为 T-M1-04 补测：只补真行为/真失败分支（version 默认回退、joinSystem 组合、
// 坏输入消息、端点畸形 tool_use），非凑覆盖率。明确不补 sleepCtx 与各防御兜底分支。

func TestVersionFallback(t *testing.T) {
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) {
		writeJSON(w, 200, []byte(`{"data":[]}`))
	})
	c.Version = "" // 强制走默认回退
	if _, err := c.Models(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := st.hdr(0).Get("anthropic-version"); got != defaultVersion {
		t.Errorf("version 默认回退失败, header=%q want %q", got, defaultVersion)
	}
}

func TestJoinSystem(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"", "x", "x"},
		{"x", "", "x"},
		{"a", "b", "a\nb"},
		{"", "", ""},
		{"  a  ", "  b  ", "a\nb"}, // 两端 trim
	}
	for _, c := range cases {
		if got := joinSystem(c.a, c.b); got != c.want {
			t.Errorf("joinSystem(%q,%q) = %q, want %q", c.a, c.b, got, c.want)
		}
	}
}

// system 消息里出现非 text 块 → 坏输入，必须 KindBadRequest 且不发请求。
func TestConvertMessagesSystemBlockUnsupported(t *testing.T) {
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) {
		writeJSON(w, 200, []byte(`{}`))
	})
	req := ChatRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 8,
		Messages: []types.Message{
			{Role: types.RoleSystem, Blocks: []types.ContentBlock{{Type: types.BlockImage, MIMEType: "image/png", Data: []byte{1}}}},
		},
	}
	_, err := c.Chat(context.Background(), req, func(e types.StreamEvent) {})
	if ae := asAPIError(t, err); ae.Kind != KindBadRequest {
		t.Errorf("system 非法块须判 KindBadRequest, kind=%s", ae.Kind)
	}
	if st.count() != 0 {
		t.Errorf("本地校验失败不得发请求, requests=%d", st.count())
	}
}

// tool_result 缺 call_id → 坏输入。
func TestConvertMessagesToolResultNoCallID(t *testing.T) {
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) {
		writeJSON(w, 200, []byte(`{}`))
	})
	req := ChatRequest{
		Model:     "claude-3-5-sonnet-20241022",
		MaxTokens: 8,
		Messages: []types.Message{
			{Role: types.RoleTool, Blocks: []types.ContentBlock{{Type: types.BlockToolResult, Text: "x"}}},
		},
	}
	_, err := c.Chat(context.Background(), req, func(e types.StreamEvent) {})
	if ae := asAPIError(t, err); ae.Kind != KindBadRequest {
		t.Errorf("tool_result 缺 call_id 须判 KindBadRequest, kind=%s", ae.Kind)
	}
	if st.count() != 0 {
		t.Errorf("本地校验失败不得发请求, requests=%d", st.count())
	}
}

// 端点返回畸形 tool_use（id 为空）→ 结构校验失败，不许静默成功。
func TestConsumeJSONMalformedToolUse(t *testing.T) {
	c := New("http://127.0.0.1", testKey)
	body := `{"type":"message","model":"m","content":[{"type":"tool_use","id":"","name":"f","input":{}}],"stop_reason":"tool_use","usage":{}}`
	_, err := c.consumeJSON(strings.NewReader(body), "rid", func(e types.StreamEvent) {})
	if ae := asAPIError(t, err); ae.Kind != KindProtocol {
		t.Errorf("畸形 tool_use 须判 KindProtocol 失败, kind=%s", ae.Kind)
	}
}
