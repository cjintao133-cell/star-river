package mobilebind

// T-M4-05 受控自我进化 · 审计面板数据桥（真实数据，不造假）。
// 数据来源：hippocampus（记忆库条数/容量）、endocrine（情绪五维）、bus（演进事件门铃）。
// 边界（硬约束，与 SafetyScreen 文案一致）：不自主改代码、不越红线、仅本地、白名单字段、可审计。
// 红线2：此桥在入口层 mobilebind，可 import 插件；底座不 import。

import (
	"encoding/json"

	"star-river/kernel/core/types"
)

// evolutionEvent 一条演进事件（来自 bus 门铃）。
type evolutionEvent struct {
	Topic  string `json:"topic"`
	Source string `json:"source"`
	Ts     int64  `json:"ts"`
}

// evolutionStats 自我进化审计快照。
type evolutionStats struct {
	MemoryCount int                `json:"memoryCount"`
	MemCap      int                `json:"memCap"`
	Mood        map[string]float64 `json:"mood"`
	EventCounts map[string]int     `json:"eventCounts"`
	EventTotal  int                `json:"eventTotal"`
	Recent      []evolutionEvent   `json:"recent"`
	Boundaries  map[string]bool    `json:"boundaries"`
}

// EvolutionStats 返回自我进化审计快照 JSON。失败返回 "ERR:..."。
func EvolutionStats() string {
	asmMu.Lock()
	ensureAssembly()
	count := asmMem.Len()
	mood := asmMood.State()
	asmMu.Unlock()

	// 演进事件门铃（bus 环形缓冲，≤50 条）。
	var evs []types.Event
	_ = json.Unmarshal([]byte(BusRecent()), &evs)

	counts := map[string]int{}
	for _, e := range evs {
		counts[e.Topic]++
	}
	// 最近演进事件（新→旧，聚焦数据层演进 topic，最多 8 条）。
	recent := make([]evolutionEvent, 0, 8)
	for i := len(evs) - 1; i >= 0 && len(recent) < 8; i-- {
		e := evs[i]
		if e.Topic == "memory.turn" || e.Topic == "mood.changed" || e.Topic == "adapter.route.failover" {
			recent = append(recent, evolutionEvent{Topic: e.Topic, Source: e.Source, Ts: e.Timestamp})
		}
	}

	stats := evolutionStats{
		MemoryCount: count,
		MemCap:      memCap,
		Mood:        mood,
		EventCounts: counts,
		EventTotal:  len(evs),
		Recent:      recent,
		Boundaries: map[string]bool{
			"noCodeEdit":    true, // 不自主修改内核代码 / 不自提权
			"localOnly":     true, // 全本地，不外发
			"whitelistOnly": true, // 仅写白名单数据字段
			"redlineFloor":  true, // 任何开关不得降低安全红线
			"auditable":     true, // 演进经 bus 门铃留痕，可审计
		},
	}
	b, err := json.Marshal(stats)
	if err != nil {
		return "ERR:" + err.Error()
	}
	return string(b)
}
