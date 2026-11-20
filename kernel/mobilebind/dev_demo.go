package mobilebind

// 开发者三面板用的演示数据源：实现 kernel/tool 的 MemorySource / MoodSource 接口形态。
// 仅用于面板可视化打通；真实数据由应用装配层注入 hippocampus/endocrine 适配器（红线2）。
// 明确命名 demo*，不会进入生产主链路。演示文本一律中性占位，不含任何具体人设。

import "github.com/cjintao133-cell/star-river/kernel/tool"

type demoMoodSource struct{}

func (demoMoodSource) Snapshot() map[string]float64 {
	return map[string]float64{
		"valence": 0.62, "arousal": 0.41, "intimacy": 0.73, "fatigue": 0.28, "curiosity": 0.55,
	}
}

type demoMemorySource struct{}

func (demoMemorySource) Query(query string, limit int) []tool.MemoryHit {
	all := []tool.MemoryHit{
		{ID: "m1", Text: "示例记忆：最近在赶一个项目，答应早点休息", Score: 0.94},
		{ID: "m2", Text: "示例记忆：希望被温和地提醒别太拼", Score: 0.88},
		{ID: "m3", Text: "示例记忆：一件还没完成的待办事项", Score: 0.81},
		{ID: "m4", Text: "示例记忆：不喜欢被反复质疑", Score: 0.76},
		{ID: "m5", Text: "示例记忆：一个自定义的记账条目", Score: 0.63},
		{ID: "m6", Text: "示例记忆：正在做一个长期项目", Score: 0.57},
	}
	if limit > 0 && limit < len(all) {
		return all[:limit]
	}
	return all
}
