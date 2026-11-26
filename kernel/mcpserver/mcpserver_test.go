package mcpserver

import (
	"encoding/json"
	"errors"
	"testing"
)

func newTestServer() *Server {
	ex := func(name string, args map[string]any) (string, bool, error) {
		switch name {
		case "echo":
			return "echo:" + toString(args["text"]), false, nil
		case "fail":
			return "", true, errors.New("执行失败")
		default:
			return "", true, errors.New("unknown tool")
		}
	}
	return New(Config{
		Name: "star-river", Version: "0.1.0",
		Tools: []ToolDef{{Name: "echo", Description: "回声", InputSchema: map[string]any{"type": "object"}}},
	}, ex)
}

func toString(v any) string { s, _ := v.(string); return s }

func parse(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("响应非 JSON: %v raw=%s", err, s)
	}
	return m
}

func TestInitialize(t *testing.T) {
	s := newTestServer()
	resp, has := s.Handle(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if !has {
		t.Fatal("initialize 应有响应")
	}
	m := parse(t, resp)
	res := m["result"].(map[string]any)
	if res["protocolVersion"] != DefaultProtocolVersion {
		t.Errorf("协议版本不对: %v", res["protocolVersion"])
	}
	if res["serverInfo"].(map[string]any)["name"] != "star-river" {
		t.Errorf("serverInfo 不对: %v", res["serverInfo"])
	}
}

func TestInitializedNotification(t *testing.T) {
	s := newTestServer()
	_, has := s.Handle(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if has {
		t.Fatal("通知不应有响应")
	}
}

func TestToolsList(t *testing.T) {
	s := newTestServer()
	m := parse(t, mustResp(t, s, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
	tools := m["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["name"] != "echo" {
		t.Fatalf("tools/list 不对: %+v", tools)
	}
}

func TestToolsCallOK(t *testing.T) {
	s := newTestServer()
	m := parse(t, mustResp(t, s, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"text":"hi"}}}`))
	res := m["result"].(map[string]any)
	if res["isError"] != false {
		t.Errorf("不应出错: %+v", res)
	}
	content := res["content"].([]any)[0].(map[string]any)
	if content["text"] != "echo:hi" {
		t.Errorf("结果不对: %+v", content)
	}
}

func TestToolsCallError(t *testing.T) {
	s := newTestServer()
	m := parse(t, mustResp(t, s, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"fail","arguments":{}}}`))
	res := m["result"].(map[string]any)
	if res["isError"] != true {
		t.Errorf("应 isError=true: %+v", res)
	}
}

func TestPingAndUnknown(t *testing.T) {
	s := newTestServer()
	if m := parse(t, mustResp(t, s, `{"jsonrpc":"2.0","id":5,"method":"ping"}`)); m["result"] == nil {
		t.Errorf("ping 应有 result")
	}
	m := parse(t, mustResp(t, s, `{"jsonrpc":"2.0","id":6,"method":"bogus"}`))
	if m["error"] == nil {
		t.Errorf("未知方法应返回 error: %+v", m)
	}
}

func TestParseError(t *testing.T) {
	s := newTestServer()
	resp, has := s.Handle(`{not json`)
	if !has {
		t.Fatal("解析错误也应响应")
	}
	m := parse(t, resp)
	if int(m["error"].(map[string]any)["code"].(float64)) != -32700 {
		t.Errorf("应 -32700: %+v", m)
	}
}

func TestMissingToolName(t *testing.T) {
	s := newTestServer()
	m := parse(t, mustResp(t, s, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{}}`))
	if m["error"] == nil {
		t.Errorf("缺工具名应 error: %+v", m)
	}
}

func mustResp(t *testing.T, s *Server, req string) string {
	t.Helper()
	resp, has := s.Handle(req)
	if !has {
		t.Fatalf("应有响应: %s", req)
	}
	return resp
}
