package tool

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestStderrBufferInternal(t *testing.T) {
	tr := NewStdioTransport(StdioConfig{})
	tr.appendStderr("hello")
	if tr.StderrTail() != "hello" {
		t.Errorf("StderrTail=%q", tr.StderrTail())
	}
	big := strings.Repeat("x", defaultStderrTail+500)
	tr.appendStderr(big)
	tail := tr.StderrTail()
	if len(tail) != defaultStderrTail {
		t.Errorf("应截断到 %d, got %d", defaultStderrTail, len(tail))
	}
}

func TestStdioDoubleStart(t *testing.T) {
	tr := NewStdioTransport(helperConfig())
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	if err := tr.Start(); err == nil {
		t.Error("重复 Start 应报错")
	}
}

func TestStdioCloseIdle(t *testing.T) {
	tr := NewStdioTransport(StdioConfig{})
	if err := tr.Close(); err != nil { // 未启动时 Close 应安全
		t.Errorf("未启动 Close 应无错: %v", err)
	}
}

func TestRequestBeforeStart(t *testing.T) {
	tr := NewStdioTransport(StdioConfig{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := tr.request(ctx, "ping", nil)
	mustKind(t, err, KindNetwork)
}

func TestTransportGetterAndPlatform(t *testing.T) {
	c := NewStdioClient(helperConfig(), StdioClientOptions{})
	if c.Transport() == nil {
		t.Error("Transport 不应为空")
	}
	if StdioMCPEnabled() != (Capability().OS == "windows") {
		t.Error("StdioMCPEnabled 应与平台一致")
	}
}

func TestStdioClientPingError(t *testing.T) {
	// 打到不存在的工具方法：Ping 用 ping（helper 支持），改为 verif y 未知方法经 call 走 RPC error。
	c := NewStdioClient(helperConfig(), StdioClientOptions{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	// 直接向传输层发一个 helper 不支持的方法 → RPC error
	tr := c.Transport()
	msg, err := tr.request(ctx, "no/such/method", nil)
	if err != nil {
		t.Fatal(err)
	}
	res := resultErr(msg)
	mustKind(t, res, KindRPC)
}
