package proactive

import (
	"sync"
	"testing"
	"time"

	"star-river/kernel/core/clock"
)

func fakeAt(h, mi int) *clock.FakeClock {
	return clock.NewFakeClock(time.Date(2026, 1, 1, h, mi, 0, 0, time.UTC))
}

// recorder 收集出口。
type recorder struct {
	mu  sync.Mutex
	got []Nudge
}

func (r *recorder) Emit(n Nudge) {
	r.mu.Lock()
	r.got = append(r.got, n)
	r.mu.Unlock()
}

func TestDisabledByDefault(t *testing.T) {
	s := New(Config{}, fakeAt(12, 0), MemoryReviewSource{})
	// Enabled 零值 = false
	got := s.Tick([]ReviewItem{{ID: "a", NextReviewAt: 0}}, nil)
	if len(got) != 0 {
		t.Errorf("默认关闭不应发出, got %d", len(got))
	}
}

func TestMemoryReviewFiresWhenEnabled(t *testing.T) {
	s := New(Config{Enabled: true, DefaultCooldown: time.Hour}, fakeAt(12, 0), MemoryReviewSource{})
	got := s.Tick([]ReviewItem{{ID: "a", NextReviewAt: 0}}, nil)
	if len(got) != 1 || got[0].Key != "memory_review:a" {
		t.Fatalf("应发出 1 条复习提醒, got %+v", got)
	}
}

func TestCooldownDedup(t *testing.T) {
	clk := fakeAt(12, 0)
	s := New(Config{Enabled: true, DefaultCooldown: 2 * time.Hour, Rate: RateLimit{}}, clk, MemoryReviewSource{})
	rev := []ReviewItem{{ID: "a", NextReviewAt: 0}}
	if got := s.Tick(rev, nil); len(got) != 1 {
		t.Fatalf("首次应发出")
	}
	clk.Advance(time.Hour) // 仍在冷却
	if got := s.Tick(rev, nil); len(got) != 0 {
		t.Errorf("冷却内不应再发, got %d", len(got))
	}
	clk.Advance(time.Hour) // 累计 2h，过冷却
	if got := s.Tick(rev, nil); len(got) != 1 {
		t.Errorf("过冷却应再发, got %d", len(got))
	}
}

func TestPriorityOrderingAndRateWindow(t *testing.T) {
	clk := fakeAt(12, 0)
	src := staticSource{kind: SourceEmotion, cands: []Candidate{
		{Source: SourceEmotion, Key: "low", Priority: PriorityLow},
		{Source: SourceEmotion, Key: "high", Priority: PriorityHigh},
		{Source: SourceEmotion, Key: "normal", Priority: PriorityNormal},
	}}
	s := New(Config{Enabled: true, DefaultCooldown: time.Hour, Rate: RateLimit{MaxPerWindow: 1, Window: time.Hour}}, clk, src)
	got := s.Tick(nil, nil)
	if len(got) != 1 || got[0].Key != "high" {
		t.Fatalf("限频 1 时应只发最高优先级 high, got %+v", got)
	}
	// 限频窗口内，再 tick 也不发（其他候选仍受窗口限制）
	clk.Advance(30 * time.Minute)
	if got := s.Tick(nil, nil); len(got) != 0 {
		t.Errorf("窗口内不应再发, got %d", len(got))
	}
}

func TestDailyMax(t *testing.T) {
	clk := fakeAt(9, 0)
	rev := []ReviewItem{{ID: "a", NextReviewAt: 0}, {ID: "b", NextReviewAt: 0}, {ID: "c", NextReviewAt: 0}}
	s := New(Config{Enabled: true, DefaultCooldown: time.Minute, Rate: RateLimit{DailyMax: 2}}, clk, MemoryReviewSource{})
	got := s.Tick(rev, nil)
	if len(got) != 2 {
		t.Fatalf("每日上限 2 应只发 2, got %d", len(got))
	}
	clk.Advance(2 * time.Minute) // 同日，已达上限
	if got = s.Tick(rev, nil); len(got) != 0 {
		t.Errorf("同日达上限不应再发, got %d", len(got))
	}
	clk.Advance(24 * time.Hour) // 次日重置
	if got = s.Tick(rev, nil); len(got) != 2 {
		t.Errorf("次日应重置并发 2, got %d", len(got))
	}
}

func TestQuietHoursDeferNonUrgent(t *testing.T) {
	clk := fakeAt(23, 30) // 静默 23:00–07:00
	cfg := Config{Enabled: true, DefaultCooldown: time.Minute,
		Quiet: QuietHours{Enabled: true, StartMinutes: 23 * 60, EndMinutes: 7 * 60}}
	s := New(cfg, clk, MemoryReviewSource{})
	rev := []ReviewItem{{ID: "a", NextReviewAt: 0}}
	if got := s.Tick(rev, nil); len(got) != 0 {
		t.Errorf("静默时段非紧急不应发, got %d", len(got))
	}
	clk.Advance(7*time.Hour + 30*time.Minute) // 到 07:00 解禁
	if got := s.Tick(rev, nil); len(got) != 1 {
		t.Errorf("解禁后应发出, got %d", len(got))
	}
}

func TestQuietHoursUrgentBypass(t *testing.T) {
	clk := fakeAt(2, 0) // 静默中
	cfg := Config{Enabled: true, DefaultCooldown: time.Minute,
		Quiet: QuietHours{Enabled: true, StartMinutes: 23 * 60, EndMinutes: 7 * 60}}
	src := staticSource{kind: SourceEmotion, cands: []Candidate{
		{Source: SourceEmotion, Key: "u", Priority: PriorityUrgent, Title: "紧急"},
	}}
	s := New(cfg, clk, src)
	if got := s.Tick(nil, nil); len(got) != 1 {
		t.Errorf("紧急应穿透静默, got %d", len(got))
	}
}

func TestSourceToggle(t *testing.T) {
	s := New(Config{Enabled: true, DefaultCooldown: time.Minute, Rate: RateLimit{}}, fakeAt(12, 0),
		ScheduleSource{Entries: DefaultScheduleEntries()})
	if got := s.Tick(nil, nil); len(got) != 1 {
		t.Fatalf("应触发午餐日程, got %d", len(got))
	}
	s.SetSourceEnabled(SourceSchedule, false)
	if s.SourceEnabled(SourceSchedule) {
		t.Error("SourceEnabled 应 false")
	}
	// 新调度器同刻、源禁用 → 不触发
	s2 := New(Config{Enabled: true, DefaultCooldown: time.Minute, Rate: RateLimit{}}, fakeAt(12, 0),
		ScheduleSource{Entries: DefaultScheduleEntries()})
	s2.SetSourceEnabled(SourceSchedule, false)
	if got := s2.Tick(nil, nil); len(got) != 0 {
		t.Errorf("源禁用不应触发, got %d", len(got))
	}
}

func TestMasterToggle(t *testing.T) {
	s := New(Config{Enabled: true, DefaultCooldown: time.Minute, Rate: RateLimit{}}, fakeAt(12, 0), MemoryReviewSource{})
	s.SetEnabled(false)
	if s.Enabled() {
		t.Error("Enabled 应 false")
	}
	if got := s.Tick([]ReviewItem{{ID: "a", NextReviewAt: 0}}, nil); len(got) != 0 {
		t.Errorf("总开关关闭不应发, got %d", len(got))
	}
	s.SetEnabled(true)
	if got := s.Tick([]ReviewItem{{ID: "a", NextReviewAt: 0}}, nil); len(got) != 1 {
		t.Errorf("重开应发, got %d", len(got))
	}
}

func TestEmitterCalled(t *testing.T) {
	s := New(Config{Enabled: true, DefaultCooldown: time.Minute, Rate: RateLimit{}}, fakeAt(12, 0), MemoryReviewSource{})
	r := &recorder{}
	s.SetEmitter(r)
	s.Tick([]ReviewItem{{ID: "a", NextReviewAt: 0}}, nil)
	if len(r.got) != 1 || r.got[0].Key != "memory_review:a" {
		t.Errorf("Emitter 应收到 1 条, got %+v", r.got)
	}
}

func TestDedupeWithinTick(t *testing.T) {
	a := Candidate{Source: SourceEmotion, Key: "dup", Priority: PriorityLow}
	b := Candidate{Source: SourceMemoryReview, Key: "dup", Priority: PriorityHigh}
	got := dedupeWithinTick([]Candidate{a, b})
	if len(got) != 1 || got[0].Priority != PriorityHigh {
		t.Errorf("同轮去重应保留高优先级, got %+v", got)
	}
}

func TestSnapshotRestore(t *testing.T) {
	clk := fakeAt(12, 0)
	cfg := Config{Enabled: true, DefaultCooldown: 2 * time.Hour, Rate: RateLimit{MaxPerWindow: 5, Window: time.Hour}}
	s := New(cfg, clk, MemoryReviewSource{})
	rev := []ReviewItem{{ID: "a", NextReviewAt: 0}}
	if got := s.Tick(rev, nil); len(got) != 1 {
		t.Fatalf("首次应发")
	}
	sn := s.Snapshot()

	s2 := New(cfg, clk, MemoryReviewSource{})
	s2.Restore(sn)
	if !s2.Enabled() {
		t.Error("恢复后应保持 Enabled")
	}
	if got := s2.Tick(rev, nil); len(got) != 0 {
		t.Errorf("恢复冷却后不应再发, got %d", len(got))
	}
}

func TestRestoreUnknownSourceIgnored(t *testing.T) {
	s := New(Config{Enabled: true, DefaultCooldown: time.Minute, Rate: RateLimit{}}, fakeAt(12, 0), MemoryReviewSource{})
	s.Restore(Snapshot{Enabled: true, Sources: map[string]bool{"bogus": true, "memory_review": false}})
	if s.SourceEnabled(SourceMemoryReview) {
		t.Error("已知源应恢复到 false")
	}
	// bogus 不应产生任何源条目（SourceEnabled 对未知 key 返回 false）
	if s.SourceEnabled(SourceKind("bogus")) {
		t.Error("未知源应不存在")
	}
}

func TestConcurrentTickNoRace(t *testing.T) {
	// FakeClock 非并发安全，这里用 RealClock；多 goroutine Tick 不应 panic/race。
	s := New(Config{Enabled: true, DefaultCooldown: time.Hour, Rate: RateLimit{MaxPerWindow: 1, Window: time.Hour}},
		clock.RealClock{}, MemoryReviewSource{})
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Tick([]ReviewItem{{ID: "a", NextReviewAt: 0}}, nil)
		}()
	}
	wg.Wait()
}

func TestPriorityString(t *testing.T) {
	cases := map[Priority]string{
		PriorityLow: "low", PriorityNormal: "normal", PriorityHigh: "high",
		PriorityUrgent: "urgent", Priority(99): "unknown",
	}
	for p, want := range cases {
		if got := p.String(); got != want {
			t.Errorf("Priority(%d).String()=%q want %q", p, got, want)
		}
	}
}

// staticSource 测试用固定候选源（纯函数）。
type staticSource struct {
	kind  SourceKind
	cands []Candidate
}

func (s staticSource) Kind() SourceKind               { return s.kind }
func (s staticSource) Candidates(Context) []Candidate { return s.cands }
