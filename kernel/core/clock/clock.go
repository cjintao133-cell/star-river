// Package clock 内核统一可注入时钟（T-M3-06）。
//
// 依赖方向（红线2）：本包为内核底座，任何模块（含 hippocampus/proactive/endocrine
// 三个能力插件包）都可 import；本包不 import 业务包。全系统时间源经此注入，
// 使衰减/情绪/调度等时间相关逻辑可被假时钟确定性测试（月测/年测）。
package clock

import "time"

// Clock 可注入时钟。
type Clock interface {
	Now() time.Time
}

// RealClock 真实时钟（生产默认）。
type RealClock struct{}

// Now 返回当前时间。
func (RealClock) Now() time.Time { return time.Now() }

// FakeClock 测试用可控时钟（非并发安全；测试单线程推进）。
type FakeClock struct {
	t time.Time
}

// NewFakeClock 以给定起点建假时钟。
func NewFakeClock(start time.Time) *FakeClock { return &FakeClock{t: start} }

// Now 返回假时钟当前时刻。
func (c *FakeClock) Now() time.Time { return c.t }

// Advance 推进 d。
func (c *FakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

// Set 直接设到某时刻。
func (c *FakeClock) Set(t time.Time) { c.t = t }
