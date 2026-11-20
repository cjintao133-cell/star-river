package hippocampus

import (
	"math"
	"testing"
	"time"

	"github.com/cjintao133-cell/star-river/kernel/core/clock"
)

// T-M7-01 年测（记忆）：365 天衰减/复习全推演，验证无状态爆炸（存储增长有界）。
//
// 思路：固定一组记忆（不新增），跑 365 天每日触发到期复习；
// 断言 e.mem 规模恒定为初始集合（内部不因复习积累无界簿记），
// 稳定度/留存为有限值（无 NaN/Inf 溢出），复习计数单调但为定长字段。

func yearFakeClock() *clock.FakeClock {
	return clock.NewFakeClock(time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC))
}

func TestYearMemoryBoundedState(t *testing.T) {
	clk := yearFakeClock()
	e := New(Options{Clock: clk})

	// 预置 10 条记忆（不同重要度/情绪），模拟"全生命周期内固定集合"。
	const n = 10
	for i := 0; i < n; i++ {
		e.Encode(idf(i), "记忆内容", float64(i)/float64(n), float64(n-i)/float64(n))
	}
	if e.Len() != n {
		t.Fatalf("初始应 %d 条, got %d", n, e.Len())
	}

	// 365 天：每天推进 → 触发到期复习。
	totalReviews := 0
	for d := 1; d <= 365; d++ {
		clk.Advance(24 * time.Hour)
		now := clk.Now().UnixMilli()
		due := make([]string, 0)
		for id, m := range e.mem {
			if m.NextReviewAt <= now {
				due = append(due, id)
			}
		}
		for _, id := range due {
			if _, _, ok := e.Review(id); ok {
				totalReviews++
			}
		}
		// 关键：内存条目数恒定，不因时间/复习增长（无状态爆炸）。
		if e.Len() != n {
			t.Fatalf("第 %d 天内存条数应恒定 %d, got %d", d, n, e.Len())
		}
	}

	if totalReviews == 0 {
		t.Fatal("一年内应发生多次复习")
	}
	// 所有记忆最终数值有限、稳定度>0、留存∈[0,1]。
	now := clk.Now().UnixMilli()
	for id, m := range e.mem {
		if math.IsNaN(m.Stability) || math.IsInf(m.Stability, 0) || m.Stability <= 0 {
			t.Errorf("记忆 %s 稳定度异常: %v", id, m.Stability)
		}
		r := Retention(*m, now)
		if math.IsNaN(r) || r < 0 || r > 1 {
			t.Errorf("记忆 %s 留存异常: %v", id, r)
		}
	}
	t.Logf("年测(记忆)：%d 条 × 365 天，总复习 %d 次，最终条目数 %d（有界）", n, totalReviews, e.Len())
}

// TestYearRunMonthRegression 用 RunMonth 跑满一年，报告结构正常、留存有界。
func TestYearRunMonthRegression(t *testing.T) {
	clk := yearFakeClock()
	e := New(Options{Clock: clk})
	for i := 0; i < 5; i++ {
		e.Encode(idf(i), "x", 0.5, 0.5)
	}
	rep := RunMonth(e, clk, 365)
	if rep.Days != 365 {
		t.Errorf("应 365 天, got %d", rep.Days)
	}
	if len(rep.Memories) != 5 {
		t.Errorf("应 5 条, got %d", len(rep.Memories))
	}
	if rep.TotalReviews == 0 {
		t.Error("一年应有复习")
	}
	for _, m := range rep.Memories {
		if math.IsNaN(m.FinalRetention) || m.FinalRetention < 0 || m.FinalRetention > 1 {
			t.Errorf("最终留存越界: %v", m.FinalRetention)
		}
		if math.IsNaN(m.FinalStability) || m.FinalStability <= 0 {
			t.Errorf("最终稳定度异常: %v", m.FinalStability)
		}
	}
}

// idf 造 id。
func idf(i int) string {
	const digits = "0123456789"
	return "mem-" + string(digits[i%10])
}
