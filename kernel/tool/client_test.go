package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// ---- 测试基础设施 ----

// rpcHandler 解析一次 POST 的 JSON-RPC，notification 自动 202，其余交 handle。
func rpcHandler(t *testing.T, handle func(method string, id int64, params json.RawMessage, w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var msg rpcMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if msg.ID == nil { // notification
			w.WriteHeader(http.StatusAccepted)
			return
		}
		handle(msg.Method, *msg.ID, msg.Params, w, r)
	}
}

func writeResult(w http.ResponseWriter, id int64, result any) {
	w.Header().Set("Content-Type", "application/json")
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	_, _ = w.Write(b)
}

func writeRPCErr(w http.ResponseWriter, id int64, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	b, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id,
		"error": map[string]any{"code": code, "message": msg},
	})
	_, _ = w.Write(b)
}

func writeSSE(w http.ResponseWriter, payloads ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	for _, p := range payloads {
		fmt.Fprintf(w, "data: %s\n\n", p)
	}
}

func mustKind(t *testing.T, err error, want ErrKind) {
	t.Helper()
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("期望 *APIError, got %v", err)
	}
	if ae.Kind != want {
		t.Fatalf("期望 Kind=%s, got %s (%v)", want, ae.Kind, err)
	}
}

func newInitClient(t *testing.T, srv *httptest.Server, opts Options) *Client {
	t.Helper()
	c := NewClient(srv.URL, opts)
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize 失败: %v", err)
	}
	return c
}

// ---- JSON-RPC helpers ----

func TestJSONRPCEncode(t *testing.T) {
	b, err := encodeRequest(7, "tools/list", nil)
	if err != nil {
		t.Fatal(err)
	}
	var m rpcMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m.JSONRPC != "2.0" || m.ID == nil || *m.ID != 7 || m.Method != "tools/list" {
		t.Fatalf("编码不对: %s", b)
	}
	if len(m.Params) != 0 {
		t.Errorf("nil params 应省略, got %s", m.Params)
	}

	nb, err := encodeNotification("notifications/initialized", nil)
	if err != nil {
		t.Fatal(err)
	}
	var n rpcMessage
	_ = json.Unmarshal(nb, &n)
	if n.ID != nil {
		t.Errorf("通知不应带 id: %s", nb)
	}
}

func TestResultErrAndDecode(t *testing.T) {
	// error 信封
	em := &rpcMessage{Error: &rpcError{Code: -32602, Message: "bad"}}
	err := resultErr(em)
	var ae *APIError
	if !errors.As(err, &ae) || ae.Kind != KindRPC || ae.Code != -32602 {
		t.Fatalf("resultErr 分类不对: %v", err)
	}
	// 缺 result
	if err := decodeResult(&rpcMessage{}, &struct{}{}); err == nil {
		t.Error("缺 result 应报错")
	}
	// 正常 result
	var out struct {
		X int `json:"x"`
	}
	rm := &rpcMessage{Result: json.RawMessage(`{"x":5}`)}
	if err := decodeResult(rm, &out); err != nil || out.X != 5 {
		t.Fatalf("decodeResult 失败: %v %+v", err, out)
	}
}

// ---- 传输层 / 握手 ----

func TestInitializeJSONAndSession(t *testing.T) {
	var gotAccept, gotCT string
	srv := httptest.NewServer(rpcHandler(t, func(method string, id int64, _ json.RawMessage, w http.ResponseWriter, r *http.Request) {
		if method == "initialize" {
			gotAccept = r.Header.Get("Accept")
			gotCT = r.Header.Get("Content-Type")
			w.Header().Set("Mcp-Session-Id", "sess-123")
			writeResult(w, id, InitResult{ProtocolVersion: "2025-06-18", ServerInfo: Implementation{Name: "mock"}})
			return
		}
		if method == "notifications/initialized" {
			w.WriteHeader(202)
			return
		}
		w.WriteHeader(400)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, Options{})
	res, err := c.Initialize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.ServerInfo.Name != "mock" {
		t.Errorf("serverInfo 不对: %+v", res)
	}
	if c.SessionID() != "sess-123" {
		t.Errorf("会话 id 应为 sess-123, got %q", c.SessionID())
	}
	if c.ProtocolVersion() != "2025-06-18" {
		t.Errorf("协议版本不对: %q", c.ProtocolVersion())
	}
	if gotAccept != "application/json, text/event-stream" {
		t.Errorf("Accept 头不对: %q", gotAccept)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type 不对: %q", gotCT)
	}
}

func TestInitializeViaSSE(t *testing.T) {
	srv := httptest.NewServer(rpcHandler(t, func(method string, id int64, _ json.RawMessage, w http.ResponseWriter, _ *http.Request) {
		if method == "initialize" {
			// 先发一条服务器通知，再发应答（客户端应忽略前者）
			notif := `{"jsonrpc":"2.0","method":"notifications/message"}`
			resp := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":{"protocolVersion":"2025-06-18","serverInfo":{"name":"sse"}}}`, id)
			writeSSE(w, notif, resp)
			return
		}
		w.WriteHeader(202)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, Options{})
	res, err := c.Initialize(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.ServerInfo.Name != "sse" {
		t.Errorf("SSE 初始化结果不对: %+v", res)
	}
}

// ---- tools/list 分页 ----

func TestListToolsPagination(t *testing.T) {
	var listCalls int
	srv := httptest.NewServer(rpcHandler(t, func(method string, id int64, params json.RawMessage, w http.ResponseWriter, _ *http.Request) {
		switch method {
		case "initialize":
			writeResult(w, id, InitResult{ProtocolVersion: "2025-06-18"})
		case "tools/list":
			listCalls++
			var p struct {
				Cursor string `json:"cursor"`
			}
			_ = json.Unmarshal(params, &p)
			if p.Cursor == "" {
				writeResult(w, id, map[string]any{"tools": []Tool{{Name: "a"}}, "nextCursor": "c1"})
			} else {
				if p.Cursor != "c1" {
					t.Errorf("cursor 应为 c1, got %q", p.Cursor)
				}
				writeResult(w, id, map[string]any{"tools": []Tool{{Name: "b"}}})
			}
		default:
			w.WriteHeader(400)
		}
	}))
	defer srv.Close()

	c := newInitClient(t, srv, Options{})
	tools, err := c.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 2 || tools[0].Name != "a" || tools[1].Name != "b" {
		t.Fatalf("分页汇总不对: %+v", tools)
	}
	if listCalls != 2 {
		t.Errorf("应翻页 2 次, got %d", listCalls)
	}
}

// ---- tools/call ----

func TestCallToolSuccessAndIsError(t *testing.T) {
	srv := httptest.NewServer(rpcHandler(t, func(method string, id int64, params json.RawMessage, w http.ResponseWriter, _ *http.Request) {
		switch method {
		case "initialize":
			writeResult(w, id, InitResult{ProtocolVersion: "2025-06-18"})
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(params, &p)
			if p.Name == "boom" {
				writeResult(w, id, CallResult{Content: []Content{{Type: "text", Text: "failed"}}, IsError: true})
				return
			}
			writeResult(w, id, CallResult{Content: []Content{{Type: "text", Text: "ok:" + fmt.Sprint(p.Arguments["q"])}}})
		default:
			w.WriteHeader(400)
		}
	}))
	defer srv.Close()

	c := newInitClient(t, srv, Options{})
	res, err := c.CallTool(context.Background(), "echo", map[string]any{"q": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || len(res.Content) != 1 || res.Content[0].Text != "ok:hi" {
		t.Fatalf("调用结果不对: %+v", res)
	}
	// 工具执行错误：协议成功、IsError=true，不应是 Go error
	res, err = c.CallTool(context.Background(), "boom", nil)
	if err != nil {
		t.Fatalf("isError 不应返回 Go error: %v", err)
	}
	if !res.IsError || res.Content[0].Text != "failed" {
		t.Errorf("isError 结果不对: %+v", res)
	}
}

// ---- 错误路径 ----

func TestRPCErrorSurfaced(t *testing.T) {
	srv := httptest.NewServer(rpcHandler(t, func(method string, id int64, _ json.RawMessage, w http.ResponseWriter, _ *http.Request) {
		switch method {
		case "initialize":
			writeResult(w, id, InitResult{ProtocolVersion: "2025-06-18"})
		default:
			writeRPCErr(w, id, -32601, "method not found")
		}
	}))
	defer srv.Close()
	c := newInitClient(t, srv, Options{})
	_, err := c.CallTool(context.Background(), "x", nil)
	mustKind(t, err, KindRPC)
}

func TestHTTPErrorSurfaced(t *testing.T) {
	srv := httptest.NewServer(rpcHandler(t, func(method string, id int64, _ json.RawMessage, w http.ResponseWriter, _ *http.Request) {
		if method == "initialize" {
			writeResult(w, id, InitResult{ProtocolVersion: "2025-06-18"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()
	c := newInitClient(t, srv, Options{})
	_, err := c.CallTool(context.Background(), "x", nil)
	mustKind(t, err, KindHTTP)
}

func TestCallToolBeforeInit(t *testing.T) {
	c := NewClient("http://127.0.0.1:0", Options{})
	_, err := c.CallTool(context.Background(), "x", nil)
	mustKind(t, err, KindProtocol)
}

// ---- 会话失效 404 → 自动重开 ----

func TestSessionExpiredReinit(t *testing.T) {
	var initCalls, listCalls int
	srv := httptest.NewServer(rpcHandler(t, func(method string, id int64, _ json.RawMessage, w http.ResponseWriter, _ *http.Request) {
		switch method {
		case "initialize":
			initCalls++
			w.Header().Set("Mcp-Session-Id", fmt.Sprintf("s%d", initCalls))
			writeResult(w, id, InitResult{ProtocolVersion: "2025-06-18"})
		case "tools/list":
			listCalls++
			if listCalls == 1 {
				w.WriteHeader(http.StatusNotFound) // 首次会话失效
				return
			}
			writeResult(w, id, map[string]any{"tools": []Tool{{Name: "z"}}})
		default:
			w.WriteHeader(400)
		}
	}))
	defer srv.Close()
	c := newInitClient(t, srv, Options{})
	tools, err := c.ListTools(context.Background())
	if err != nil {
		t.Fatalf("应自动重开会话后成功: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "z" {
		t.Fatalf("重开后工具不对: %+v", tools)
	}
	if initCalls != 2 {
		t.Errorf("应重初始化 1 次(共 2), got %d", initCalls)
	}
}

// ---- 超时 + 取消通知 ----

func TestCallTimeoutAndCancelNotify(t *testing.T) {
	var mu sync.Mutex
	var cancels int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(200)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var msg rpcMessage
		_ = json.Unmarshal(body, &msg)
		if msg.ID == nil { // notification（含 cancelled）
			mu.Lock()
			if msg.Method == "notifications/cancelled" {
				cancels++
			}
			mu.Unlock()
			w.WriteHeader(202)
			return
		}
		if msg.Method == "initialize" {
			writeResult(w, *msg.ID, InitResult{ProtocolVersion: "2025-06-18"})
			return
		}
		// tools/call 慢响应 → 触发客户端超时
		time.Sleep(300 * time.Millisecond)
		writeResult(w, *msg.ID, CallResult{})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, Options{CallTimeout: 60 * time.Millisecond})
	if _, err := c.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := c.CallTool(context.Background(), "slow", nil)
	mustKind(t, err, KindTimeout)

	// 等待取消通知到达
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := cancels
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	n := cancels
	mu.Unlock()
	if n == 0 {
		t.Error("超时应尽力发送 notifications/cancelled")
	}
}

// ---- 传输请求头（会话/版本头）----

func TestSessionAndVersionHeaders(t *testing.T) {
	var mu sync.Mutex
	var seen []http.Header
	srv := httptest.NewServer(rpcHandler(t, func(method string, id int64, _ json.RawMessage, w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Clone())
		mu.Unlock()
		if method == "initialize" {
			w.Header().Set("Mcp-Session-Id", "SID")
			writeResult(w, id, InitResult{ProtocolVersion: "2025-06-18"})
			return
		}
		writeResult(w, id, map[string]any{"tools": []Tool{}})
	}))
	defer srv.Close()
	c := newInitClient(t, srv, Options{})
	if _, err := c.ListTools(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	// 最后一条是 tools/list，应带会话 id 与协议版本
	last := seen[len(seen)-1]
	if last.Get("Mcp-Session-Id") != "SID" {
		t.Errorf("后续请求应带 Mcp-Session-Id, got %q", last.Get("Mcp-Session-Id"))
	}
	if last.Get("MCP-Protocol-Version") != "2025-06-18" {
		t.Errorf("后续请求应带 MCP-Protocol-Version, got %q", last.Get("MCP-Protocol-Version"))
	}
}

// ---- tools/list 空/关闭 ----

func TestCloseSendsDelete(t *testing.T) {
	var deleted bool
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			mu.Lock()
			deleted = true
			mu.Unlock()
			w.WriteHeader(200)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var msg rpcMessage
		_ = json.Unmarshal(body, &msg)
		if msg.ID == nil {
			w.WriteHeader(202)
			return
		}
		if msg.Method == "initialize" {
			w.Header().Set("Mcp-Session-Id", "X")
			writeResult(w, *msg.ID, InitResult{ProtocolVersion: "2025-06-18"})
			return
		}
		w.WriteHeader(400)
	}))
	defer srv.Close()
	c := newInitClient(t, srv, Options{})
	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !deleted {
		t.Error("Close 应发送 DELETE 会话")
	}
}
