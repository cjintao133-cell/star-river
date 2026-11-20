package search

import (
	"sync"
	"time"
)

// TokenBucket 令牌桶限流器（每后端一个）。并发安全，时钟可注入。
// 容量=bucket，按 rate 个/秒恢复。
type TokenBucket struct {
	mu     sync.Mutex
	now    func() time.Time
	rate   float64 // 每秒恢复令牌
	burst  float64 // 桶容量
	tokens float64
	last   time.Time
}

// NewTokenBucket 建令牌桶。rate<=0 视为不限流（不拦截）；burst<=0 取 max(1,rate)。
// now nil 用 time.Now。
func NewTokenBucket(ratePerSec float64, burst int, now func() time.Time) *TokenBucket {
	if now == nil {
		now = time.Now
	}
	b := float64(burst)
	if b <= 0 {
		b = ratePerSec
		if b < 1 {
			b = 1
		}
	}
	return &TokenBucket{now: now, rate: ratePerSec, burst: b, tokens: b, last: now()}
}

// Allow 尝试取 1 个令牌；无限流(rate<=0)恒真。
func (t *TokenBucket) Allow() bool {
	if t.rate <= 0 {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	elapsed := now.Sub(t.last).Seconds()
	if elapsed > 0 {
		t.tokens += elapsed * t.rate
		if t.tokens > t.burst {
			t.tokens = t.burst
		}
		t.last = now
	}
	if t.tokens >= 1 {
		t.tokens -= 1
		return true
	}
	return false
}

// Available 当前可用令牌数（观测/测试用）。
func (t *TokenBucket) Available() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.tokens
}
