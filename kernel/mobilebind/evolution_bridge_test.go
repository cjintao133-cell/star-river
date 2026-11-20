package mobilebind

import (
	"encoding/json"
	"strings"
	"testing"
)

// EvolutionStats 必须返回合法 JSON，含边界硬约束且全 true（自我进化审计完整性）。
func TestEvolutionStatsJSON(t *testing.T) {
	raw := EvolutionStats()
	if strings.HasPrefix(raw, "ERR:") {
		t.Fatalf("EvolutionStats 返回错误: %s", raw)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("非法 JSON: %v / %s", err, raw)
	}
	for _, k := range []string{"memoryCount", "memCap", "mood", "eventCounts", "eventTotal", "recent", "boundaries"} {
		if _, ok := m[k]; !ok {
			t.Errorf("缺字段 %q", k)
		}
	}
	b, ok := m["boundaries"].(map[string]any)
	if !ok {
		t.Fatalf("boundaries 非对象")
	}
	for _, k := range []string{"noCodeEdit", "localOnly", "whitelistOnly", "redlineFloor", "auditable"} {
		v, ok := b[k].(bool)
		if !ok || !v {
			t.Errorf("边界 %q 应为 true，实际 %v", k, b[k])
		}
	}
	if m["memCap"].(float64) != float64(memCap) {
		t.Errorf("memCap=%v 期望 %d", m["memCap"], memCap)
	}
}
