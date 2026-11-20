package tooldisc

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/cjintao133-cell/star-river/kernel/tool"
)

// callSession 模拟一个 MCP 连接，记录被调用情况（验证懒连接只连命中的 server）。
type callSession struct {
	server string
	calls  []string
	closed bool
}

func (s *callSession) Close() error { s.closed = true; return nil }
func (s *callSession) Call(name string) string {
	s.calls = append(s.calls, name)
	return "called:" + name
}

// buildScale 造 nServers × perServer 个工具，返回值含每 server 的工具名。
func buildScale(t *testing.T, ix *Index, nServers, perServer int) map[string][]string {
	t.Helper()
	ctx := context.Background()
	byServer := map[string][]string{}
	// 让每个 server 的工具都含 "weather" 关键词之一，便于检索命中。
	kinds := []string{"weather", "mail", "calendar", "search", "translate", "news", "music", "map", "shop", "chat"}
	for s := 0; s < nServers; s++ {
		server := fmt.Sprintf("srv%02d", s)
		kind := kinds[s%len(kinds)]
		items := make([]IndexedTool, perServer)
		names := make([]string, perServer)
		for i := 0; i < perServer; i++ {
			name := fmt.Sprintf("%s_tool_%03d", kind, i)
			names[i] = name
			items[i] = IndexedTool{
				Tool: tool.Tool{
					Name:        name,
					Description: kind + " 相关工具",
					InputSchema: json.RawMessage(fmt.Sprintf(`{"type":"object","properties":{"q_%d":{"type":"string"}}}`, i)),
				},
				Category: kind,
			}
		}
		if err := ix.ReplaceServer(ctx, server, items); err != nil {
			t.Fatal(err)
		}
		byServer[server] = names
	}
	return byServer
}

// TestScale2000EndToEnd 跨 10 个 mock server 注册 2000 工具，验证「检索→拉 schema→调用」全链路，
// 且懒连接只连被命中的 server（不连全部 10 个）。
func TestScale2000EndToEnd(t *testing.T) {
	ctx := context.Background()
	ix := newIndex(t)
	buildScale(t, ix, 10, 200)

	if n, _ := ix.Count(ctx); n != 2000 {
		t.Fatalf("应注册 2000 工具, got %d", n)
	}
	sc, _ := ix.ServerCounts(ctx)
	if len(sc) != 10 {
		t.Fatalf("应跨 10 server, got %d", len(sc))
	}

	// 懒连接器：记录建连的 server。
	opened := map[string]*callSession{}
	factory := func(server string) (Session, error) {
		s := &callSession{server: server}
		opened[server] = s
		return s, nil
	}
	conn := NewLazyConnector(factory, time.Minute, nil)

	// 1) 检索：找 weather 工具
	st := NewSearchTool(ix)
	res, err := st.Execute(ctx, []byte(`{"query":"weather","limit":5}`))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(res.Content[0].Text, "weather_tool_") {
		t.Fatalf("检索应命中 weather 工具: %s", res.Content[0].Text)
	}

	// 2) 拉 schema（按名，懒取；来自索引，不建连接）
	metas, _ := ix.Search(ctx, "weather", SearchOptions{Limit: 1})
	if len(metas) == 0 {
		t.Fatal("应至少 1 个 weather 工具")
	}
	target := metas[0]
	sch, ok, err := ix.Schema(ctx, target.Server, target.Name)
	if err != nil || !ok || !json.Valid(sch) {
		t.Fatalf("拉 schema 失败: %v %v %s", ok, err, sch)
	}
	// 检索/拉 schema 阶段不应建立任何连接
	if conn.LiveCount() != 0 || len(opened) != 0 {
		t.Fatalf("检索/拉 schema 不应建连, live=%d opened=%d", conn.LiveCount(), len(opened))
	}

	// 3) 调用：经懒连接器取该 server 会话（仅连这一个）
	sess, err := conn.Get(target.Server)
	if err != nil {
		t.Fatal(err)
	}
	cs := sess.(*callSession)
	out := cs.Call(target.Name)
	if out != "called:"+target.Name {
		t.Fatalf("调用结果不对: %s", out)
	}
	// 只连了命中的 server，其余 9 个未连
	if conn.LiveCount() != 1 || len(opened) != 1 {
		t.Errorf("应只连 1 个 server, live=%d opened=%d", conn.LiveCount(), len(opened))
	}
	if _, ok := opened[target.Server]; !ok {
		t.Errorf("应连接命中 server %s", target.Server)
	}
	t.Logf("2000 工具/10 server：全链路通过，仅懒连接 %s（未连其余 9 个）", target.Server)
}

// TestScaleSearchP99 2000 工具下检索 P99 延迟基准（入仓报告由调用方记录）。
func TestScaleSearchP99(t *testing.T) {
	ctx := context.Background()
	ix := newIndex(t)
	buildScale(t, ix, 10, 200)

	const iters = 300
	durs := make([]time.Duration, 0, iters)
	for i := 0; i < iters; i++ {
		q := []string{"weather", "mail", "calendar", "news", "chat"}[i%5]
		start := time.Now()
		if _, err := ix.Search(ctx, q, SearchOptions{Limit: 10}); err != nil {
			t.Fatal(err)
		}
		durs = append(durs, time.Since(start))
	}
	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	p50 := durs[len(durs)*50/100]
	p99 := durs[len(durs)*99/100]
	t.Logf("检索延迟（2000 工具）：p50=%v p99=%v（%d 次）", p50, p99, iters)
	// CI 上应远低于 100ms；留宽松上限防抖动。
	// 注：-race 下 modernc 纯 Go sqlite 有巨大开销（数十倍）；真实（非 race）P99 见报告。
	// 此处仅设宽松上限防异常，不作为性能断言。
	if p99 > 2*time.Second {
		t.Errorf("P99 异常高（可能回归）: %v", p99)
	}
}

// TestScaleHotPlug 热插拔：注册/移除 server 不需重启，索引即时反映。
func TestScaleHotPlug(t *testing.T) {
	ctx := context.Background()
	ix := newIndex(t)
	_ = ix.ReplaceServer(ctx, "a", []IndexedTool{{Tool: mkTool("a_tool", "a")}})
	if n, _ := ix.Count(ctx); n != 1 {
		t.Fatal("应 1")
	}
	// 新增 server b
	_ = ix.ReplaceServer(ctx, "b", []IndexedTool{{Tool: mkTool("b_tool", "b")}})
	if n, _ := ix.Count(ctx); n != 2 {
		t.Errorf("新增后应 2, got %d", n)
	}
	// 拔除 a
	_ = ix.RemoveServer(ctx, "a")
	metas, _ := ix.Search(ctx, "a_tool", SearchOptions{})
	if len(metas) != 0 {
		t.Errorf("移除 a 后不应再命中 a_tool: %+v", metas)
	}
	if n, _ := ix.Count(ctx); n != 1 {
		t.Errorf("拔除后应 1, got %d", n)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
