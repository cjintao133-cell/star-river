package mobilebind

// 事件总线：发布后可查最近事件；灾备切换会向总线发 adapter.route.failover 门铃。

import (
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBusPublishRecent(t *testing.T) {
	InitKernel(filepath.Join(t.TempDir(), "b.db"), base64.StdEncoding.EncodeToString(make([]byte, 32)))
	// 记录当前基线
	base := BusRecent()
	if r := BusPublish("test.topic", "unit"); r != "OK" {
		t.Fatalf("publish: %s", r)
	}
	time.Sleep(20 * time.Millisecond) // 允许订阅协程收齐
	after := BusRecent()
	if after == base || !strings.Contains(after, "test.topic") {
		t.Fatalf("最近事件应含 test.topic: %s", after)
	}
	// 一轮对话应发 memory.turn
	observeTurn("你好", "在的")
	time.Sleep(20 * time.Millisecond)
	if !strings.Contains(BusRecent(), "memory.turn") {
		t.Fatalf("对话应发 memory.turn: %s", BusRecent())
	}
}
