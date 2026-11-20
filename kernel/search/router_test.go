package search

import (
	"context"
	"errors"
	"testing"
	"time"
)

// stubProvider 可配置行为的测试后端。
type stubProvider struct {
	name  string
	caps  Capabilities
	res   []Result
	err   error
	calls int
}

func (s *stubProvider) Name() string                      { return s.name }
func (s *stubProvider) Caps() Capabilities                { return s.caps }
func (s *stubProvider) Healthy(ctx context.Context) error { return s.err }
func (s *stubProvider) Search(ctx context.Context, q Query) ([]Result, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.res, nil
}

func TestRouterFailover(t *testing.T) {
	a := &stubProvider{name: "a", err: &APIError{Kind: KindHTTP, Message: "down"}}
	b := &stubProvider{name: "b", res: []Result{{Title: "B", URL: "u"}}}
	r := NewRouter(nil, 0)
	r.Add(a, nil)
	r.Add(b, nil)
	out, err := r.Search(context.Background(), Query{Text: "q"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Provider != "b" || len(out.Results) != 1 || out.Results[0].Title != "B" {
		t.Fatalf("应降级到 b, got %+v", out)
	}
	if a.calls != 1 || b.calls != 1 {
		t.Errorf("调用次数不对: a=%d b=%d", a.calls, b.calls)
	}
}

func TestRouterAllFail(t *testing.T) {
	a := &stubProvider{name: "a", err: errors.New("a down")}
	b := &stubProvider{name: "b", err: errors.New("b down")}
	r := NewRouter(nil, 0)
	r.Add(a, nil)
	r.Add(b, nil)
	_, err := r.Search(context.Background(), Query{Text: "q"})
	if err == nil {
		t.Fatal("全失败应报错")
	}
}

func TestRouterNoProvider(t *testing.T) {
	r := NewRouter(nil, 0)
	if _, err := r.Search(context.Background(), Query{Text: "q"}); !errors.Is(err, ErrNoProvider) {
		t.Errorf("无后端应 ErrNoProvider, got %v", err)
	}
}

func TestRouterCache(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cache := NewMemoryCache(100, func() time.Time { return now })
	a := &stubProvider{name: "a", res: []Result{{Title: "cached"}}}
	r := NewRouter(cache, time.Hour)
	r.Add(a, nil)
	// 首次：走 provider
	out, _ := r.Search(context.Background(), Query{Text: "q"})
	if out.Cached || a.calls != 1 {
		t.Fatalf("首次应不缓存且调用1: %+v calls=%d", out, a.calls)
	}
	// 二次：命中缓存，不再调用
	out, _ = r.Search(context.Background(), Query{Text: "q"})
	if !out.Cached || a.calls != 1 {
		t.Fatalf("二次应命中缓存: %+v calls=%d", out, a.calls)
	}
	// 过期后：再次调用
	now = now.Add(2 * time.Hour)
	out, _ = r.Search(context.Background(), Query{Text: "q"})
	if out.Cached || a.calls != 2 {
		t.Errorf("过期后应重新调用: %+v calls=%d", out, a.calls)
	}
}

func TestRouterRateLimitSkip(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	a := &stubProvider{name: "a", res: []Result{{Title: "A"}}}
	b := &stubProvider{name: "b", res: []Result{{Title: "B"}}}
	// a 桶：容量1、恢复率0.001/s（几乎不恢复），首次可用后耗尽
	bucketA := NewTokenBucket(0.001, 1, clock)
	r := NewRouter(nil, 0)
	r.Add(a, bucketA)
	r.Add(b, nil)
	// 第一次走 a
	out, _ := r.Search(context.Background(), Query{Text: "q1"})
	if out.Provider != "a" {
		t.Fatalf("首次应走 a, got %s", out.Provider)
	}
	// 第二次 a 限流 → 降级 b
	out, _ = r.Search(context.Background(), Query{Text: "q2"})
	if out.Provider != "b" {
		t.Errorf("a 限流后应降级 b, got %s", out.Provider)
	}
	st := r.Stats()
	if st["a"].RateLimit < 1 {
		t.Errorf("a 应记录限流, stats=%+v", st["a"])
	}
}

func TestRouterStats(t *testing.T) {
	a := &stubProvider{name: "a", res: []Result{{Title: "A"}}}
	r := NewRouter(nil, 0)
	r.Add(a, nil)
	_, _ = r.Search(context.Background(), Query{Text: "q"})
	st := r.Stats()
	if st["a"].Attempts != 1 || st["a"].Hits != 1 {
		t.Errorf("统计不对: %+v", st["a"])
	}
	names := r.ProviderNames()
	if len(names) != 1 || names[0] != "a" {
		t.Errorf("ProviderNames 不对: %v", names)
	}
}

func TestMemoryCache(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := NewMemoryCache(2, func() time.Time { return now })
	c.Put("k1", []Result{{Title: "x"}}, time.Minute)
	if rs, ok := c.Get("k1"); !ok || len(rs) != 1 {
		t.Fatal("应命中")
	}
	// 过期
	now = now.Add(2 * time.Minute)
	if _, ok := c.Get("k1"); ok {
		t.Error("过期应未命中")
	}
	// ttl<=0 不缓存
	c.Put("k2", []Result{{}}, 0)
	if _, ok := c.Get("k2"); ok {
		t.Error("ttl<=0 不应缓存")
	}
}

func TestMemoryCacheEvict(t *testing.T) {
	c := NewMemoryCache(2, nil)
	c.Put("a", []Result{{Title: "a"}}, time.Hour)
	c.Put("b", []Result{{Title: "b"}}, time.Hour)
	c.Put("c", []Result{{Title: "c"}}, time.Hour) // 超容量触发淘汰
	if c.Len() > 2 {
		t.Errorf("应保持 <=2, got %d", c.Len())
	}
}

func TestTokenBucket(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	b := NewTokenBucket(1, 2, clock) // 2/s 容量2
	if !b.Allow() {
		t.Fatal("第1个令牌应允许")
	}
	if !b.Allow() {
		t.Fatal("第2个令牌应允许")
	}
	if b.Allow() {
		t.Error("耗尽后应拒")
	}
	now = now.Add(time.Second) // 恢复 1
	if !b.Allow() {
		t.Error("1 秒后应恢复 1")
	}
	// 不限流
	u := NewTokenBucket(0, 0, clock)
	for i := 0; i < 100; i++ {
		if !u.Allow() {
			t.Fatal("rate<=0 应恒允许")
		}
	}
}
