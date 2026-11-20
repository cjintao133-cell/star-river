package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// —— 子进程 helper：本测试二进制以 MCP_STDIO_HELPER=1 重入时化身 stdio MCP 服务器 ——
func TestStdioHelperProcess(t *testing.T) {
	if os.Getenv("MCP_STDIO_HELPER") != "1" {
		return // 父进程正常跑测试时直接返回
	}
	out := os.Stdout
	writeJSON := func(v any) {
		b, _ := json.Marshal(v)
		_, _ = out.Write(b)
		_, _ = out.Write([]byte("\n"))
	}
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 64*1024), 4<<20)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var msg struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.ID == nil { // 通知（notifications/initialized 等）忽略
			continue
		}
		id := *msg.ID
		switch msg.Method {
		case "initialize":
			if os.Getenv("MCP_STDIO_NOTIF") == "1" {
				writeJSON(map[string]any{"jsonrpc": "2.0", "method": "notifications/message", "params": map[string]any{"level": "info"}})
			}
			writeJSON(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{
				"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
				"serverInfo": map[string]any{"name": "stdio-helper", "version": "0"},
			}})
		case "ping":
			writeJSON(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{}})
		case "tools/list":
			writeJSON(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{
				"tools": []any{
					map[string]any{"name": "echo"},
					map[string]any{"name": "crash"},
					map[string]any{"name": "slow"},
				},
			}})
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(msg.Params, &p)
			switch p.Name {
			case "crash":
				os.Exit(1)
			case "slow":
				time.Sleep(500 * time.Millisecond)
				writeJSON(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{
					"content": []any{map[string]any{"type": "text", "text": "slow-done"}},
				}})
			default:
				writeJSON(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{
					"content": []any{map[string]any{"type": "text", "text": fmt.Sprintf("echo:%v", p.Arguments["q"])}},
				}})
			}
		default:
			writeJSON(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": -32601, "message": "unknown method"}})
		}
	}
	os.Exit(0)
}

func helperConfig(extraEnv ...string) StdioConfig {
	env := append([]string{"MCP_STDIO_HELPER=1"}, extraEnv...)
	return StdioConfig{Command: os.Args[0], Args: []string{"-test.run=TestStdioHelperProcess"}, Env: env}
}

// —— 传输层 ——

func TestStdioTransportBasic(t *testing.T) {
	tr := NewStdioTransport(helperConfig())
	if err := tr.Start(); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	defer tr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	msg, err := tr.request(ctx, "initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t"}})
	if err != nil {
		t.Fatal(err)
	}
	var res InitResult
	if err := decodeResult(msg, &res); err != nil || res.ServerInfo.Name != "stdio-helper" {
		t.Fatalf("initialize 结果不对: %v %+v", err, res)
	}
	// ping
	msg, err = tr.request(ctx, "ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := resultErr(msg); err != nil {
		t.Fatal(err)
	}
	if !tr.Alive() {
		t.Error("应存活")
	}
}

func TestStdioTransportNotification(t *testing.T) {
	got := make(chan string, 1)
	tr := NewStdioTransport(helperConfig("MCP_STDIO_NOTIF=1"))
	tr.SetNotificationHandler(func(method string, _ json.RawMessage) { got <- method })
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := tr.request(ctx, "initialize", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-got:
		if m != "notifications/message" {
			t.Errorf("通知方法名不对: %s", m)
		}
	case <-time.After(2 * time.Second):
		t.Error("应收到服务器通知")
	}
}

func TestStdioCrashIsolation(t *testing.T) {
	tr := NewStdioTransport(helperConfig())
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := tr.request(ctx, "initialize", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	// 触发子进程 os.Exit(1)
	_, err := tr.request(ctx, "tools/call", map[string]any{"name": "crash"})
	if err == nil {
		t.Fatal("崩溃应返回错误")
	}
	mustKind(t, err, KindNetwork)
	// 宿主进程未受牵连（能继续执行到这里即证明隔离）
	if tr.Alive() {
		t.Error("崩溃后应标记死亡")
	}
	// 死亡后请求快速失败
	_, err = tr.request(ctx, "ping", nil)
	if err == nil {
		t.Error("死亡后请求应失败")
	}
}

func TestStdioTimeout(t *testing.T) {
	tr := NewStdioTransport(helperConfig())
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	ictx, icancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer icancel()
	if _, err := tr.request(ictx, "initialize", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	// slow 工具 + 50ms 超时 → KindTimeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := tr.request(ctx, "tools/call", map[string]any{"name": "slow"})
	mustKind(t, err, KindTimeout)
	// 超时不应杀死进程
	if !tr.Alive() {
		t.Error("超时不应杀死进程")
	}
}

// —— 客户端 ——

func TestStdioClientLifecycle(t *testing.T) {
	c := NewStdioClient(helperConfig(), StdioClientOptions{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	init, err := c.Initialize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if init.ServerInfo.Name != "stdio-helper" {
		t.Errorf("serverInfo 不对: %+v", init)
	}
	if err := c.Ping(ctx); err != nil {
		t.Fatalf("Ping 失败: %v", err)
	}
	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 3 {
		t.Fatalf("应 3 工具, got %d", len(tools))
	}
	res, err := c.CallTool(ctx, "echo", map[string]any{"q": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Content[0].Text != "echo:hi" {
		t.Errorf("调用结果不对: %+v", res)
	}
	c.Close()
}

func TestStdioClientBeforeInit(t *testing.T) {
	c := NewStdioClient(helperConfig(), StdioClientOptions{})
	ctx := context.Background()
	if _, err := c.ListTools(ctx); err == nil {
		t.Error("未初始化 ListTools 应报错")
	}
	if _, err := c.CallTool(ctx, "echo", nil); err == nil {
		t.Error("未初始化 CallTool 应报错")
	}
}

func TestPlatformCapability(t *testing.T) {
	cap := Capability()
	if !cap.RemoteMCP {
		t.Error("远程 MCP 应全平台可用")
	}
	// stdio 仅 Windows（本测试在 Linux 跑 → false）
	if cap.StdioMCP != (cap.OS == "windows") {
		t.Errorf("stdio 能力应仅 Windows: os=%s stdio=%v", cap.OS, cap.StdioMCP)
	}
}
