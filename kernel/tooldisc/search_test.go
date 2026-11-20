package tooldisc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"star-river/kernel/tool"
)

func TestSearchToolDefinition(t *testing.T) {
	ix := newIndex(t)
	st := NewSearchTool(ix)
	if st.Name() != SearchToolName || SearchToolName != "tool_search" {
		t.Errorf("名称不对: %s", st.Name())
	}
	def := st.Definition()
	if def.Name != "tool_search" || len(def.InputSchema) == 0 {
		t.Errorf("定义不全: %+v", def)
	}
	if !json.Valid(def.InputSchema) {
		t.Error("inputSchema 应合法 JSON")
	}
	reg := tool.NewRegistry()
	reg.Register(st)
	if !reg.Has("tool_search") {
		t.Error("应可注册进内置工具表")
	}
}

func TestSearchToolExecute(t *testing.T) {
	ctx := context.Background()
	ix := newIndex(t)
	_ = ix.ReplaceServer(ctx, "srv", []IndexedTool{
		{Tool: tool.Tool{Name: "get_weather", Title: "查天气", Description: "查询天气", InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)}, Category: "weather"},
		{Tool: mkTool("send_mail", "发邮件"), Category: "comm"},
	})
	st := NewSearchTool(ix)
	res, err := st.Execute(ctx, []byte(`{"query":"weather"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Content) != 1 || !strings.Contains(res.Content[0].Text, "get_weather") {
		t.Fatalf("命中不对: %+v", res.Content)
	}
	// 关键：返回目录**不含完整 schema**
	if strings.Contains(res.Content[0].Text, "properties") || strings.Contains(res.Content[0].Text, "city") {
		t.Errorf("目录不应泄露完整 schema: %s", res.Content[0].Text)
	}
	// 无命中
	res, _ = st.Execute(ctx, []byte(`{"query":"zzz-nope"}`))
	if !strings.Contains(res.Content[0].Text, "没有匹配") {
		t.Errorf("无命中应提示: %s", res.Content[0].Text)
	}
	// 坏 JSON 报错
	if _, err := st.Execute(ctx, []byte(`{bad`)); err == nil {
		t.Error("坏 JSON 应报错")
	}
}

func TestSearchToolNilIndex(t *testing.T) {
	st := NewSearchTool(nil)
	if _, err := st.Execute(context.Background(), []byte(`{"query":"x"}`)); err == nil {
		t.Error("nil index 应报错")
	}
}

func TestFootprintConstant(t *testing.T) {
	// footprint token 与工具总数无关：10 档 vs 2000 档应相等且 ≤ 固定配额。
	ctx := context.Background()
	ix10 := newIndex(t)
	tens := make([]IndexedTool, 10)
	for i := 0; i < 10; i++ {
		tens[i] = IndexedTool{Tool: mkTool(fmt.Sprintf("t%02d", i), "描述")}
	}
	_ = ix10.ReplaceServer(ctx, "s", tens)

	ixBig := newIndex(t)
	big := make([]IndexedTool, 2000)
	for i := 0; i < 2000; i++ {
		big[i] = IndexedTool{Tool: mkTool(fmt.Sprintf("t%04d", i), "描述")}
	}
	_ = ixBig.ReplaceServer(ctx, "s", big)

	f10 := FootprintTokens(ix10)
	fBig := FootprintTokens(ixBig)
	if f10 != fBig {
		t.Errorf("footprint 应与工具数无关: 10档=%d 2000档=%d", f10, fBig)
	}
	const quota = 800
	if f10 > quota {
		t.Errorf("footprint 应 ≤ 配额 %d, got %d", quota, f10)
	}
	t.Logf("工具层 footprint token = %d（配额 %d），与注册工具数（10 或 2000）无关", f10, quota)
}
