package mobilebind

// 工具发现（两段式）测试：工具少→全直发；工具多→只发 tool_search + 关键词预筛，footprint 有界。

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cjintao133-cell/star-river/kernel/adapter/openai"
	"github.com/cjintao133-cell/star-river/kernel/tool"
	"github.com/cjintao133-cell/star-river/kernel/tooldisc"
)

func mockMany(toolNames []string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusOK)
			return
		}
		var msg struct {
			Method string `json:"method"`
			ID     *int64 `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&msg)
		if msg.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		switch msg.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-06-18", "serverInfo": map[string]any{"name": "many"}}
		case "tools/list":
			ts := make([]map[string]any, 0, len(toolNames))
			for _, n := range toolNames {
				ts = append(ts, map[string]any{"name": n, "description": "工具 " + n + " 用来处理 " + n, "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}}})
			}
			result = map[string]any{"tools": ts}
		default:
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": result})
	}))
}

func qtsFromMgr() []tool.QualifiedTool {
	mcpMu.Lock()
	defer mcpMu.Unlock()
	return mcpEnsure().Tools()
}

func TestToolPlanSmallInlines(t *testing.T) {
	McpInit(filepath.Join(t.TempDir(), "m.json"))
	InitKernel(filepath.Join(t.TempDir(), "s.db"), base64.StdEncoding.EncodeToString(make([]byte, 32)))
	srv := mockMany([]string{"a1", "a2", "a3"})
	defer srv.Close()
	if r := McpAddServer("s_small", srv.URL, ""); r != "OK" {
		t.Fatalf("add: %s", r)
	}
	defs := toolPlan(context.Background(), qtsFromMgr(), "hi")
	if len(defs) != 3 {
		t.Fatalf("小量应直发3个，实际 %d", len(defs))
	}
}

func TestToolPlanLargeUsesDiscovery(t *testing.T) {
	McpInit(filepath.Join(t.TempDir(), "m.json"))
	InitKernel(filepath.Join(t.TempDir(), "s.db"), base64.StdEncoding.EncodeToString(make([]byte, 32)))
	names := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		names = append(names, fmt.Sprintf("tool%02d", i))
	}
	names[5] = "weather_query" // 一个与查询相关的
	srv := mockMany(names)
	defer srv.Close()
	if r := McpAddServer("s_large", srv.URL, ""); r != "OK" {
		t.Fatalf("add: %s", r)
	}
	defs := toolPlan(context.Background(), qtsFromMgr(), "帮我查 weather 天气")
	// 应有界：1(tool_search) + 最多 prefilterLimit
	if len(defs) > 1+prefilterLimit {
		t.Fatalf("footprint 应 <= %d，实际 %d", 1+prefilterLimit, len(defs))
	}
	hasSearch := false
	for _, d := range defs {
		if d.Name == tooldisc.SearchToolName {
			hasSearch = true
		}
	}
	if !hasSearch {
		t.Fatalf("大量工具时应含 tool_search 元工具")
	}
	// 不应把 30 个 schema 全塞进去
	if len(defs) >= 30 {
		t.Fatalf("未生效：仍下发了 %d 个（接近全量）", len(defs))
	}
	// 关键词预筛应优先带出 weather_query
	found := false
	for _, d := range defs {
		if strings.Contains(d.Name, "weather") {
			found = true
		}
	}
	if !found {
		t.Fatalf("关键词预筛应带出 weather_query，defs=%v", namesOf(defs))
	}
}

func namesOf(ds []openai.ToolDef) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.Name)
	}
	return out
}
