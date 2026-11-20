package mobilebind

// 主动能力真实触发：把情绪疲劳推高 → ProactiveTick 应产出"歇一会儿"nudge。

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"star-river/kernel/endocrine"
	"star-river/kernel/proactive"
)

func TestProactiveEmotionFires(t *testing.T) {
	// 关静默时段，使断言不依赖测试运行时点（生产默认静默 23:00–07:00 保留）。
	proBaseCfg = proactive.DefaultConfig()
	proBaseCfg.Quiet.Enabled = false
	InitKernel(filepath.Join(t.TempDir(), "f.db"), base64.StdEncoding.EncodeToString(make([]byte, 32)))
	ProactiveSetEnabled(true)

	// 将疲劳推高（直接与装配层情绪引擎交互）。
	asmMu.Lock()
	ensureAssembly()
	asmMood.Interact(map[string]float64{endocrine.KeyFatigue: 0.95})
	asmMu.Unlock()

	tick := ProactiveTick()
	if !strings.Contains(tick, "歇一会儿") && !strings.Contains(tick, "fatigue") {
		t.Fatalf("高疲劳应触发休息 nudge: %s", tick)
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(tick), &arr); err != nil || len(arr) == 0 {
		t.Fatalf("应有 nudge 数组: %q err=%v", tick, err)
	}
}
