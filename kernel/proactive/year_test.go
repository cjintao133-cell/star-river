package proactive

import (
	"testing"
	"time"

	"github.com/cjintao133-cell/star-river/kernel/core/clock"
)

// T-M7-01 年测（主动）：365 天三类触发源全推演，验证无状态爆炸——
// 关键看内部簿记有界：s.fired 历史被 prune 到时间窗内、s.lastFired 键数 = 去重候选数，不随天数线性膨胀。

func TestYearProactiveBoundedState(t *testing.T) {
	clk := clock.NewFakeClock(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	cfg := Config{
		Enabled:         true,
		DefaultCooldown: time.Hour,
		Rate:            RateLimit{MaxPerWindow: 100, Window: time.Hour, DailyMax: 1000},
		Quiet:           QuietHours{Enabled: false},
	}
	s := New(cfg, clk,
		MemoryReviewSource{},
		ScheduleSource{Entries: DefaultScheduleEntries()},
		EmotionSource{Rules: DefaultEmotionRules()},
	)

	// 固定 3 条到期记忆（Key 稳定）+ 每日疲劳高（触发 fatigue_rest）+ 正午命中午餐日程。
	reviews := []ReviewItem{
		{ID: "r0", NextReviewAt: 0},
		{ID: "r1", NextReviewAt: 0},
		{ID: "r2", NextReviewAt: 0},
	}
	emotion := EmotionState{"fatigue": 0.9}

	total := 0
	maxFired := 0
	for d := 1; d <= 365; d++ {
		clk.Advance(24 * time.Hour) // 每日正午评估
		out := s.Tick(reviews, emotion)
		total += len(out)
		if len(s.fired) > maxFired {
			maxFired = len(s.fired)
		}
	}
	// 一年确实发生了大量主动提醒（有活动）。
	if total < 365 {
		t.Errorf("一年总发出应 >365, got %d", total)
	}
	// 关键：内部 fired 历史保持有界（远小于总发出数，被 prune 到时间窗）。
	if maxFired > 50 {
		t.Errorf("fired 历史应保持有界(<50)，实际峰值 %d（疑似状态爆炸）", maxFired)
	}
	// lastFired 键数 = 去重候选数（有界，不随天数增长）。
	distinctKeys := len(s.lastFired)
	if distinctKeys > 10 {
		t.Errorf("lastFired 键数应受去重约束(<10)，实际 %d", distinctKeys)
	}
	t.Logf("年测(主动)：365 天总发出 %d 次；fired 历史峰值 %d（有界）；lastFired 去重键 %d（有界）",
		total, maxFired, distinctKeys)
}

// TestYearProactiveRateStillEnforced 长跑下静默/限频/去重仍生效（不因长期运行退化）。
func TestYearProactiveRateStillEnforced(t *testing.T) {
	clk := clock.NewFakeClock(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	cfg := Config{
		Enabled:         true,
		DefaultCooldown: time.Minute,
		Rate:            RateLimit{MaxPerWindow: 2, Window: time.Hour, DailyMax: 5},
		Quiet:           QuietHours{Enabled: false},
	}
	s := New(cfg, clk, MemoryReviewSource{})
	reviews := make([]ReviewItem, 0, 10)
	for i := 0; i < 10; i++ {
		reviews = append(reviews, ReviewItem{ID: "k" + string(rune('a'+i)), NextReviewAt: 0})
	}
	// 一年，但每天只发 ≤2（小时窗）/≤5（日上限）；去重 + 限频必须始终成立。
	for d := 1; d <= 365; d++ {
		clk.Advance(24 * time.Hour)
		out := s.Tick(reviews, nil)
		if len(out) > 2 {
			t.Fatalf("第 %d 天发出 %d 超小时窗上限 2", d, len(out))
		}
	}
}
