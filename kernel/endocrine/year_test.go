package endocrine

import (
	"math"
	"testing"
	"time"

	"github.com/cjintao133-cell/star-river/kernel/core/clock"
)

// T-M7-01 年测（情绪）：365 天衰减 + 每日交互全推演，验证状态有界（维度数恒定、值域内、无 NaN）。

func TestYearEmotionBoundedState(t *testing.T) {
	clk := clock.NewFakeClock(time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC))
	reg := DefaultRegistry()
	e := New(reg, clk)
	dims := len(reg.Keys())

	totalInteracts := 0
	for d := 1; d <= 365; d++ {
		clk.Advance(24 * time.Hour)
		// 每天一次交互（拉高唤醒/亲密，增加疲劳），模拟长期使用。
		e.Interact(map[string]float64{
			KeyArousal:  0.4,
			KeyIntimacy: 0.1,
			KeyFatigue:  0.2,
			KeyValence:  0.15,
		})
		totalInteracts++
		// 后台衰减 tick。
		e.Tick()

		// 关键：维度数恒定，不随时间增长（无状态爆炸）。
		if got := len(e.State()); got != dims {
			t.Fatalf("第 %d 天维数应恒定 %d, got %d", d, dims, got)
		}
	}

	// 所有维度值在各自范围内、有限。
	st := e.State()
	for _, d := range reg.All() {
		v, ok := st[d.Key]
		if !ok {
			t.Errorf("缺维度 %s", d.Key)
			continue
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("维度 %s 出现 NaN/Inf: %v", d.Key, v)
		}
		if v < d.Min-1e-9 || v > d.Max+1e-9 {
			t.Errorf("维度 %s 越界: %v ∉ [%v,%v]", d.Key, v, d.Min, d.Max)
		}
	}
	t.Logf("年测(情绪)：%d 维 × 365 天，%d 次交互，状态恒定 %d 维（有界）", dims, totalInteracts, len(st))
}

// TestYearEmotionRecovers 长期无交互后各维度回落到基线附近（衰减有界）。
func TestYearEmotionRecovers(t *testing.T) {
	clk := clock.NewFakeClock(time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC))
	reg := DefaultRegistry()
	e := New(reg, clk)
	// 猛拉一次
	e.Interact(map[string]float64{KeyArousal: 1.0, KeyFatigue: 1.0, KeyIntimacy: 1.0})
	// 静置 365 天
	clk.Advance(365 * 24 * time.Hour)
	e.Tick()
	for _, d := range reg.All() {
		v, _ := e.Value(d.Key)
		if math.Abs(v-d.Baseline) > 0.05 {
			t.Errorf("维度 %s 未回落基线(%v)，got %v", d.Key, d.Baseline, v)
		}
	}
}
