package mobilebind

// 工具调用循环端到端测试：mock LLM（第一轮返回 tool_call，第二轮返回正文）+ mock MCP（提供 echo 工具）。
// 验证：模型发起 tool_call → 经 Manager 执行 → 结果回填 → 再问 → 收尾。全离线。

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func mcpWithCall(tools []string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusOK)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var msg struct {
			Method string          `json:"method"`
			ID     *int64          `json:"id"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.Unmarshal(body, &msg)
		if msg.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		switch msg.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-06-18", "serverInfo": map[string]any{"name": "m"}}
		case "tools/list":
			ts := make([]map[string]any, 0, len(tools))
			for _, n := range tools {
				ts = append(ts, map[string]any{"name": n, "description": "", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "string"}}}})
			}
			result = map[string]any{"tools": ts}
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(msg.Params, &p)
			result = map[string]any{"content": []map[string]any{{"type": "text", "text": "executed:" + p.Name + ":" + toString(p.Arguments["x"])}}, "isError": false}
		default:
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": result})
	}))
}

func toString(v any) string { s, _ := v.(string); return s }

func sseData(w http.ResponseWriter, payloads ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, p := range payloads {
		_, _ = w.Write([]byte("data: " + p + "\n\n"))
	}
}

func TestChatStreamToolsLoop(t *testing.T) {
	state := filepath.Join(t.TempDir(), "mcp.json")
	McpInit(state)
	mcpSrv := mcpWithCall([]string{"echo"})
	defer mcpSrv.Close()
	if r := McpAddServer("srv", mcpSrv.URL, ""); r != "OK" {
		t.Fatalf("McpAddServer: %s", r)
	}

	var calls int32
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		switch n {
		case 1:
			// 第一轮：返回 tool_call（echo, {"x":"hi"}）
			sseData(w,
				`{"id":"x","model":"mock-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"echo","arguments":"{\"x\":\"hi\"}"}}]},"finish_reason":null}]}`,
				`{"id":"x","model":"mock-model","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
				`[DONE]`,
			)
		default:
			// 第二轮：返回正文（回填了工具结果）
			sseData(w,
				`{"id":"x","model":"mock-model","choices":[{"index":0,"delta":{"content":"done-ok"},"finish_reason":null}]}`,
				`{"id":"x","model":"mock-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
				`[DONE]`,
			)
		}
	}))
	defer llm.Close()

	s := &recSink{}
	ChatStreamTools(llm.URL+"/v1", "k", "mock-model", "人设", "hi", s)
	if s.err != "" {
		t.Fatalf("unexpected err: %s", s.err)
	}
	if !strings.Contains(strings.Join(s.deltas, ""), "done-ok") {
		t.Fatalf("应收到最终正文，deltas=%v", s.deltas)
	}
	if s.done != "mock-model" {
		t.Fatalf("done=%q", s.done)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("应发生 2 轮请求（工具调用+收尾），实际 %d", calls)
	}
}

func TestChatStreamToolsNoToolsFallsBack(t *testing.T) {
	// 无插件时 tools 为空，应等价普通对话（一轮）。
	McpInit(filepath.Join(t.TempDir(), "mcp.json"))
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseData(w,
			`{"id":"x","model":"mock-model","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}`,
			`{"id":"x","model":"mock-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			`[DONE]`,
		)
	}))
	defer llm.Close()
	s := &recSink{}
	ChatStreamTools(llm.URL+"/v1", "k", "mock-model", "", "hi", s)
	if s.err != "" || s.done != "mock-model" {
		t.Fatalf("应正常结束: %+v", s)
	}
}
