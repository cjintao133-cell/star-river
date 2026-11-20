package proactive

import (
	"sort"
	"sync"
	"time"

	"star-river/kernel/core/clock"
)

// Emitter 主动提醒出口（应用层把它接到通知/bus/UI；本包不依赖 bus，保持纯内核可测）。
type Emitter interface {
	Emit(Nudge)
}

// EmitterFunc 函数适配器。
type EmitterFunc func(Nudge)

// Emit 实现 Emitter。
func (f EmitterFunc) Emit(n Nudge) { f(n) }

// RateLimit 频率限制（滚动窗口 + 每日上限）。
type RateLimit struct {
	MaxPerWindow int           // 滚动 Window 内最大发出数；<=0 不限
	Window       time.Duration // 滚动窗口长度（<=0 默认 1h）
	DailyMax     int           // 自然日内最大发出数；<=0 不限
}

// Config 调度器配置。Enabled 零值为 false —— 主动能力出厂默认关闭（可撤回权限）。
type Config struct {
	Enabled         bool          // 总开关（默认 false=关闭）
	DefaultCooldown time.Duration // 每候选默认冷却（<=0 默认 20h）
	Rate            RateLimit
	Quiet           QuietHours
}

// DefaultConfig 一套合理默认（启用态）：3 次/小时、8 次/天、每候选 20h 冷却、23:00–07:00 静默。
// 宿主是否启用由应用层决定；组件本身仍以 Enabled 字段为准。
func DefaultConfig() Config {
	return Config{
		Enabled:         true,
		DefaultCooldown: 20 * time.Hour,
		Rate:            RateLimit{MaxPerWindow: 3, Window: time.Hour, DailyMax: 8},
		Quiet:           QuietHours{Enabled: true, StartMinutes: 23 * 60, EndMinutes: 7 * 60},
	}
}

// withDefaults 填充数值默认，但保留 Enabled（不擅自启用）。
func withDefaults(c Config) Config {
	if c.DefaultCooldown <= 0 {
		c.DefaultCooldown = 20 * time.Hour
	}
	if c.Rate.Window <= 0 {
		c.Rate.Window = time.Hour
	}
	return c
}

// Snapshot 调度器可持久化状态（去重冷却 + 限频历史 + 开关），供重启恢复。
type Snapshot struct {
	LastFired   map[string]int64 `json:"last_fired"` // key -> UnixMilli
	Fired       []int64          `json:"fired"`      // 发出时刻（UnixMilli）
	Enabled     bool             `json:"enabled"`
	Sources     map[string]bool  `json:"sources"` // SourceKind -> 开关
	AtUnixMilli int64            `json:"at_unix_milli"`
}

// Scheduler 主动调度器。并发安全（内部互斥）。
type Scheduler struct {
	mu        sync.Mutex
	clk       clock.Clock
	cfg       Config
	sources   []Source
	sourceOn  map[SourceKind]bool
	emit      Emitter
	lastFired map[string]int64 // key -> 上次发出 UnixMilli
	fired     []int64          // 全部发出时刻（UnixMilli），用于限频
}

// New 建调度器。数值零值走默认；Enabled 保持原样（默认关闭）。
// sources 可变参，每个默认开启；可后经 SetSourceEnabled 关闭。
func New(cfg Config, clk clock.Clock, sources ...Source) *Scheduler {
	cfg = withDefaults(cfg)
	if clk == nil {
		clk = clock.RealClock{}
	}
	s := &Scheduler{
		clk:       clk,
		cfg:       cfg,
		sources:   sources,
		sourceOn:  make(map[SourceKind]bool, len(sources)),
		lastFired: make(map[string]int64),
	}
	for _, src := range sources {
		s.sourceOn[src.Kind()] = true
	}
	return s
}

// SetEmitter 设置出口（可为 nil）。
func (s *Scheduler) SetEmitter(e Emitter) {
	s.mu.Lock()
	s.emit = e
	s.mu.Unlock()
}

// SetEnabled 总开关（置 false 立即停止一切发出；已记冷却/限频不受影响）。
func (s *Scheduler) SetEnabled(v bool) {
	s.mu.Lock()
	s.cfg.Enabled = v
	s.mu.Unlock()
}

// Enabled 读总开关。
func (s *Scheduler) Enabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.Enabled
}

// SetSourceEnabled 按源开关（场景级可撤回）。
func (s *Scheduler) SetSourceEnabled(k SourceKind, v bool) {
	s.mu.Lock()
	s.sourceOn[k] = v
	s.mu.Unlock()
}

// SourceEnabled 读某源开关。
func (s *Scheduler) SourceEnabled(k SourceKind) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sourceOn[k]
}

// Tick 以当前时钟评估一轮：收集候选 → 同轮去重 → 优先级排序 → 逐条过闸门 → 发出。
// reviews/emotion 为应用层喂入的只读输入。返回本轮真正发出的 Nudge（同时经 Emitter 外发）。
func (s *Scheduler) Tick(reviews []ReviewItem, emotion EmotionState) []Nudge {
	s.mu.Lock()
	out := s.tickLocked(reviews, emotion)
	emit := s.emit
	s.mu.Unlock()
	// 出锁后再外发，避免 Emitter 回调重入调度器导致死锁。
	if emit != nil {
		for _, n := range out {
			emit.Emit(n)
		}
	}
	return out
}

// tickLocked 在持锁下执行一轮评估。
func (s *Scheduler) tickLocked(reviews []ReviewItem, emotion EmotionState) []Nudge {
	if !s.cfg.Enabled {
		return nil
	}
	now := s.clk.Now()
	ctx := Context{Now: now, Reviews: reviews, Emotion: emotion}

	cands := s.collect(ctx)
	if len(cands) == 0 {
		return nil
	}
	cands = dedupeWithinTick(cands)
	// 优先级降序（稳定：同优先级保留源声明顺序）——限频额度优先给高优先级。
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].Priority > cands[j].Priority })

	s.prune(now)
	nowMs := now.UnixMilli()
	quiet := s.cfg.Quiet.contains(now)

	var out []Nudge
	for _, c := range cands {
		// 闸门 1：静默时段，非紧急延后（不记冷却、不占额度，静默后可再评估）。
		if quiet && c.Priority < PriorityUrgent {
			continue
		}
		// 闸门 2：按 Key 冷却去重。
		if !s.cooledDown(c, nowMs) {
			continue
		}
		// 闸门 3：频率限制（滚动窗口 + 每日上限）。
		if !s.allow(now) {
			continue
		}
		// 通过：记录并发出。
		s.lastFired[c.Key] = nowMs
		s.fired = append(s.fired, nowMs)
		out = append(out, Nudge{
			Source:      c.Source,
			Key:         c.Key,
			Priority:    c.Priority,
			Title:       c.Title,
			Body:        c.Body,
			AtUnixMilli: nowMs,
		})
	}
	return out
}

// collect 汇总各启用源的候选，丢弃空 Key。
func (s *Scheduler) collect(ctx Context) []Candidate {
	var out []Candidate
	for _, src := range s.sources {
		if !s.sourceOn[src.Kind()] {
			continue
		}
		for _, c := range src.Candidates(ctx) {
			if c.Key == "" {
				continue
			}
			out = append(out, c)
		}
	}
	return out
}

// cooledDown 报告候选是否已过冷却（可发出）。
func (s *Scheduler) cooledDown(c Candidate, nowMs int64) bool {
	last, ok := s.lastFired[c.Key]
	if !ok {
		return true
	}
	cd := c.Cooldown
	if cd <= 0 {
		cd = s.cfg.DefaultCooldown
	}
	return nowMs-last >= int64(cd/time.Millisecond)
}

// allow 报告当前时刻是否仍有限频额度。
func (s *Scheduler) allow(now time.Time) bool {
	nowMs := now.UnixMilli()
	if s.cfg.Rate.MaxPerWindow > 0 {
		winMs := int64(s.cfg.Rate.Window / time.Millisecond)
		cnt := 0
		for _, t := range s.fired {
			if nowMs-t < winMs {
				cnt++
			}
		}
		if cnt >= s.cfg.Rate.MaxPerWindow {
			return false
		}
	}
	if s.cfg.Rate.DailyMax > 0 {
		if s.countDay(now) >= s.cfg.Rate.DailyMax {
			return false
		}
	}
	return true
}

// countDay 统计与 now 同一自然日的发出数。
func (s *Scheduler) countDay(now time.Time) int {
	y, m, d := now.Date()
	cnt := 0
	for _, t := range s.fired {
		tt := time.UnixMilli(t).In(now.Location())
		yy, mm, dd := tt.Date()
		if yy == y && mm == m && dd == d {
			cnt++
		}
	}
	return cnt
}

// prune 丢弃超出限频所需历史窗口的发出记录（窗口 = max(Rate.Window, 24h 若设每日上限)）。
func (s *Scheduler) prune(now time.Time) {
	if len(s.fired) == 0 {
		return
	}
	horizon := s.cfg.Rate.Window
	if s.cfg.Rate.DailyMax > 0 && horizon < 24*time.Hour {
		horizon = 24 * time.Hour
	}
	if horizon <= 0 {
		horizon = time.Hour
	}
	cut := now.UnixMilli() - int64(horizon/time.Millisecond)
	keep := s.fired[:0]
	for _, t := range s.fired {
		if t >= cut {
			keep = append(keep, t)
		}
	}
	s.fired = keep
}

// Snapshot 生成可持久化状态。
func (s *Scheduler) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	lf := make(map[string]int64, len(s.lastFired))
	for k, v := range s.lastFired {
		lf[k] = v
	}
	srcs := make(map[string]bool, len(s.sourceOn))
	for k, v := range s.sourceOn {
		srcs[string(k)] = v
	}
	return Snapshot{
		LastFired:   lf,
		Fired:       append([]int64(nil), s.fired...),
		Enabled:     s.cfg.Enabled,
		Sources:     srcs,
		AtUnixMilli: s.clk.Now().UnixMilli(),
	}
}

// Restore 从快照恢复（未知源开关忽略；空快照恢复为空历史；Enabled 依快照）。
func (s *Scheduler) Restore(sn Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastFired = make(map[string]int64, len(sn.LastFired))
	for k, v := range sn.LastFired {
		s.lastFired[k] = v
	}
	s.fired = append([]int64(nil), sn.Fired...)
	s.cfg.Enabled = sn.Enabled
	for k, v := range sn.Sources {
		k2 := SourceKind(k)
		// 仅恢复已知源种类；未知忽略。
		switch k2 {
		case SourceMemoryReview, SourceSchedule, SourceEmotion:
			s.sourceOn[k2] = v
		}
	}
}

// dedupeWithinTick 同一轮内按 Key 去重：保留优先级最高者（同级保留首次出现）。
func dedupeWithinTick(cands []Candidate) []Candidate {
	idx := make(map[string]int, len(cands))
	out := make([]Candidate, 0, len(cands))
	for _, c := range cands {
		if i, ok := idx[c.Key]; ok {
			if c.Priority > out[i].Priority {
				out[i] = c
			}
			continue
		}
		idx[c.Key] = len(out)
		out = append(out, c)
	}
	return out
}
