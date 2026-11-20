package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeMCP 一个可配置的完整 MCP 服务器（initialize/tools/list/tools/call）。
type fakeMCP struct {
	name      string
	tools     []Tool
	callFn    func(name string, args map[string]any) *CallResult
	initCalls int
	failInit  bool
}

func (f *fakeMCP) handler(t *testing.T) http.HandlerFunc {
	return rpcHandler(t, func(method string, id int64, params json.RawMessage, w http.ResponseWriter, _ *http.Request) {
		switch method {
		case "initialize":
			f.initCalls++
			if f.failInit {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			writeResult(w, id, InitResult{ProtocolVersion: "2025-06-18", ServerInfo: Implementation{Name: f.name}})
		case "tools/list":
			writeResult(w, id, map[string]any{"tools": f.tools})
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(params, &p)
			if f.callFn != nil {
				writeResult(w, id, f.callFn(p.Name, p.Arguments))
				return
			}
			writeResult(w, id, CallResult{Content: []Content{{Type: "text", Text: f.name + ":" + p.Name}}})
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})
}

func startFake(t *testing.T, f *fakeMCP) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	return srv
}

func TestManagerHotPlug(t *testing.T) {
	ctx := context.Background()
	a := startFake(t, &fakeMCP{name: "A", tools: []Tool{{Name: "a1"}, {Name: "a2"}}})
	b := startFake(t, &fakeMCP{name: "B", tools: []Tool{{Name: "b1"}}})

	m := NewManager()
	if err := m.Add(ctx, ServerConfig{Name: "A", Endpoint: a.URL}); err != nil {
		t.Fatal(err)
	}
	if err := m.Add(ctx, ServerConfig{Name: "B", Endpoint: b.URL}); err != nil {
		t.Fatal(err)
	}
	// 初始聚合 3 个工具
	if got := m.Tools(); len(got) != 3 {
		t.Fatalf("初始应 3 工具, got %d", len(got))
	}
	// 停用 A → 只剩 b1
	if err := m.SetEnabled("A", false); err != nil {
		t.Fatal(err)
	}
	got := m.Tools()
	if len(got) != 1 || got[0].Server != "B" || got[0].Tool.Name != "b1" {
		t.Fatalf("停用 A 后应只剩 b1, got %+v", got)
	}
	// Servers 状态
	sts := m.Servers()
	if len(sts) != 2 || sts[0].Name != "A" || sts[0].Enabled || sts[0].ToolCount != 2 {
		t.Errorf("Servers 状态不对: %+v", sts)
	}
	// 重新启用 A
	_ = m.SetEnabled("A", true)
	if got := m.Tools(); len(got) != 3 {
		t.Errorf("启用 A 后应 3 工具, got %d", len(got))
	}
	// 移除 B
	if err := m.Remove(ctx, "B"); err != nil {
		t.Fatal(err)
	}
	if got := m.Tools(); len(got) != 2 {
		t.Errorf("移除 B 后应 2 工具(A), got %d", len(got))
	}
	if _, ok := m.Enabled("B"); ok {
		t.Error("B 应已不存在")
	}
}

func TestManagerCallRouting(t *testing.T) {
	ctx := context.Background()
	a := startFake(t, &fakeMCP{name: "A", tools: []Tool{{Name: "echo"}},
		callFn: func(name string, args map[string]any) *CallResult {
			return &CallResult{Content: []Content{{Type: "text", Text: fmt.Sprintf("A/%s/%v", name, args["q"])}}}
		}})
	m := NewManager()
	if err := m.Add(ctx, ServerConfig{Name: "A", Endpoint: a.URL}); err != nil {
		t.Fatal(err)
	}
	res, err := m.CallTool(ctx, "A", "echo", map[string]any{"q": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Content[0].Text != "A/echo/hi" {
		t.Errorf("路由调用结果不对: %+v", res)
	}
	// 停用后调用应报错
	_ = m.SetEnabled("A", false)
	if _, err := m.CallTool(ctx, "A", "echo", nil); err == nil {
		t.Error("停用服务器调用应报错")
	}
	// 未知服务器
	if _, err := m.CallTool(ctx, "nope", "x", nil); err == nil {
		t.Error("未知服务器应报错")
	}
}

func TestManagerAddDuplicateAndFail(t *testing.T) {
	ctx := context.Background()
	a := startFake(t, &fakeMCP{name: "A", tools: []Tool{{Name: "a1"}}})
	m := NewManager()
	if err := m.Add(ctx, ServerConfig{Name: "A", Endpoint: a.URL}); err != nil {
		t.Fatal(err)
	}
	if err := m.Add(ctx, ServerConfig{Name: "A", Endpoint: a.URL}); err == nil {
		t.Error("重复添加应报错")
	}
	// 初始化失败的服务器不应登记
	bad := startFake(t, &fakeMCP{name: "bad", failInit: true})
	if err := m.Add(ctx, ServerConfig{Name: "bad", Endpoint: bad.URL}); err == nil {
		t.Error("初始化失败应返回错误")
	}
	if _, ok := m.Enabled("bad"); ok {
		t.Error("初始化失败的服务器不应登记")
	}
	// 空配置
	if err := m.Add(ctx, ServerConfig{Name: "", Endpoint: a.URL}); err == nil {
		t.Error("空名应报错")
	}
	// 移除不存在
	if err := m.Remove(ctx, "ghost"); err == nil {
		t.Error("移除不存在应报错")
	}
}

func TestManagerRefreshTools(t *testing.T) {
	ctx := context.Background()
	f := &fakeMCP{name: "A", tools: []Tool{{Name: "a1"}}}
	a := startFake(t, f)
	m := NewManager()
	if err := m.Add(ctx, ServerConfig{Name: "A", Endpoint: a.URL}); err != nil {
		t.Fatal(err)
	}
	if got := m.Tools(); len(got) != 1 {
		t.Fatalf("初始应 1 工具, got %d", len(got))
	}
	// 服务器工具集变化后刷新
	f.tools = []Tool{{Name: "a1"}, {Name: "a2"}, {Name: "a3"}}
	if err := m.RefreshTools(ctx, "A"); err != nil {
		t.Fatal(err)
	}
	if got := m.Tools(); len(got) != 3 {
		t.Errorf("刷新后应 3 工具, got %d", len(got))
	}
	if err := m.RefreshTools(ctx, "ghost"); err == nil {
		t.Error("刷新不存在服务器应报错")
	}
}
