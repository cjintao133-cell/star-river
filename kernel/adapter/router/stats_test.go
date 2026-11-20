package router

import (
	"testing"
)

// 用固定失败率的 StatsProvider 测排序与标灰。
func fixedStats(rates map[string]float64) StatsProvider {
	return StatsProviderFunc(func(model string) (float64, bool) {
		r, ok := rates[model]
		return r, ok
	})
}

// 备用按失败率升序（健康优先）；主始终第一。
func TestAttemptsReordersByFailureRate(t *testing.T) {
	r, _ := New([]Endpoint{
		ep("primary", "pm", TierStandard),
		ep("bad", "m-bad", TierStandard),
		ep("good", "m-good", TierStandard),
	}, newFake(nil), WithStats(fixedStats(map[string]float64{
		"m-bad":  0.6,
		"m-good": 0.05,
	}), 0)) // grayRate=0 不标灰，仅排序
	plan := r.Plan()
	if len(plan) != 3 {
		t.Fatalf("plan len = %d", len(plan))
	}
	if plan[0].Name != "primary" {
		t.Errorf("主应第一: %+v", plan)
	}
	if plan[1].Name != "good" || plan[2].Name != "bad" {
		t.Errorf("备用应失败率升序 good->bad: %+v", plan)
	}
}

// 失败率超阈值标灰：该备用被剔除。
func TestAttemptsGrayOut(t *testing.T) {
	r, _ := New([]Endpoint{
		ep("primary", "pm", TierStandard),
		ep("gray", "m-gray", TierStandard),
		ep("ok", "m-ok", TierStandard),
	}, newFake(nil), WithStats(fixedStats(map[string]float64{
		"m-gray": 0.9,
		"m-ok":   0.1,
	}), 0.5)) // >0.5 标灰
	plan := r.Plan()
	for _, e := range plan {
		if e.Name == "gray" {
			t.Errorf("失败率 0.9>0.5 应被标灰剔除: %+v", plan)
		}
	}
	if len(plan) != 2 || plan[1].Name != "ok" {
		t.Errorf("plan = %+v", plan)
	}
}

// 无统计的备用保持原序排在有统计之后。
func TestAttemptsUnknownStatsAfterKnown(t *testing.T) {
	r, _ := New([]Endpoint{
		ep("primary", "pm", TierStandard),
		ep("unknown", "m-unknown", TierStandard),
		ep("known", "m-known", TierStandard),
	}, newFake(nil), WithStats(fixedStats(map[string]float64{
		"m-known": 0.3,
	}), 0.5))
	plan := r.Plan()
	if plan[1].Name != "known" || plan[2].Name != "unknown" {
		t.Errorf("有统计应排前: %+v", plan)
	}
}

// 未挂 stats 时行为不变（仅档位过滤）。
func TestAttemptsNoStats(t *testing.T) {
	r, _ := New([]Endpoint{
		ep("primary", "pm", TierStandard),
		ep("a", "ma", TierStandard),
		ep("b", "mb", TierStandard),
	}, newFake(nil))
	plan := r.Plan()
	if len(plan) != 3 || plan[1].Name != "a" || plan[2].Name != "b" {
		t.Errorf("无 stats 应保持原序: %+v", plan)
	}
}
