package search

import (
	"context"
	"sync"
	"time"
)

// ProviderStats 单后端统计（观测/排障）。
type ProviderStats struct {
	Attempts  int
	Hits      int
	Cached    int
	RateLimit int
	Fails     int
	LastErr   string
}

// SearchOutcome 一次搜索的结果。
type SearchOutcome struct {
	Results  []Result
	Provider string // 实际命中后端
	Cached   bool
}

type routerEntry struct {
	p      Provider
	bucket *TokenBucket
	stats  ProviderStats
}

// Router 多后端主备路由：按添加顺序尝试，失败/限流/无额度→降级到下一个；带本地缓存。
// 并发安全（stats 用互斥保护；各 Provider 自身并发安全）。
type Router struct {
	mu      sync.Mutex
	entries []*routerEntry
	cache   Cache
	ttl     time.Duration
}

// NewRouter 建路由。cache 可 nil（不缓存）；ttl<=0 用默认 24h。
func NewRouter(cache Cache, ttl time.Duration) *Router {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Router{cache: cache, ttl: ttl}
}

// Add 追加一个后端（顺序即主备优先级）。bucket 可 nil（不限流）。
func (r *Router) Add(p Provider, bucket *TokenBucket) {
	r.mu.Lock()
	r.entries = append(r.entries, &routerEntry{p: p, bucket: bucket})
	r.mu.Unlock()
}

// SetTTL 设置缓存时长（时效类查询可在调用方用更短 ttl，此处为默认）。
func (r *Router) SetTTL(ttl time.Duration) {
	if ttl > 0 {
		r.ttl = ttl
	}
}

// ProviderNames 已注册后端名（按主备顺序）。
func (r *Router) ProviderNames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e.p.Name())
	}
	return out
}

// Search 依次尝试各后端：缓存命中→返回；限流→跳过；出错→降级下一个。
// 全部失败返回最后一个错误（无后端返回 ErrNoProvider）。
func (r *Router) Search(ctx context.Context, q Query) (SearchOutcome, error) {
	r.mu.Lock()
	entries := append([]*routerEntry(nil), r.entries...)
	r.mu.Unlock()
	if len(entries) == 0 {
		return SearchOutcome{}, ErrNoProvider
	}

	var lastErr error
	for _, e := range entries {
		name := e.p.Name()
		key := cacheKey(name, q)

		// 缓存命中（先于限流，命中不耗额度）。
		if r.cache != nil {
			if rs, ok := r.cache.Get(key); ok && len(rs) > 0 {
				r.bump(name, func(s *ProviderStats) { s.Cached++ })
				return SearchOutcome{Results: rs, Provider: name, Cached: true}, nil
			}
		}

		// 本地令牌桶限流：无令牌则跳过该后端（不阻断主备）。
		if e.bucket != nil && !e.bucket.Allow() {
			r.bump(name, func(s *ProviderStats) { s.RateLimit++ })
			continue
		}

		r.bump(name, func(s *ProviderStats) { s.Attempts++ })
		rs, err := e.p.Search(ctx, q)
		if err != nil {
			lastErr = err
			msg := err.Error()
			r.bump(name, func(s *ProviderStats) { s.Fails++; s.LastErr = msg })
			continue
		}
		r.bump(name, func(s *ProviderStats) { s.Hits++ })
		if r.cache != nil {
			r.cache.Put(key, rs, r.ttl)
		}
		return SearchOutcome{Results: rs, Provider: name, Cached: false}, nil
	}
	if lastErr != nil {
		return SearchOutcome{}, lastErr
	}
	// 全部被限流或被跳过。
	return SearchOutcome{}, ErrRateLimited
}

// bump 更新某后端统计。
func (r *Router) bump(name string, fn func(*ProviderStats)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.entries {
		if e.p.Name() == name {
			fn(&e.stats)
			return
		}
	}
}

// Stats 返回各后端统计快照。
func (r *Router) Stats() map[string]ProviderStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]ProviderStats, len(r.entries))
	for _, e := range r.entries {
		out[e.p.Name()] = e.stats
	}
	return out
}
