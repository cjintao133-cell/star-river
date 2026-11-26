package cronexpr

import (
	"testing"
	"time"
)

func utc(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, time.UTC)
}

func TestParseErrors(t *testing.T) {
	for _, bad := range []string{"", "* * *", "* * * * * *", "60 * * * *", "*/0 * * * *", "a * * * *", "* * * * 8"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q 应解析失败", bad)
		}
	}
	for _, ok := range []string{"* * * * *", "*/15 * * * *", "0 9 * * 1-5", "0 0 1,15 * *", "5 4 * * 0"} {
		if _, err := Parse(ok); err != nil {
			t.Errorf("%q 应解析成功: %v", ok, err)
		}
	}
}

func TestNextEvery15(t *testing.T) {
	s, _ := Parse("*/15 * * * *")
	got := s.Next(utc(2026, 1, 1, 8, 7))
	want := utc(2026, 1, 1, 8, 15)
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	// 刚好在 :15 时，应到 :30
	got = s.Next(utc(2026, 1, 1, 8, 15))
	if !got.Equal(utc(2026, 1, 1, 8, 30)) {
		t.Fatalf("边界 got %v", got)
	}
}

func TestNextDaily9am(t *testing.T) {
	s, _ := Parse("0 9 * * *")
	got := s.Next(utc(2026, 1, 1, 8, 30))
	if !got.Equal(utc(2026, 1, 1, 9, 0)) {
		t.Fatalf("got %v", got)
	}
	// 过了 9 点 -> 明天 9 点
	got = s.Next(utc(2026, 1, 1, 9, 1))
	if !got.Equal(utc(2026, 1, 2, 9, 0)) {
		t.Fatalf("跨天 got %v", got)
	}
}

func TestNextWeekdayRange(t *testing.T) {
	// 2026-01-01 是周四; "0 9 * * 1-5" 下一个工作日 09:00
	s, _ := Parse("0 9 * * 1-5")
	got := s.Next(utc(2026, 1, 1, 10, 0)) // 周四 10 点 -> 周五 9 点
	if got.Weekday() != time.Friday || got.Hour() != 9 {
		t.Fatalf("got %v (%v)", got, got.Weekday())
	}
}

func TestNextMonthStart(t *testing.T) {
	s, _ := Parse("0 0 1 * *")
	got := s.Next(utc(2026, 1, 15, 0, 0))
	if !got.Equal(utc(2026, 2, 1, 0, 0)) {
		t.Fatalf("got %v", got)
	}
}

func TestSundaySeven(t *testing.T) {
	s, _ := Parse("0 0 * * 7") // 7 归一为周日
	got := s.Next(utc(2026, 1, 1, 0, 0))
	if got.Weekday() != time.Sunday {
		t.Fatalf("7 应= 周日, got %v", got.Weekday())
	}
}
