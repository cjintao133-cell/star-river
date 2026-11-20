package hippocampus

import (
	"math"
	"testing"
	"time"
)

func approx(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func baseClock() *FakeClock {
	return NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
}

// R = exp(-t/S)：注入时钟推进，验证指数衰减。
func TestRetentionDecay(t *testing.T) {
	clk := baseClock()
	e := New(Options{Clock: clk})
	e.Encode("a", "x", 0, 0) // S = base*(1+0+0) = 1 天
	if r := e.Retention("a"); !approx(r, 1.0, 1e-9) {
		t.Errorf("t=0 R = %v, want 1", r)
	}
	clk.Advance(24 * time.Hour) // t=1 天
	if r := e.Retention("a"); !approx(r, math.Exp(-1), 1e-6) {
		t.Errorf("t=1d R = %v, want e^-1=%v", r, math.Exp(-1))
	}
	clk.Advance(24 * time.Hour) // t=2 天
	if r := e.Retention("a"); !approx(r, math.Exp(-2), 1e-6) {
		t.Errorf("t=2d R = %v, want e^-2", r)
	}
}

// 初始稳定度随 importance/emotion 提升。
func TestInitialStability(t *testing.T) {
	e := New(Options{Clock: baseClock()})
	m0 := e.Encode("m0", "", 0, 0)
	if !approx(m0.Stability, 1.0, 1e-9) {
		t.Errorf("imp/emo=0 S = %v, want 1", m0.Stability)
	}
	m1 := e.Encode("m1", "", 1, 1)
	if !approx(m1.Stability, 5.0, 1e-9) { // 1*(1+2+2)
		t.Errorf("imp/emo=1 S = %v, want 5", m1.Stability)
	}
}

// 命中强化 S'=S*(1+k(1-R))：越久没复习（R 越低）强化越多。
func TestReviewReinforcesByForgetting(t *testing.T) {
	// 立即复习：R=1 → S 不变。
	clkA := baseClock()
	eA := New(Options{Clock: clkA})
	eA.Encode("a", "", 0, 0) // S=1
	ma, rA, ok := eA.Review("a")
	if !ok || !approx(rA, 1.0, 1e-9) || !approx(ma.Stability, 1.0, 1e-9) {
		t.Errorf("立即复习: R=%v S=%v (want 1,1)", rA, ma.Stability)
	}
	if ma.ReviewCount != 1 || ma.Strength != 2.0 {
		t.Errorf("Review 计数/强度 = %d/%v", ma.ReviewCount, ma.Strength)
	}

	// 隔 1 天复习：R=e^-1≈0.368 → S'=1*(1+1*(1-0.368))=1.632。
	clkB := baseClock()
	eB := New(Options{Clock: clkB})
	eB.Encode("b", "", 0, 0)
	clkB.Advance(24 * time.Hour)
	mb, rB, _ := eB.Review("b")
	wantR := math.Exp(-1)
	wantS := 1 * (1 + 1*(1-wantR))
	if !approx(rB, wantR, 1e-6) || !approx(mb.Stability, wantS, 1e-6) {
		t.Errorf("隔1天复习: R=%v S=%v, want %v,%v", rB, mb.Stability, wantR, wantS)
	}
	if mb.Stability <= ma.Stability {
		t.Errorf("遗忘后才复习应强化更多: %v vs %v", mb.Stability, ma.Stability)
	}
}

// next_review_at = lastReview + S*ln(1/threshold)。
func TestNextReview(t *testing.T) {
	clk := baseClock()
	e := New(Options{Clock: clk, ReviewThreshold: 0.5})
	m := e.Encode("a", "", 0, 0)        // S=1
	wantDays := 1.0 * math.Log(1.0/0.5) // ≈0.693
	wantMs := int64(wantDays * float64(dayMillis))
	if got := m.NextReviewAt - m.CreatedAt; got != wantMs {
		t.Errorf("next_review 间隔 = %d ms, want %d", got, wantMs)
	}
}

func TestReviewUnknown(t *testing.T) {
	e := New(Options{Clock: baseClock()})
	if _, _, ok := e.Review("ghost"); ok {
		t.Error("未知 id Review 应 false")
	}
}
