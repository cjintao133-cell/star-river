// Package cronexpr 解析标准 5 字段 cron 表达式（分 时 日 月 周）并计算下次触发时间。
//
// 支持：*、*/n、a、a-b、a-b/n、a,b,c 及其组合。周字段 0/7=周日。
// 语义：日(dom) 与 周(dow) 都被限制时，两者皆满足才触发（可预测优先）。
// 纯逻辑、零依赖，便于离线测试。
package cronexpr

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule 已解析的 cron 调度。
type Schedule struct {
	min  map[int]bool
	hour map[int]bool
	dom  map[int]bool
	mon  map[int]bool
	dow  map[int]bool
}

// Parse 解析 5 字段表达式："分 时 日 月 周"。
func Parse(expr string) (*Schedule, error) {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return nil, fmt.Errorf("cronexpr: need 5 fields, got %d", len(fields))
	}
	var err error
	s := &Schedule{}
	if s.min, err = parseField(fields[0], 0, 59); err != nil {
		return nil, fmt.Errorf("minute: %w", err)
	}
	if s.hour, err = parseField(fields[1], 0, 23); err != nil {
		return nil, fmt.Errorf("hour: %w", err)
	}
	if s.dom, err = parseField(fields[2], 1, 31); err != nil {
		return nil, fmt.Errorf("day-of-month: %w", err)
	}
	if s.mon, err = parseField(fields[3], 1, 12); err != nil {
		return nil, fmt.Errorf("month: %w", err)
	}
	dow, err := parseField(fields[4], 0, 7)
	if err != nil {
		return nil, fmt.Errorf("day-of-week: %w", err)
	}
	// 归一 7 -> 0（周日）
	if dow[7] {
		dow[0] = true
		delete(dow, 7)
	}
	s.dow = dow
	return s, nil
}

func parseField(f string, min, max int) (map[int]bool, error) {
	out := map[int]bool{}
	for _, part := range strings.Split(f, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("empty list item")
		}
		step := 1
		if i := strings.Index(part, "/"); i >= 0 {
			sv := part[i+1:]
			n, err := strconv.Atoi(sv)
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("bad step %q", sv)
			}
			step = n
			part = part[:i]
		}
		var lo, hi int
		switch {
		case part == "*":
			lo, hi = min, max
		case strings.Contains(part, "-"):
			ab := strings.SplitN(part, "-", 2)
			a, e1 := strconv.Atoi(strings.TrimSpace(ab[0]))
			b, e2 := strconv.Atoi(strings.TrimSpace(ab[1]))
			if e1 != nil || e2 != nil {
				return nil, fmt.Errorf("bad range %q", part)
			}
			lo, hi = a, b
		default:
			v, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("bad value %q", part)
			}
			lo, hi = v, v
		}
		if lo < min || hi > max || lo > hi {
			return nil, fmt.Errorf("out of range %d-%d (%d..%d)", lo, hi, min, max)
		}
		for v := lo; v <= hi; v += step {
			out[v] = true
		}
	}
	return out, nil
}

// Next 返回严格晚于 after 的下一个触发时刻（分钟精度）。找不到（如 2月30日）返回零值。
func (s *Schedule) Next(after time.Time) time.Time {
	t := after.Truncate(time.Minute).Add(time.Minute)
	limit := t.AddDate(5, 0, 0) // 最多向前找 5 年
	for t.Before(limit) {
		if s.match(t) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}
}

func (s *Schedule) match(t time.Time) bool {
	if !s.min[t.Minute()] || !s.hour[t.Hour()] || !s.mon[int(t.Month())] {
		return false
	}
	if !s.dom[t.Day()] {
		return false
	}
	// Go: Sunday=0..Saturday=6，与 cron dow 对齐
	if !s.dow[int(t.Weekday())] {
		return false
	}
	return true
}
