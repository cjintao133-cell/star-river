// Package endocrine 是情绪引擎（能力插件包，T-M4-01 五维状态机）。
//
// 依赖方向（红线 2）：底座不得 import 本包；本包可 import 底座开源包（core/clock）。
//
// 设计：**数据驱动的维度注册表**（维度表 schema 可扩展，约束#8）。每个维度是一条
// Dimension（key/范围/基线/衰减率）。加一个维度 = 注册一条数据，不改状态机代码；
// 关闭一个维度 = 注销。默认五维（效价/唤醒/亲密/疲劳/好奇）；Dominance(掌控感,PAD 第三维)
// 作为"验证扩展"预留（DefaultRegistry 不注册，示例见 DominanceDimension）。
//
// 推演双路径（PRD 验收）：
//   - 对话触发 Interact(deltas)：先按经过时间衰减，再叠加增量并夹范围。
//   - 后台定时 Tick()：仅按经过时间向各维度基线指数衰减。
//
// 时钟经 core/clock 注入（T-M3-06），衰减函数可在假时钟上确定性单测。
package endocrine

import (
	"errors"
	"math"
	"sync"
	"time"

	"star-river/kernel/core/clock"
)

// 错误。
var (
	ErrBadDimension = errors.New("endocrine: invalid dimension (min<baseline<max required)")
	ErrUnknownDim   = errors.New("endocrine: unknown dimension")
)

// Dimension 一个情绪维度（数据驱动）。衰减每单位时间按指数向 Baseline 收敛。
type Dimension struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
	Baseline    float64 `json:"baseline"`      // 静息值（衰减归宿）
	DecayPerDay float64 `json:"decay_per_day"` // 向基线收敛速率(1/天)；<=0 不衰减
}

// valid 校验范围与基线合理。
func (d Dimension) valid() bool {
	return d.Key != "" && d.Min < d.Max && d.Baseline >= d.Min && d.Baseline <= d.Max
}

// clamp 夹到 [Min,Max]。
func (d Dimension) clamp(v float64) float64 {
	if v < d.Min {
		return d.Min
	}
	if v > d.Max {
		return d.Max
	}
	return v
}

// decayToward 返回 v 经 dtDays 后向基线指数收敛的值。
// v' = baseline + (v-baseline) * exp(-DecayPerDay*dt)。DecayPerDay<=0 时不动。
func (d Dimension) decayToward(v, dtDays float64) float64 {
	if d.DecayPerDay <= 0 || dtDays <= 0 {
		return d.clamp(v)
	}
	return d.clamp(d.Baseline + (v-d.Baseline)*math.Exp(-d.DecayPerDay*dtDays))
}

// —— 维度注册表 ——

// Registry 维度注册表（可扩展、可注销）。并发安全。
type Registry struct {
	mu    sync.RWMutex
	dims  map[string]Dimension
	order []string
}

// NewRegistry 空注册表。
func NewRegistry() *Registry {
	return &Registry{dims: make(map[string]Dimension)}
}

// Register 注册/覆盖一个维度。非法定义（min>=max 或 baseline 越界）报 ErrBadDimension。
func (r *Registry) Register(d Dimension) error {
	if !d.valid() {
		return ErrBadDimension
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.dims[d.Key]; !ok {
		r.order = append(r.order, d.Key)
	}
	r.dims[d.Key] = d
	return nil
}

// Unregister 注销一个维度（幂等）。
func (r *Registry) Unregister(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.dims[key]; !ok {
		return
	}
	delete(r.dims, key)
	for i, k := range r.order {
		if k == key {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
}

// Get 取维度定义。
func (r *Registry) Get(key string) (Dimension, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.dims[key]
	return d, ok
}

// All 按注册顺序返回全部维度。
func (r *Registry) All() []Dimension {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Dimension, 0, len(r.order))
	for _, k := range r.order {
		out = append(out, r.dims[k])
	}
	return out
}

// Keys 按注册顺序返回维度 key。
func (r *Registry) Keys() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]string(nil), r.order...)
}

// 默认五维 key。
const (
	KeyValence   = "valence"   // 效价 -1..1
	KeyArousal   = "arousal"   // 唤醒 0..1
	KeyIntimacy  = "intimacy"  // 亲密 0..1
	KeyFatigue   = "fatigue"   // 疲劳 0..1
	KeyCuriosity = "curiosity" // 好奇 0..1
)

// DominanceDimension 预留的 PAD 第三维（掌控感）示例：默认不注册，随验证扩展时启用。
func DominanceDimension() Dimension {
	return Dimension{Key: "dominance", Label: "掌控感", Min: 0, Max: 1, Baseline: 0.4, DecayPerDay: 0.15}
}

// DefaultRegistry 五维默认注册表（PRD T-M4-01 起步档）。
// 衰减率设定贴合助手直觉：情绪(效价/唤醒)平复较快，疲劳恢复中，好奇回升中，亲密最慢（近似长期记忆）。
func DefaultRegistry() *Registry {
	r := NewRegistry()
	_ = r.Register(Dimension{Key: KeyValence, Label: "效价", Min: -1, Max: 1, Baseline: 0, DecayPerDay: 0.5})
	_ = r.Register(Dimension{Key: KeyArousal, Label: "唤醒", Min: 0, Max: 1, Baseline: 0.3, DecayPerDay: 0.8})
	_ = r.Register(Dimension{Key: KeyIntimacy, Label: "亲密", Min: 0, Max: 1, Baseline: 0.1, DecayPerDay: 0.02})
	_ = r.Register(Dimension{Key: KeyFatigue, Label: "疲劳", Min: 0, Max: 1, Baseline: 0, DecayPerDay: 0.3})
	_ = r.Register(Dimension{Key: KeyCuriosity, Label: "好奇", Min: 0, Max: 1, Baseline: 0.5, DecayPerDay: 0.2})
	return r
}

// —— 状态机引擎 ——

// Snapshot 持久化快照（状态 + 采样时刻）。
type Snapshot struct {
	State       map[string]float64 `json:"state"`
	AtUnixMilli int64              `json:"at_unix_milli"`
}

// Engine 情绪状态机。并发安全。
type Engine struct {
	mu       sync.Mutex
	reg      *Registry
	clk      clock.Clock
	state    map[string]float64
	lastTick time.Time
}

// New 用注册表 + 时钟建引擎；初始各维 = 基线，lastTick=now。
func New(reg *Registry, clk clock.Clock) *Engine {
	if reg == nil {
		reg = DefaultRegistry()
	}
	if clk == nil {
		clk = clock.RealClock{}
	}
	e := &Engine{reg: reg, clk: clk, state: make(map[string]float64)}
	e.syncDims()
	e.lastTick = clk.Now()
	return e
}

// syncDims 对齐 state 与注册表：新增维度填基线，注销维度移除（调用方需持锁）。
func (e *Engine) syncDims() {
	keys := e.reg.Keys()
	live := make(map[string]bool, len(keys))
	for _, k := range keys {
		live[k] = true
		if _, ok := e.state[k]; !ok {
			if d, ok := e.reg.Get(k); ok {
				e.state[k] = d.Baseline
			}
		}
	}
	for k := range e.state {
		if !live[k] {
			delete(e.state, k)
		}
	}
}

// advanceLocked 从 lastTick 按经过时间衰减到 now（调用方需持锁）。
func (e *Engine) advanceLocked(now time.Time) {
	e.syncDims()
	dtDays := now.Sub(e.lastTick).Hours() / 24.0
	if dtDays <= 0 {
		e.lastTick = now
		return
	}
	for _, d := range e.reg.All() {
		if v, ok := e.state[d.Key]; ok {
			e.state[d.Key] = d.decayToward(v, dtDays)
		}
	}
	e.lastTick = now
}

// Tick 后台定时器推演：按经过时间衰减到当前时刻。
func (e *Engine) Tick() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.advanceLocked(e.clk.Now())
}

// Interact 对话触发推演：先衰减到 now，再对给定维度叠加增量并夹范围（未知维度忽略）。
func (e *Engine) Interact(deltas map[string]float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.advanceLocked(e.clk.Now())
	for k, dv := range deltas {
		d, ok := e.reg.Get(k)
		if !ok {
			continue
		}
		e.state[k] = d.clamp(e.state[k] + dv)
	}
}

// Value 取某维度当前值。
func (e *Engine) Value(key string) (float64, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	v, ok := e.state[key]
	return v, ok
}

// State 返回当前全部维度状态副本（先衰减到 now）。
func (e *Engine) State() map[string]float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.advanceLocked(e.clk.Now())
	out := make(map[string]float64, len(e.state))
	for k, v := range e.state {
		out[k] = v
	}
	return out
}

// Snapshot 生成持久化快照（含采样时刻，供重开时从正确起点衰减）。
func (e *Engine) Snapshot() Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.advanceLocked(e.clk.Now())
	st := make(map[string]float64, len(e.state))
	for k, v := range e.state {
		st[k] = v
	}
	return Snapshot{State: st, AtUnixMilli: e.lastTick.UnixMilli()}
}

// Restore 从快照恢复（未知维度忽略；越界按维度夹取；lastTick 恢复到快照时刻）。
func (e *Engine) Restore(s Snapshot) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.state = make(map[string]float64)
	for k, v := range s.State {
		if d, ok := e.reg.Get(k); ok {
			e.state[k] = d.clamp(v)
		}
	}
	e.syncDims()
	if s.AtUnixMilli > 0 {
		e.lastTick = time.UnixMilli(s.AtUnixMilli)
	} else {
		e.lastTick = e.clk.Now()
	}
}

// SortedState 返回按注册顺序排列的 (key,value) 切片（便于展示/耦合）。
func (e *Engine) SortedState() []struct {
	Key   string
	Value float64
} {
	st := e.State()
	keys := e.reg.Keys()
	out := make([]struct {
		Key   string
		Value float64
	}, 0, len(keys))
	for _, k := range keys {
		if v, ok := st[k]; ok {
			out = append(out, struct {
				Key   string
				Value float64
			}{k, v})
		}
	}
	return out
}
