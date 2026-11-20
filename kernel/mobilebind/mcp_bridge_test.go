package mobilebind

// MCP 热插拔桥的真实测试：起 mock MCP HTTP server（initialize/tools/list），
// 验证 Add → 启用/停用（热插拔，不重启）→ 工具聚合 → 移除 → 持久化重载。全离线可跑。

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func mockMCP(tools []string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var msg struct {
			Method string `json:"method"`
			ID     *int64 `json:"id"`
		}
		_ = json.Unmarshal(body, &msg)
		if msg.ID == nil { // notification
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		switch msg.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-06-18", "serverInfo": map[string]any{"name": "mock"}}
		case "tools/list":
			ts := make([]map[string]any, 0, len(tools))
			for _, n := range tools {
				ts = append(ts, map[string]any{"name": n, "description": "", "inputSchema": map[string]any{"type": "object"}})
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

func TestMcpBridgeHotPlug(t *testing.T) {
	state := filepath.Join(t.TempDir(), "mcp.json")
	if r := McpInit(state); r != "OK" {
		t.Fatalf("McpInit: %s", r)
	}
	srv := mockMCP([]string{"a1", "a2"})
	defer srv.Close()

	if r := McpAddServer("mock", srv.URL, ""); r != "OK" {
		t.Fatalf("McpAddServer: %s", r)
	}
	if !strings.Contains(McpServers(), "\"connected\":true") {
		t.Fatalf("servers 应含已连接：%s", McpServers())
	}
	if !strings.Contains(McpServers(), "\"tool_count\":2") {
		t.Fatalf("tool_count 应为2：%s", McpServers())
	}
	tools := McpTools()
	if !strings.Contains(tools, "a1") || !strings.Contains(tools, "a2") {
		t.Fatalf("工具应含 a1/a2：%s", tools)
	}

	// 停用（热插拔）→ 工具应隐藏
	if r := McpSetEnabled("mock", false); r != "OK" {
		t.Fatalf("SetEnabled(false): %s", r)
	}
	if strings.Contains(McpTools(), "a1") {
		t.Fatalf("停用后不应含 a1：%s", McpTools())
	}
	if !strings.Contains(McpServers(), "\"enabled\":false") {
		t.Fatalf("servers 应 enabled=false：%s", McpServers())
	}
	// 重新启用 → 工具回来（未重启）
	if r := McpSetEnabled("mock", true); r != "OK" {
		t.Fatalf("SetEnabled(true): %s", r)
	}
	if !strings.Contains(McpTools(), "a1") {
		t.Fatalf("重新启用后应含 a1：%s", McpTools())
	}

	// 移除
	if r := McpRemoveServer("mock"); r != "OK" {
		t.Fatalf("Remove: %s", r)
	}
	if strings.Contains(McpServers(), "\"mock\"") {
		t.Fatalf("移除后不应含 mock：%s", McpServers())
	}
}

func TestMcpBridgePersistReload(t *testing.T) {
	state := filepath.Join(t.TempDir(), "mcp.json")
	McpInit(state)
	srv := mockMCP([]string{"x1"})
	defer srv.Close()
	if r := McpAddServer("p1", srv.URL, `{"Authorization":"Bearer t"}`); r != "OK" {
		t.Fatalf("add: %s", r)
	}
	// 模拟重启：清空内存 Manager 单例（真实重启会丢进程内连接），再 Init 只加载配置
	mcpMu.Lock(); mcpMgr = nil; mcpMu.Unlock()
	McpInit(state)
	s := McpServers()
	if !strings.Contains(s, "p1") || !strings.Contains(s, "\"connected\":false") {
		t.Fatalf("重载后应显示 p1 未连接：%s", s)
	}
	// Restore 后连接
	if r := McpRestore(); !strings.Contains(r, "\"restored\":1") {
		t.Fatalf("restore 应恢复 1 个：%s", r)
	}
	if !strings.Contains(McpServers(), "\"connected\":true") {
		t.Fatalf("restore 后应已连接：%s", McpServers())
	}
}
