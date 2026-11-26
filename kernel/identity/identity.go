// Package identity 维护长时任务的"身份与轨迹"（identity & trajectory）：
// 跨会话稳定的 agent 身份，以及一条可审计的进程轨迹（做了什么、到哪了、关键决策）。
//
// 纯逻辑 + 可注入 Store（内存实现便于测试）；落地持久化由入口层实现。
package identity

import (
	"errors"
	"sort"
	"sync"
	"time"
)

// Identity 稳定的 agent 身份。
type Identity struct {
	AgentID   string            `json:"agent_id"`
	Name      string            `json:"name"`
	Role      string            `json:"role"`
	CreatedAt int64             `json:"created_at"`
	Meta      map[string]string `json:"meta,omitempty"`
}

// Event 一条轨迹事件。
type Event struct {
	At      int64  `json:"at"`   // UnixMilli
	Kind    string `json:"kind"` // task_created|step_done|decision|error|note...
	Summary string `json:"summary"`
	Ref     string `json:"ref,omitempty"` // 关联 id（任务/会话等）
}

// Store 持久化接口（入口层实现）。
type Store interface {
	LoadIdentity() (Identity, bool, error)
	SaveIdentity(Identity) error
	AppendEvent(Event) error
	ListEvents(limit int) ([]Event, error)
}

var (
	ErrNoIdentity = errors.New("identity: not set")
)

// Tracker 轨迹追踪器。
type Tracker struct {
	store Store
	now   func() int64
}

// Option 配置。
type Option func(*Tracker)

// WithClock 注入时钟。
func WithClock(f func() int64) Option { return func(t *Tracker) { t.now = f } }

// New 建追踪器。
func New(store Store, opts ...Option) *Tracker {
	t := &Tracker{store: store, now: func() int64 { return time.Now().UnixMilli() }}
	for _, o := range opts {
		o(t)
	}
	return t
}

// Ensure 若无身份则用默认值创建；返回当前身份。
func (t *Tracker) Ensure(defaultName, defaultRole string) (Identity, error) {
	if id, ok, err := t.store.LoadIdentity(); err != nil {
		return Identity{}, err
	} else if ok {
		return id, nil
	}
	id := Identity{AgentID: "agent-" + itoa(t.now()), Name: defaultName, Role: defaultRole, CreatedAt: t.now(), Meta: map[string]string{}}
	if err := t.store.SaveIdentity(id); err != nil {
		return Identity{}, err
	}
	return id, nil
}

// Get 取身份。
func (t *Tracker) Get() (Identity, error) {
	id, ok, err := t.store.LoadIdentity()
	if err != nil {
		return Identity{}, err
	}
	if !ok {
		return Identity{}, ErrNoIdentity
	}
	return id, nil
}

// Set 覆盖身份（保留 CreatedAt 若为 0 则填当前）。
func (t *Tracker) Set(id Identity) error {
	if id.CreatedAt == 0 {
		id.CreatedAt = t.now()
	}
	if id.AgentID == "" {
		id.AgentID = "agent-" + itoa(t.now())
	}
	return t.store.SaveIdentity(id)
}

// Record 记录一条轨迹事件（At 为 0 时填当前）。
func (t *Tracker) Record(e Event) error {
	if e.At == 0 {
		e.At = t.now()
	}
	return t.store.AppendEvent(e)
}

// Recent 最近 n 条轨迹（时间正序返回）。
func (t *Tracker) Recent(n int) ([]Event, error) {
	evs, err := t.store.ListEvents(n)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].At < evs[j].At })
	return evs, nil
}

// Summarize 把最近 n 条轨迹压成一段"当前进展"说明（供注入 system，跨会话保持连续性）。
func (t *Tracker) Summarize(n int) (string, error) {
	evs, err := t.Recent(n)
	if err != nil {
		return "", err
	}
	if len(evs) == 0 {
		return "", nil
	}
	var b []byte
	for _, e := range evs {
		b = append(b, []byte("- ["+e.Kind+"] "+e.Summary+"\n")...)
	}
	return string(b), nil
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// ---- MemStore 内存实现 ----

// MemStore 内存身份/轨迹。
type MemStore struct {
	mu  sync.Mutex
	id  Identity
	has bool
	evs []Event
}

// NewMemStore 建内存仓储。
func NewMemStore() *MemStore { return &MemStore{} }

// LoadIdentity 实现 Store。
func (s *MemStore) LoadIdentity() (Identity, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.id, s.has, nil
}

// SaveIdentity 实现 Store。
func (s *MemStore) SaveIdentity(id Identity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.id = id
	s.has = true
	return nil
}

// AppendEvent 实现 Store。
func (s *MemStore) AppendEvent(e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evs = append(s.evs, e)
	return nil
}

// ListEvents 实现 Store（返回最近 limit 条，倒序取再交给上层排正）。
func (s *MemStore) ListEvents(limit int) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > len(s.evs) {
		limit = len(s.evs)
	}
	out := make([]Event, 0, limit)
	out = append(out, s.evs[len(s.evs)-limit:]...)
	return out, nil
}
