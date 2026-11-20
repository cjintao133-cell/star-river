package tool

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPIErrorStringVariants(t *testing.T) {
	cases := []*APIError{
		{Kind: KindHTTP, HTTPStatus: 500, Code: -1, Message: "x"},
		{Kind: KindHTTP, HTTPStatus: 500, Message: "x"},
		{Kind: KindRPC, Code: -32601, Message: "x"},
		{Kind: KindNetwork, Message: "x"},
	}
	for _, c := range cases {
		if c.Error() == "" {
			t.Errorf("Error() 不应为空: %+v", c)
		}
	}
}

func TestReadSSEMultiline(t *testing.T) {
	in := "data: {\"jsonrpc\":\"2.0\",\"id\":1,\ndata: \"result\":{\"x\":5}}\n\n"
	msg, err := readSSEResponse(strings.NewReader(in), 1)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		X int `json:"x"`
	}
	if err := decodeResult(msg, &out); err != nil || out.X != 5 {
		t.Fatalf("多行 SSE 解析不对: %v %+v", err, out)
	}
}

func TestReadSSENoMatch(t *testing.T) {
	in := "data: {\"jsonrpc\":\"2.0\",\"id\":999,\"result\":{}}\n\n"
	_, err := readSSEResponse(strings.NewReader(in), 1)
	mustKind(t, err, KindProtocol)
}

func TestReadSSEBadJSON(t *testing.T) {
	in := "data: notjson\n\n"
	_, err := readSSEResponse(strings.NewReader(in), 1)
	mustKind(t, err, KindProtocol)
}

func TestReadSSEMultilineFlushNoTrailingBlank(t *testing.T) {
	// 最后一事件无结尾空行，应在 EOF flush 命中
	in := "data: {\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{}}"
	msg, err := readSSEResponse(strings.NewReader(in), 2)
	if err != nil {
		t.Fatal(err)
	}
	if msg.ID == nil || *msg.ID != 2 {
		t.Fatalf("flush 命中不对: %+v", msg)
	}
}

func TestPing(t *testing.T) {
	srv := httptest.NewServer(rpcHandler(t, func(method string, id int64, _ json.RawMessage, w http.ResponseWriter, _ *http.Request) {
		switch method {
		case "initialize":
			writeResult(w, id, InitResult{ProtocolVersion: "2025-06-18"})
		case "ping":
			writeResult(w, id, map[string]any{})
		default:
			w.WriteHeader(400)
		}
	}))
	defer srv.Close()
	c := newInitClient(t, srv, Options{})
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping 应成功: %v", err)
	}
}

func TestPingRPCError(t *testing.T) {
	srv := httptest.NewServer(rpcHandler(t, func(method string, id int64, _ json.RawMessage, w http.ResponseWriter, _ *http.Request) {
		switch method {
		case "initialize":
			writeResult(w, id, InitResult{ProtocolVersion: "2025-06-18"})
		case "ping":
			writeRPCErr(w, id, -1, "nope")
		default:
			w.WriteHeader(400)
		}
	}))
	defer srv.Close()
	c := newInitClient(t, srv, Options{})
	mustKind(t, c.Ping(context.Background()), KindRPC)
}

func TestInitResultGetter(t *testing.T) {
	srv := httptest.NewServer(rpcHandler(t, func(method string, id int64, _ json.RawMessage, w http.ResponseWriter, _ *http.Request) {
		if method == "initialize" {
			writeResult(w, id, InitResult{ProtocolVersion: "2025-06-18", ServerInfo: Implementation{Name: "got"}})
			return
		}
		w.WriteHeader(202)
	}))
	defer srv.Close()
	c := newInitClient(t, srv, Options{})
	if c.InitResult().ServerInfo.Name != "got" {
		t.Errorf("InitResult 取值不对: %+v", c.InitResult())
	}
}

// 初始化后服务器拒绝 initialized 通知（400）→ Initialize 返回 KindHTTP。
// 用裸 handler（rpcHandler 会把通知一律回 202，测不到拒绝路径）。
func TestInitializeNotifyRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var msg rpcMessage
		_ = json.Unmarshal(body, &msg)
		if msg.ID == nil { // 通知 → 明确拒绝
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if msg.Method == "initialize" {
			writeResult(w, *msg.ID, InitResult{ProtocolVersion: "2025-06-18"})
			return
		}
		w.WriteHeader(400)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, Options{})
	_, err := c.Initialize(context.Background())
	mustKind(t, err, KindHTTP)
}

func TestEncodeMarshalError(t *testing.T) {
	if _, err := encodeRequest(1, "m", make(chan int)); err == nil {
		t.Error("不可序列化 params 应报错")
	}
	if _, err := encodeNotification("m", make(chan int)); err == nil {
		t.Error("通知不可序列化应报错")
	}
}

func TestResultErrNilAndDecodeNilOut(t *testing.T) {
	mustKind(t, resultErr(nil), KindProtocol)
	if err := decodeResult(&rpcMessage{Result: json.RawMessage(`{}`)}, nil); err != nil {
		t.Errorf("nil out 应无错: %v", err)
	}
}

func TestAsAPIErrorWrap(t *testing.T) {
	ae := asAPIError(context.Canceled)
	if ae.Kind != KindNetwork {
		t.Errorf("非 APIError 应归 network, got %s", ae.Kind)
	}
	orig := &APIError{Kind: KindRPC, Code: 1}
	if got := asAPIError(orig); got != orig {
		t.Error("已是 APIError 应原样返回")
	}
	var target *APIError
	if asAPIErrorErr(nil, &target) {
		t.Error("nil 应 false")
	}
}

func TestDeleteNon405Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusInternalServerError)
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
			w.Header().Set("Mcp-Session-Id", "S")
			writeResult(w, *msg.ID, InitResult{ProtocolVersion: "2025-06-18"})
			return
		}
		w.WriteHeader(400)
	}))
	defer srv.Close()
	c := newInitClient(t, srv, Options{})
	mustKind(t, c.Close(context.Background()), KindHTTP)
}

func TestManagerClose(t *testing.T) {
	ctx := context.Background()
	a := startFake(t, &fakeMCP{name: "A", tools: []Tool{{Name: "a1"}}})
	m := NewManager()
	if err := m.Add(ctx, ServerConfig{Name: "A", Endpoint: a.URL}); err != nil {
		t.Fatal(err)
	}
	m.Close(ctx) // 不应 panic
}

// 会话失效但重初始化也失败 → 原样返回错误（不无限重试）。
func TestSessionExpiredReinitFails(t *testing.T) {
	var initCalls int
	srv := httptest.NewServer(rpcHandler(t, func(method string, id int64, _ json.RawMessage, w http.ResponseWriter, _ *http.Request) {
		switch method {
		case "initialize":
			initCalls++
			if initCalls > 1 { // 第二次初始化失败
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			writeResult(w, id, InitResult{ProtocolVersion: "2025-06-18"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := newInitClient(t, srv, Options{})
	_, err := c.ListTools(context.Background())
	if err == nil {
		t.Fatal("重初始化失败应返回错误")
	}
}
