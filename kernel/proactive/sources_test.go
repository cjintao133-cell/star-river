package proactive

import (
	"testing"
	"time"
)

func TestNormMinutes(t *testing.T) {
	cases := map[int]int{-1: 1439, 0: 0, 1439: 1439, 1440: 0, 1500: 60, -1440: 0}
	for in, want := range cases {
		if got := normMinutes(in); got != want {
			t.Errorf("normMinutes(%d)=%d want %d", in, got, want)
		}
	}
}

func TestInWindow(t *testing.T) {
	if !inWindow(12*60+10, 12*60, 30) {
		t.Error("窗口内应 true")
	}
	if inWindow(12*60+30, 12*60, 30) {
		t.Error("窗口右开边界应 false")
	}
	// 跨零点：23:50 起 30 分钟，00:10 应命中
	if !inWindow(10, 23*60+50, 30) {
		t.Error("跨零点应命中")
	}
	if inWindow(0, 600, 0) {
		t.Error("零窗口应 false")
	}
}

func TestWeekdayAllowed(t *testing.T) {
	if !weekdayAllowed(nil, time.Monday) {
		t.Error("空集合=每天")
	}
	if !weekdayAllowed([]time.Weekday{time.Monday, time.Friday}, time.Monday) {
		t.Error("周一应允许")
	}
	if weekdayAllowed([]time.Weekday{time.Monday}, time.Tuesday) {
		t.Error("周二应不允许")
	}
}

func TestMemoryReviewSource(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC).UnixMilli()
	ctx := Context{Now: time.UnixMilli(now), Reviews: []ReviewItem{
		{ID: "a", NextReviewAt: now - 1000, Summary: "旧事"},
		{ID: "b", NextReviewAt: now + 1000}, // 未到期
		{ID: "", NextReviewAt: now - 1000},  // 空 id 跳过
	}}
	got := MemoryReviewSource{}.Candidates(ctx)
	if len(got) != 1 {
		t.Fatalf("应 1 条到期, got %d", len(got))
	}
	if got[0].Key != "memory_review:a" || got[0].Priority != PriorityNormal || got[0].Body != "旧事" {
		t.Errorf("到期候选不对: %+v", got[0])
	}
}

func TestScheduleSourceWindowAndWeekday(t *testing.T) {
	// 2026-01-01 是周四
	thu := time.Date(2026, 1, 1, 12, 10, 0, 0, time.UTC)
	src := ScheduleSource{Entries: []ScheduleEntry{
		{Key: "lunch", Priority: PriorityNormal, AtMinutes: 12 * 60, WindowMinutes: 30},
	}}
	if got := src.Candidates(Context{Now: thu}); len(got) != 1 {
		t.Fatalf("午餐窗口内应触发, got %d", len(got))
	}
	late := time.Date(2026, 1, 1, 12, 45, 0, 0, time.UTC)
	if got := src.Candidates(Context{Now: late}); len(got) != 0 {
		t.Errorf("窗口外不应触发, got %d", len(got))
	}
	// 星期过滤：只在周一触发
	monOnly := ScheduleSource{Entries: []ScheduleEntry{{Key: "m", AtMinutes: 12 * 60, Weekdays: []time.Weekday{time.Monday}}}}
	if got := monOnly.Candidates(Context{Now: thu}); len(got) != 0 {
		t.Errorf("周四不应触发周一日程, got %d", len(got))
	}
	// 默认窗口：不填 WindowMinutes 用 30
	def := ScheduleSource{Entries: []ScheduleEntry{{Key: "d", AtMinutes: 12 * 60}}}
	if got := def.Candidates(Context{Now: thu}); len(got) != 1 {
		t.Errorf("默认窗口应触发, got %d", len(got))
	}
}

func TestScheduleCrossMidnight(t *testing.T) {
	// 23:50 起 30 分钟窗口，次日 00:10 应命中
	src := ScheduleSource{Entries: []ScheduleEntry{{Key: "night", AtMinutes: 23*60 + 50, WindowMinutes: 30}}}
	at := time.Date(2026, 1, 2, 0, 10, 0, 0, time.UTC)
	if got := src.Candidates(Context{Now: at}); len(got) != 1 {
		t.Errorf("跨零点日程应触发, got %d", len(got))
	}
}

func TestEmotionSource(t *testing.T) {
	src := EmotionSource{Rules: []EmotionRule{
		{Key: "rest", Dimension: "fatigue", Op: OpGE, Threshold: 0.8, Priority: PriorityHigh},
		{Key: "reach", Dimension: "intimacy", Op: OpLE, Threshold: 0.1, Priority: PriorityLow},
	}}
	// 疲劳 0.9 命中、亲密 0.05 命中；好奇存在但无规则应跳过
	got := src.Candidates(Context{Emotion: EmotionState{"fatigue": 0.9, "intimacy": 0.05, "curiosity": 0.5}})
	if len(got) != 2 {
		t.Fatalf("应 2 条规则命中, got %d", len(got))
	}
	// 边界：fatigue=0.8 命中(>=)，intimacy=0.1 命中(<=)
	got = src.Candidates(Context{Emotion: EmotionState{"fatigue": 0.8, "intimacy": 0.1}})
	if len(got) != 2 {
		t.Errorf("边界应命中, got %d", len(got))
	}
	// 不满足
	got = src.Candidates(Context{Emotion: EmotionState{"fatigue": 0.7, "intimacy": 0.2}})
	if len(got) != 0 {
		t.Errorf("不满足不应命中, got %d", len(got))
	}
}

func TestCompareOp(t *testing.T) {
	cases := []struct {
		v    float64
		op   CompareOp
		th   float64
		want bool
	}{
		{1, OpGE, 1, true}, {1, OpGT, 1, false}, {1, OpLE, 1, true}, {0, OpLT, 1, true},
		{1, CompareOp("??"), 1, false},
	}
	for _, c := range cases {
		if got := compareOp(c.v, c.op, c.th); got != c.want {
			t.Errorf("compareOp(%v,%v,%v)=%v want %v", c.v, c.op, c.th, got, c.want)
		}
	}
}

func TestDefaultDataNonEmpty(t *testing.T) {
	if len(DefaultScheduleEntries()) == 0 {
		t.Error("默认日程不应为空")
	}
	if len(DefaultEmotionRules()) == 0 {
		t.Error("默认情绪规则不应为空")
	}
	// DefaultConfig 应为启用态且带静默
	if !DefaultConfig().Enabled {
		t.Error("DefaultConfig 应启用")
	}
}
