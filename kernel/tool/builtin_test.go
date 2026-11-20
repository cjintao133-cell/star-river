package tool

import (
	"context"
	"strings"
	"testing"

	"star-river/kernel/core/types"
)

// —— 测试替身（不 import 插件包，纯中性实现）——

type fakeMem struct {
	hits []MemoryHit
	last struct {
		q     string
		limit int
	}
}

func (f *fakeMem) Query(q string, limit int) []MemoryHit {
	f.last.q = q
	f.last.limit = limit
	if limit > 0 && limit < len(f.hits) {
		return f.hits[:limit]
	}
	return f.hits
}

type fakeMood struct{ st map[string]float64 }

func (f *fakeMood) Snapshot() map[string]float64 { return f.st }

type fakeVisual struct {
	jobID  string
	err    error
	gotPmt string
	gotPar map[string]any
}

func (f *fakeVisual) Request(_ context.Context, prompt string, params map[string]any) (string, error) {
	f.gotPmt = prompt
	f.gotPar = params
	return f.jobID, f.err
}

// —— 定义与注册 ——

func TestBuiltinDefinitions(t *testing.T) {
	r := NewRegistry()
	r.Register(NewMemoryQueryTool(&fakeMem{}))
	r.Register(NewMoodReportTool(&fakeMood{}))
	r.Register(NewVisualRequestTool(&fakeVisual{}))
	names := r.Names()
	if len(names) != 3 {
		t.Fatalf("应 3 工具, got %v", names)
	}
	defs := r.Definitions()
	if len(defs) != 3 {
		t.Fatalf("应 3 定义, got %d", len(defs))
	}
	for _, d := range defs {
		if d.Name == "" || len(d.InputSchema) == 0 {
			t.Errorf("定义缺字段: %+v", d)
		}
	}
	if !r.Has("memory_query") || r.Has("nope") {
		t.Error("Has 判断不对")
	}
}

// —— memory_query ——

func TestMemoryQueryExecute(t *testing.T) {
	src := &fakeMem{hits: []MemoryHit{
		{ID: "1", Text: "用户喜欢吃辣", Score: 0.9},
		{ID: "2", Text: "用户常熬夜", Score: 0.7},
	}}
	tool := NewMemoryQueryTool(src)
	res, err := tool.Execute(context.Background(), []byte(`{"query":"吃","limit":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if src.last.q != "吃" || src.last.limit != 1 {
		t.Errorf("参数传递不对: %+v", src.last)
	}
	text := res.Content[0].Text
	if !strings.Contains(text, "找到 1 条") || !strings.Contains(text, "吃辣") {
		t.Errorf("结果文本不对: %q", text)
	}
}

func TestMemoryQueryEmptyAndErrors(t *testing.T) {
	tool := NewMemoryQueryTool(&fakeMem{})
	// 空命中
	res, err := tool.Execute(context.Background(), []byte(`{"query":"x"}`))
	if err != nil || !strings.Contains(res.Content[0].Text, "没有找到") {
		t.Errorf("空命中应提示: %v %+v", err, res)
	}
	// 缺 query
	if _, err := tool.Execute(context.Background(), []byte(`{}`)); err == nil {
		t.Error("缺 query 应报错")
	}
	// 坏参数
	if _, err := tool.Execute(context.Background(), []byte(`{bad`)); err == nil {
		t.Error("坏参数应报错")
	}
	// 未配置源
	if _, err := NewMemoryQueryTool(nil).Execute(context.Background(), []byte(`{"query":"x"}`)); err == nil {
		t.Error("未配置应报错")
	}
	// limit 夹取到 20
	src := &fakeMem{}
	src.hits = make([]MemoryHit, 30)
	for i := range src.hits {
		src.hits[i] = MemoryHit{ID: "x", Text: "t"}
	}
	_, _ = NewMemoryQueryTool(src).Execute(context.Background(), []byte(`{"query":"q","limit":100}`))
	if src.last.limit != 20 {
		t.Errorf("limit 应夹到 20, got %d", src.last.limit)
	}
}

// —— mood_report ——

func TestMoodReportExecute(t *testing.T) {
	tool := NewMoodReportTool(&fakeMood{st: map[string]float64{"valence": 0.4, "fatigue": 0.8}})
	res, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	text := res.Content[0].Text
	// 排序输出：fatigue 在 valence 前
	if strings.Index(text, "fatigue") > strings.Index(text, "valence") {
		t.Errorf("应按 key 排序: %q", text)
	}
	if !strings.Contains(text, "0.80") {
		t.Errorf("应含数值: %q", text)
	}
	// 空状态
	res, _ = NewMoodReportTool(&fakeMood{st: map[string]float64{}}).Execute(context.Background(), nil)
	if !strings.Contains(res.Content[0].Text, "暂无") {
		t.Errorf("空状态应提示: %q", res.Content[0].Text)
	}
	// 未配置
	if _, err := NewMoodReportTool(nil).Execute(context.Background(), nil); err == nil {
		t.Error("未配置应报错")
	}
}

// —— visual_request ——

func TestVisualRequestExecute(t *testing.T) {
	vis := &fakeVisual{jobID: "job-42"}
	tool := NewVisualRequestTool(vis)
	res, err := tool.Execute(context.Background(), []byte(`{"prompt":"星空下的女孩","params":{"size":"512"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if vis.gotPmt != "星空下的女孩" || vis.gotPar["size"] != "512" {
		t.Errorf("参数传递不对: %q %v", vis.gotPmt, vis.gotPar)
	}
	if !strings.Contains(res.Content[0].Text, "job-42") {
		t.Errorf("应含任务 id: %q", res.Content[0].Text)
	}
	// 缺 prompt
	if _, err := tool.Execute(context.Background(), []byte(`{}`)); err == nil {
		t.Error("缺 prompt 应报错")
	}
	// 后端错误
	vis2 := &fakeVisual{err: &APIError{Kind: KindNetwork, Message: "boom"}}
	if _, err := NewVisualRequestTool(vis2).Execute(context.Background(), []byte(`{"prompt":"x"}`)); err == nil {
		t.Error("后端错误应上抛")
	}
	// 未配置
	if _, err := NewVisualRequestTool(nil).Execute(context.Background(), []byte(`{"prompt":"x"}`)); err == nil {
		t.Error("未配置应报错")
	}
}

// —— 主脑按需调用链路（集成）——

func TestMainBrainToolCallLoop(t *testing.T) {
	// 1) 装配：内置工具三件（三源都配置）
	r := NewRegistry()
	r.Register(NewMemoryQueryTool(&fakeMem{hits: []MemoryHit{{ID: "1", Text: "用户爱喝冰美式", Score: 0.88}}}))
	r.Register(NewMoodReportTool(&fakeMood{st: map[string]float64{"fatigue": 0.9, "valence": 0.2}}))
	r.Register(NewVisualRequestTool(&fakeVisual{jobID: "img-001"}))

	// 2) 模型请求侧会带 tools 定义（这里取 Definitions 证明可拼装）
	defs := r.Definitions()
	if len(defs) != 3 {
		t.Fatalf("应提供 3 个工具定义给模型, got %d", len(defs))
	}

	// 3) 模拟"主脑"返回的工具调用（types.ToolCall，即适配器解析出的 tool_use）
	calls := []types.ToolCall{
		{ID: "call_1", Name: "memory_query", Arguments: `{"query":"喝什么"}`},
		{ID: "call_2", Name: "mood_report", Arguments: `{}`},
		{ID: "call_3", Name: "visual_request", Arguments: `{"prompt":"深夜的猫"}`},
		{ID: "call_4", Name: "nonexistent", Arguments: `{}`}, // 未知工具 → 错误结果
	}

	// 4) 分发执行 → 得到可回填 model 的 tool_result 块
	blocks := r.DispatchAll(context.Background(), calls)
	if len(blocks) != 4 {
		t.Fatalf("应 4 个结果块, got %d", len(blocks))
	}
	for _, b := range blocks {
		if b.Type != types.BlockToolResult {
			t.Errorf("应为 tool_result 块: %+v", b)
		}
		if err := b.Validate(); err != nil {
			t.Errorf("tool_result 块非法: %v", err)
		}
	}
	// call_1 命中记忆
	if blocks[0].CallID != "call_1" || !strings.Contains(blocks[0].Text, "冰美式") || blocks[0].IsError {
		t.Errorf("memory_query 结果不对: %+v", blocks[0])
	}
	// call_2 情绪
	if !strings.Contains(blocks[1].Text, "fatigue") || blocks[1].IsError {
		t.Errorf("mood_report 结果不对: %+v", blocks[1])
	}
	// call_3 生成
	if !strings.Contains(blocks[2].Text, "img-001") || blocks[2].IsError {
		t.Errorf("visual_request 结果不对: %+v", blocks[2])
	}
	// call_4 未知工具 → IsError
	if !blocks[3].IsError || !strings.Contains(blocks[3].Text, "unknown tool") {
		t.Errorf("未知工具应 IsError: %+v", blocks[3])
	}

	// 5) 把结果组装成一条 tool 消息（回填模型的完整链路可用）
	toolMsg := types.Message{
		ID: "m2", SessionID: "s1", Role: types.RoleTool,
		Blocks: blocks,
	}
	if err := toolMsg.Validate(); err != nil {
		t.Fatalf("回填 tool 消息应合法: %v", err)
	}
}

func TestDispatchInvalidCall(t *testing.T) {
	r := NewRegistry()
	r.Register(NewMoodReportTool(&fakeMood{st: map[string]float64{"x": 1}}))
	// 缺 name 的非法调用
	b := r.Dispatch(context.Background(), types.ToolCall{ID: "c1", Arguments: `{}`})
	if !b.IsError || !strings.Contains(b.Text, "invalid tool call") {
		t.Errorf("非法调用应错误块: %+v", b)
	}
	// 参数非法 JSON
	b = r.Dispatch(context.Background(), types.ToolCall{ID: "c2", Name: "mood_report", Arguments: `{bad`})
	if !b.IsError {
		t.Errorf("坏参数应错误块: %+v", b)
	}
}

func TestRegistryOverride(t *testing.T) {
	r := NewRegistry()
	r.Register(NewMoodReportTool(&fakeMood{st: map[string]float64{"a": 1}}))
	r.Register(NewMoodReportTool(&fakeMood{st: map[string]float64{"b": 2}})) // 同名覆盖
	if len(r.Names()) != 1 {
		t.Errorf("同名不应重复占位, got %v", r.Names())
	}
	b := r.Dispatch(context.Background(), types.ToolCall{ID: "c", Name: "mood_report"})
	if !strings.Contains(b.Text, "b") {
		t.Errorf("应用新注册的工具: %q", b.Text)
	}
}
