package search

import (
	"sync"
	"time"
)

// Cache 结果缓存接口。键为规范化查询。
type Cache interface {
	Get(key string) ([]Result, bool)
	Put(key string, results []Result, ttl time.Duration)
}

// MemoryCache 带 TTL 与容量上限的内存缓存（本地结果缓存）。并发安全，时钟可注入。
type MemoryCache struct {
	mu    sync.Mutex
	now   func() time.Time
	max   int
	items map[string]cacheEntry
}

type cacheEntry struct {
	results []Result
	expire  time.Time
}

// NewMemoryCache 建缓存。maxEntries<=0 用 512；now nil 用 time.Now。
func NewMemoryCache(maxEntries int, now func() time.Time) *MemoryCache {
	if maxEntries <= 0 {
		maxEntries = 512
	}
	if now == nil {
		now = time.Now
	}
	return &MemoryCache{now: now, max: maxEntries, items: make(map[string]cacheEntry)}
}

// Get 取缓存（过期视为未命中并删除）。
func (c *MemoryCache) Get(key string) ([]Result, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok {
		return nil, false
	}
	if c.now().After(e.expire) {
		delete(c.items, key)
		return nil, false
	}
	return e.results, true
}

// Put 写缓存（ttl<=0 不缓存）。超出容量时清理过期项，仍超则淘汰一个。
func (c *MemoryCache) Put(key string, results []Result, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) >= c.max {
		c.evictLocked()
	}
	c.items[key] = cacheEntry{results: results, expire: c.now().Add(ttl)}
}

// evictLocked 清理过期；若仍无空位则删任意一项（简单 LRU 近似）。
func (c *MemoryCache) evictLocked() {
	now := c.now()
	for k, e := range c.items {
		if now.After(e.expire) {
			delete(c.items, k)
		}
	}
	if len(c.items) >= c.max {
		for k := range c.items {
			delete(c.items, k)
			break
		}
	}
}

// Len 当前条目数（观测用）。
func (c *MemoryCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// cacheKey 规范化缓存键（后端+查询全参数）。
func cacheKey(provider string, q Query) string {
	return provider + "|" + q.Text + "|" + q.Lang + "|" + q.Region + "|" + q.Recency + "|" + itoa(normalizeCount(q.Count))
}

// itoa 无 fmt 依赖的整型转串。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
