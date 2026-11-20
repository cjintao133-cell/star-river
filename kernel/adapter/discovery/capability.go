package discovery

import (
	"context"
	"sync"

	"star-river/kernel/core/types"
)

// ChannelKey 能力台账的渠道键："同一 base_url + model 视为独立渠道"（增补 P2）。
type ChannelKey struct {
	BaseURL string
	ModelID string
}

// Capability 一条渠道能力台账。
// Available=true 表示懒校验探测成功；LastError 为最近一次探测失败原因（已脱敏），成功为空。
type Capability struct {
	Key       ChannelKey
	Available bool
	Caps      types.ModelCapabilities
	LastError string
}

// CapabilityStore 能力台账持久化抽象。M1 用 MemoryStore 供测试；
// M2 起由 kernel/store（SQLite）实现同一接口，键为 ChannelKey。
type CapabilityStore interface {
	Put(c Capability)
	Get(key ChannelKey) (Capability, bool)
}

// MemoryStore CapabilityStore 的内存实现（并发安全）。
type MemoryStore struct {
	mu sync.RWMutex
	m  map[ChannelKey]Capability
}

// NewMemoryStore 建一个空内存台账。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{m: make(map[ChannelKey]Capability)}
}

// Put 写入/覆盖一条台账。
func (s *MemoryStore) Put(c Capability) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[c.Key] = c
}

// Get 读取一条台账。
func (s *MemoryStore) Get(key ChannelKey) (Capability, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.m[key]
	return c, ok
}

// Prober 发极短探测请求，返回探测到的能力位（流式/视觉/工具/推理）。
// 由各协议适配器在集成层实现（用极小 max_tokens 发一发探测）；本包只编排与落库。
// 约定：失败务必返回已脱敏错误（复用适配器的 *APIError），本包会原样记入 LastError。
type Prober interface {
	Probe(ctx context.Context, modelID string) (types.ModelCapabilities, error)
}

// ProberFunc 把函数适配为 Prober（便于测试与轻量接线）。
type ProberFunc func(ctx context.Context, modelID string) (types.ModelCapabilities, error)

// Probe 实现 Prober。
func (f ProberFunc) Probe(ctx context.Context, modelID string) (types.ModelCapabilities, error) {
	return f(ctx, modelID)
}

// LazyValidate 选中模型时调用：发极短探测请求，成功点亮可用标记并记能力位；
// 失败标记不可用并记原因（失败路径不许静默成功）。结果经 store 落库（键=渠道）。
// 返回本次探测后的台账条目。探测失败属预期分支，不 panic、不吞错。
func LazyValidate(ctx context.Context, store CapabilityStore, key ChannelKey, prober Prober) Capability {
	caps, err := prober.Probe(ctx, key.ModelID)
	if err != nil {
		c := Capability{
			Key:       key,
			Available: false,
			Caps:      types.ModelCapabilities{ModelID: key.ModelID},
			LastError: err.Error(),
		}
		store.Put(c)
		return c
	}
	caps.ModelID = key.ModelID
	c := Capability{Key: key, Available: true, Caps: caps}
	store.Put(c)
	return c
}
