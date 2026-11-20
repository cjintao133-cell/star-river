package tooldisc

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"star-river/kernel/store"
	"star-river/kernel/tool"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "tools.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func newIndex(t *testing.T) *Index {
	t.Helper()
	ix, err := NewIndex(newStore(t))
	if err != nil {
		t.Fatalf("new index: %v", err)
	}
	return ix
}

func mkTool(name, desc string) tool.Tool {
	return tool.Tool{Name: name, Description: desc, InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func TestIndexReplaceAndCount(t *testing.T) {
	ctx := context.Background()
	ix := newIndex(t)
	if err := ix.ReplaceServer(ctx, "srvA", []IndexedTool{
		{Tool: mkTool("get_weather", "查天气"), Category: "weather"},
		{Tool: mkTool("get_time", "查时间"), Category: "util"},
	}); err != nil {
		t.Fatal(err)
	}
	if n, _ := ix.Count(ctx); n != 2 {
		t.Fatalf("应 2 条, got %d", n)
	}
	// 替换式：再替换为 1 条
	if err := ix.ReplaceServer(ctx, "srvA", []IndexedTool{{Tool: mkTool("only", "唯一")}}); err != nil {
		t.Fatal(err)
	}
	if n, _ := ix.Count(ctx); n != 1 {
		t.Errorf("替换后应 1 条, got %d", n)
	}
	// 移除
	if err := ix.RemoveServer(ctx, "srvA"); err != nil {
		t.Fatal(err)
	}
	if n, _ := ix.Count(ctx); n != 0 {
		t.Errorf("移除后应 0, got %d", n)
	}
}

func TestIndexSearchAndSchema(t *testing.T) {
	ctx := context.Background()
	ix := newIndex(t)
	if err := ix.ReplaceServer(ctx, "srvA", []IndexedTool{
		{Tool: tool.Tool{Name: "get_weather", Description: "查询天气", InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)}, Category: "weather"},
		{Tool: mkTool("send_mail", "发邮件"), Category: "comm"},
	}); err != nil {
		t.Fatal(err)
	}
	// 关键词命中
	metas, err := ix.Search(ctx, "weather", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || metas[0].Name != "get_weather" || metas[0].Server != "srvA" {
		t.Fatalf("检索不对: %+v", metas)
	}
	// 命中项不含完整 schema（结构上 ToolMeta 无 schema 字段）
	// 分类过滤
	metas, _ = ix.Search(ctx, "", SearchOptions{Category: "comm"})
	if len(metas) != 1 || metas[0].Name != "send_mail" {
		t.Errorf("分类过滤不对: %+v", metas)
	}
	// 懒取 schema
	sch, ok, err := ix.Schema(ctx, "srvA", "get_weather")
	if err != nil || !ok {
		t.Fatalf("取 schema 失败: %v %v", ok, err)
	}
	if !json.Valid(sch) || len(sch) == 0 {
		t.Errorf("schema 非法: %s", sch)
	}
	// 不存在
	if _, ok, _ := ix.Schema(ctx, "srvA", "nope"); ok {
		t.Error("不存在工具应 false")
	}
}

func TestIndexCategoriesAndServerCounts(t *testing.T) {
	ctx := context.Background()
	ix := newIndex(t)
	_ = ix.ReplaceServer(ctx, "a", []IndexedTool{{Tool: mkTool("x", ""), Category: "weather"}})
	_ = ix.ReplaceServer(ctx, "b", []IndexedTool{{Tool: mkTool("y", ""), Category: "comm"}, {Tool: mkTool("z", "")}})
	cats, _ := ix.Categories(ctx)
	if len(cats) != 2 || cats[0] != "comm" || cats[1] != "weather" {
		t.Errorf("分类不对: %v", cats)
	}
	sc, _ := ix.ServerCounts(ctx)
	if sc["a"] != 1 || sc["b"] != 2 {
		t.Errorf("server 计数不对: %v", sc)
	}
}

func TestIndexShortDescTruncation(t *testing.T) {
	ctx := context.Background()
	ix := newIndex(t)
	long := ""
	for i := 0; i < 200; i++ {
		long += "x"
	}
	_ = ix.ReplaceServer(ctx, "s", []IndexedTool{{Tool: mkTool("t", long)}})
	metas, _ := ix.Search(ctx, "t", SearchOptions{})
	if len(metas[0].ShortDesc) > 90 { // 80 runes + 省略号
		t.Errorf("短描述应被截断, len=%d", len(metas[0].ShortDesc))
	}
}

func TestIndexEmptyServer(t *testing.T) {
	ix := newIndex(t)
	if err := ix.ReplaceServer(context.Background(), "", nil); err == nil {
		t.Error("空 server 应报错")
	}
}

func TestNewIndexNilStore(t *testing.T) {
	if _, err := NewIndex(nil); err == nil {
		t.Error("nil store 应报错")
	}
}
