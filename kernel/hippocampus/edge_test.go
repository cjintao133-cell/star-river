package hippocampus

import (
	"testing"
	"time"
)

func TestClamp01(t *testing.T) {
	cases := map[float64]float64{-1: 0, 0: 0, 0.5: 0.5, 1: 1, 2: 1}
	for in, want := range cases {
		if got := clamp01(in); got != want {
			t.Errorf("clamp01(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestReinforceStabilityClamp(t *testing.T) {
	// r<0 当 0；r>1 当 1。
	if got := reinforceStability(1, -1, 1); !approx(got, 2, 1e-9) {
		t.Errorf("r<0 clamp 后 = %v, want 2", got)
	}
	if got := reinforceStability(1, 2, 1); !approx(got, 1, 1e-9) {
		t.Errorf("r>1 clamp 后 = %v, want 1", got)
	}
}

func TestNextReviewEdge(t *testing.T) {
	if nextReviewAt(100, 0, 0.5) != 100 {
		t.Error("S<=0 应返回 lastReview")
	}
	if nextReviewAt(100, 1, 0) != 100 {
		t.Error("threshold=0 应返回 lastReview")
	}
	if nextReviewAt(100, 1, 1) != 100 {
		t.Error("threshold=1 应返回 lastReview")
	}
}

func TestEngineHelpers(t *testing.T) {
	clk := baseClock()
	e := New(Options{Clock: clk})
	e.Encode("a", "", 1, 1)
	if e.Len() != 1 {
		t.Errorf("Len = %d", e.Len())
	}
	if _, ok := e.Get("ghost"); ok {
		t.Error("Get ghost 应 false")
	}
	if e.Retention("ghost") != 0 {
		t.Error("Retention ghost 应 0")
	}
	// Encode 越界 imp/emo 被夹。
	m := e.Encode("b", "", 3, -1)
	if m.Importance != 1 || m.EmotionIntensity != 0 {
		t.Errorf("clamp: imp=%v emo=%v", m.Importance, m.EmotionIntensity)
	}
}

func TestClocks(t *testing.T) {
	if (RealClock{}).Now().IsZero() {
		t.Error("RealClock.Now 不应为零")
	}
	fc := NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	fc.Set(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if fc.Now().Month() != time.June {
		t.Errorf("FakeClock.Set 未生效: %v", fc.Now())
	}
}
