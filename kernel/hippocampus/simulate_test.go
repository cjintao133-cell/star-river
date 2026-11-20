package hippocampus

import (
	"testing"
	"time"
)

// 30 天月测：低重要记忆反复衰减与复习触发；衰减曲线呈锯齿；复习确实发生。
func TestRunMonth30(t *testing.T) {
	fc := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	e := New(Options{Clock: fc, ReviewThreshold: 0.5})
	e.Encode("a", "低重要", 0, 0) // S=1，~0.69 天到期
	e.Encode("b", "高情绪", 1, 1) // S=5，~3.47 天到期

	rep := RunMonth(e, fc, 30)
	if rep.Days != 30 {
		t.Errorf("Days = %d", rep.Days)
	}
	if rep.TotalReviews == 0 {
		t.Fatal("30 天内应有复习触发")
	}
	if len(rep.DecayCurve) != 30 {
		t.Errorf("衰减曲线应 30 点, got %d", len(rep.DecayCurve))
	}
	// 衰减曲线含<R 跌破 0.5 再回升的锯齿，证明既有衰减又有复习回升。
	hadBelow := false
	hadRise := false
	for i := 1; i < len(rep.DecayCurve); i++ {
		if rep.DecayCurve[i].Retention < 0.6 {
			hadBelow = true
		}
		if rep.DecayCurve[i].Retention > rep.DecayCurve[i-1].Retention+0.01 {
			hadRise = true
		}
	}
	if !hadBelow {
		t.Error("曲线应出现留存跌破 0.6（衰减）")
	}
	if !hadRise {
		t.Error("曲线应出现留存回升（复习强化）")
	}
	// 每条记忆复习次数 >=1（30 天内都被复习过）。
	for _, m := range rep.Memories {
		if m.Reviews < 1 {
			t.Errorf("记忆 %s 30 天内复习次数 = %d, 应 >=1", m.ID, m.Reviews)
		}
	}
	// 高情绪记忆 S 增长后最终稳定度应 >= 初始 5。
	for _, m := range rep.Memories {
		if m.ID == "b" && m.FinalStability < 5.0 {
			t.Errorf("b 最终稳定度应 >=5, got %v", m.FinalStability)
		}
	}
}

// 单条高稳定记忆 30 天不复习 → 留存持续下降（无锯齿，验证衰减本身）。
func TestRunMonthNoReviewKeepsDecaying(t *testing.T) {
	fc := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	e := New(Options{Clock: fc, ReviewThreshold: 0.01}) // 阈值极低，30 天内不到期
	e.Encode("x", "稳定", 1, 1)                           // S=5，阈值0.01 → 到期 ~23 天
	rep := RunMonth(e, fc, 20)                          // 20 天 < 23，不触发
	if rep.TotalReviews != 0 {
		t.Errorf("20 天内不应复习, got %d", rep.TotalReviews)
	}
	// 曲线单调下降。
	for i := 1; i < len(rep.DecayCurve); i++ {
		if rep.DecayCurve[i].Retention > rep.DecayCurve[i-1].Retention+1e-9 {
			t.Errorf("无复习时留存应单调降, day%d %v > day%d %v", i+1, rep.DecayCurve[i].Retention, i, rep.DecayCurve[i-1].Retention)
		}
	}
}

func TestRunMonthDefaultDays(t *testing.T) {
	fc := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	e := New(Options{Clock: fc})
	e.Encode("a", "", 0, 0)
	if rep := RunMonth(e, fc, 0); rep.Days != 30 {
		t.Errorf("days<=0 应默认 30, got %d", rep.Days)
	}
}
