package mobilebind

// T-M4-03 开发者模式三面板数据桥（情绪曲线 / 记忆强度 / 七层上下文预览）。
// 走 kernel/tool 的中性接口（MemorySource/MoodSource）形态；演示实现在 dev_demo.go。
// 真实数据由应用装配层用 hippocampus/endocrine 适配后注入（红线2：底座不 import 插件）。
// 当前返回 demo=true 的示意数据，UI 明确标注"演示数据，待插件接入"。

import (
	"encoding/json"

	"github.com/cjintao133-cell/star-river/kernel/tool"
)

type devPanels struct {
	Demo    bool               `json:"demo"`
	Mood    map[string]float64 `json:"mood"`
	Memory  []tool.MemoryHit   `json:"memory"`
	Context []contextLayer     `json:"context"`
}

type contextLayer struct {
	Name   string `json:"name"`
	Tokens int    `json:"tokens"`
	Note   string `json:"note,omitempty"`
}

// DevPanels 返回开发者三面板 JSON（演示数据）。失败返回 "ERR:..."。
func DevPanels() string {
	mood := realMoodMap()
	demo := len(mood) == 0
	mem := realMemoryHits("", 8)
	if demo { // 引擎不可用（无库）→ 演示兜底
		mood = demoMoodSource{}.Snapshot()
		mem = demoMemorySource{}.Query("用户", 8)
	}
	ctx := []contextLayer{
		{Name: "系统规则", Tokens: 180, Note: "全局约束/安全红线"},
		{Name: "人设", Tokens: 320, Note: "角色卡 system/personality"},
		{Name: "世界书", Tokens: 260, Note: "触发式条目"},
		{Name: "长期记忆", Tokens: 540, Note: "hippocampus 检索命中"},
		{Name: "情绪快照", Tokens: 90, Note: "endocrine 五维"},
		{Name: "近期对话", Tokens: 1200, Note: "滑窗历史"},
		{Name: "当前输入", Tokens: 60, Note: "本轮用户消息"},
	}
	out := devPanels{Demo: demo, Mood: mood, Memory: mem, Context: ctx}
	b, err := json.Marshal(out)
	if err != nil {
		return "ERR:" + err.Error()
	}
	return string(b)
}
