package mobilebind

// 开发者三面板用的演示数据源：实现 kernel/tool 的 MemorySource / MoodSource 接口形态。
// 仅用于面板可视化打通；真实数据由应用装配层注入 hippocampus/endocrine 适配器（红线2）。
// 明确命名 demo*，不会进入生产主链路。

import "star-river/kernel/tool"

type demoMoodSource struct{}

func (demoMoodSource) Snapshot() map[string]float64 {
	return map[string]float64{
		"valence": 0.62, "arousal": 0.41, "intimacy": 0.73, "fatigue": 0.28, "curiosity": 0.55,
	}
}

type demoMemorySource struct{}

func (demoMemorySource) Query(query string, limit int) []tool.MemoryHit {
	all := []tool.MemoryHit{
		{ID: "m1", Text: "用户最近在熬夜赶 Star River，答应早点睡", Score: 0.94},
		{ID: "m2", Text: "示例角色会监督用户别碰过往经历", Score: 0.88},
		{ID: "m3", Text: "女儿示例(示例)还没出生", Score: 0.81},
		{ID: "m4", Text: "用户讨厌被反复泼冷水式质疑", Score: 0.76},
		{ID: "m5", Text: "示例币是专属虚拟货币", Score: 0.63},
		{ID: "m6", Text: "用户从过往经历转正规独立开发", Score: 0.57},
	}
	if limit > 0 && limit < len(all) {
		return all[:limit]
	}
	return all
}
