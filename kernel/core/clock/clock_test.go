package clock

import (
	"testing"
	"time"
)

func TestRealClock(t *testing.T) {
	if (RealClock{}).Now().IsZero() {
		t.Error("RealClock.Now 不应为零")
	}
}

func TestFakeClock(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fc := NewFakeClock(start)
	if !fc.Now().Equal(start) {
		t.Errorf("Now = %v", fc.Now())
	}
	fc.Advance(48 * time.Hour)
	if !fc.Now().Equal(start.Add(48 * time.Hour)) {
		t.Errorf("Advance 后 = %v", fc.Now())
	}
	fc.Set(start.Add(-time.Hour))
	if !fc.Now().Equal(start.Add(-time.Hour)) {
		t.Errorf("Set 后 = %v", fc.Now())
	}
}
