package mobilebind

// 设备策略（T-新增）：C 探针（CPU 特性/可用内存）→ 分档 → 调策略。
// 不只保活，还按内存/能力给出 UI 应采用的策略（动效开关、历史窗口、是否建议减动态）。
// 探针本身在 kernel/native/bridge（C/汇编），此处只消费其值，不重复实现。

import (
	"encoding/json"

	"star-river/kernel/native/bridge"
)

type devicePolicy struct {
	Animation           bool `json:"animation"`             // 是否开氛围动效
	SuggestReduceMotion bool `json:"suggest_reduce_motion"` // 低端机建议默认减动态
	MaxHistory          int  `json:"max_history"`           // 建议对话历史窗口
	NativeSearch        bool `json:"native_search"`         // 是否建议开模型原生搜索（省外部调用）
}

type deviceStrategy struct {
	Tier          string       `json:"tier"` // low | mid | high
	AvailMemBytes uint64       `json:"avail_mem_bytes"`
	Neon          bool         `json:"neon"`
	Avx2          bool         `json:"avx2"`
	Policy        devicePolicy `json:"policy"`
}

// DeviceStrategy 返回当前设备分档与推荐策略 JSON（供 UI 启动时/定期调用）。
func DeviceStrategy() string {
	neon, avx2 := bridge.CPUFeatures()
	mem := bridge.AvailMemBytes()

	tier := "high"
	pol := devicePolicy{Animation: true, SuggestReduceMotion: false, MaxHistory: 24, NativeSearch: true}
	switch {
	case mem > 0 && mem < 1_500_000_000: // < ~1.5GB
		tier = "low"
		pol = devicePolicy{Animation: false, SuggestReduceMotion: true, MaxHistory: 8, NativeSearch: true}
	case mem > 0 && mem < 3_000_000_000: // < ~3GB
		tier = "mid"
		pol = devicePolicy{Animation: true, SuggestReduceMotion: false, MaxHistory: 16, NativeSearch: true}
	}
	out := deviceStrategy{Tier: tier, AvailMemBytes: mem, Neon: neon, Avx2: avx2, Policy: pol}
	b, err := json.Marshal(out)
	if err != nil {
		return "ERR:" + err.Error()
	}
	return string(b)
}
