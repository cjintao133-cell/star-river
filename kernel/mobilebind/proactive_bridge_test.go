package mobilebind

// proactive 桥测试：默认关→可开、源开关、Tick 返回合法 JSON、开关状态持久化。

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestProactiveBridge(t *testing.T) {
	db := filepath.Join(t.TempDir(), "pro.db")
	InitKernel(db, base64.StdEncoding.EncodeToString(make([]byte, 32)))

	st := ProactiveStatus()
	if !strings.Contains(st, "\"enabled\":false") {
		t.Fatalf("出厂应默认关: %s", st)
	}
	if r := ProactiveSetEnabled(true); r != "OK" {
		t.Fatalf("enable: %s", r)
	}
	if !strings.Contains(ProactiveStatus(), "\"enabled\":true") {
		t.Fatalf("应为开: %s", ProactiveStatus())
	}
	if r := ProactiveSetSource("emotion", false); r != "OK" {
		t.Fatalf("source off: %s", r)
	}
	if r := ProactiveSetSource("bogus", true); !strings.HasPrefix(r, "ERR:") {
		t.Fatalf("未知源应报错: %s", r)
	}

	// Tick 返回合法 JSON 数组（真实数据为空也不崩）
	tick := ProactiveTick()
	var arr []map[string]any
	if err := json.Unmarshal([]byte(tick), &arr); err != nil {
		t.Fatalf("Tick 应返回 JSON 数组: %q", tick)
	}
	if recent := ProactiveRecent(); !strings.HasPrefix(strings.TrimSpace(recent), "[") {
		t.Fatalf("Recent 应是数组: %q", recent)
	}

	// 持久化：重开库后 enabled 应保持 true
	InitKernel(db, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if !strings.Contains(ProactiveStatus(), "\"enabled\":true") {
		t.Fatalf("重启后 enabled 应持久化为 true: %s", ProactiveStatus())
	}
}
