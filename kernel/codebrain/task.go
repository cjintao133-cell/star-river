// Package codebrain 是"任务/目标运行时"：把一次目标拆成可持久化、可断点续跑的多步任务。
//
// 设计：本包零内部依赖（仅标准库）。任务状态机 + 执行编排在此（纯逻辑、可注入 Plan/Step
// 执行器与 Store，便于离线确定性测试）；真正的 SQLite 持久化与模型调用由入口层实现并注入。
//
// 状态迁移：pending → planned → running →（done | failed | waiting_approval | paused）
// 每一步执行后落库（checkpoint），因此进程重启后可 Resume 从游标处继续。
package codebrain

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Status 任务/步骤状态。
type Status string

const (
	StatusPending         Status = "pending"
	StatusPlanned         Status = "planned"
	StatusRunning         Status = "running"
	StatusWaitingApproval Status = "waiting_approval"
	StatusPaused          Status = "paused"
	StatusDone            Status = "done"
	StatusFailed          Status = "failed"
)

// Step 一个计划步骤。
type Step struct {
	Index  int    `json:"index"`
	Title  string `json:"title"`
	Status Status `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Task 一个任务/目标。
type Task struct {
	ID          string   `json:"id"`
	SessionID   string   `json:"session_id,omitempty"`
	CharacterID string   `json:"character_id,omitempty"`
	Title       string   `json:"title"`
	Goal        string   `json:"goal"`
	Status      Status   `json:"status"`
	Plan        []Step   `json:"plan"`
	Cursor      int      `json:"cursor"`            // 下一个待执行步骤索引
	Results     []string `json:"results,omitempty"` // 每步执行结果
	Error       string   `json:"error,omitempty"`
	CreatedAt   int64    `json:"created_at"`
	UpdatedAt   int64    `json:"updated_at"`
}

// 哨兵错误。
var (
	ErrNotFound        = errors.New("codebrain: task not found")
	ErrWaitingApproval = errors.New("codebrain: step needs approval")
	ErrTerminal        = errors.New("codebrain: task already terminal")
)

// Store 任务持久化接口（由入口层实现）。
type Store interface {
	Save(t *Task) error
	Load(id string) (*Task, error)
	List(sessionID string, limit int) ([]*Task, error)
	Delete(id string) error
}

// PlanFunc 依据任务目标产出步骤标题。
type PlanFunc func(ctx context.Context, t *Task) ([]string, error)

// StepFunc 执行一个步骤，返回该步结果文本。
// 若返回 ErrWaitingApproval，任务转入 waiting_approval 并暂停。
type StepFunc func(ctx context.Context, t *Task, s Step) (string, error)

// Runner 任务编排器。
type Runner struct {
	store Store
	now   func() int64
	newID func() string
}

// Option 配置 Runner。
type Option func(*Runner)

// WithClock 注入时钟（测试）。
func WithClock(f func() int64) Option { return func(r *Runner) { r.now = f } }

// WithIDGen 注入 ID 生成（测试）。
func WithIDGen(f func() string) Option { return func(r *Runner) { r.newID = f } }

// NewRunner 建编排器。
func NewRunner(store Store, opts ...Option) *Runner {
	r := &Runner{store: store, now: func() int64 { return time.Now().UnixMilli() }, newID: defaultID}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Create 新建任务（pending）。
func (r *Runner) Create(t Task) (*Task, error) {
	if t.ID == "" {
		t.ID = r.newID()
	}
	if t.Status == "" {
		t.Status = StatusPending
	}
	t.Plan = nil
	t.Cursor = 0
	t.Results = nil
	t.CreatedAt = r.now()
	t.UpdatedAt = t.CreatedAt
	if err := r.store.Save(&t); err != nil {
		return nil, err
	}
	return &t, nil
}

// Plan 用 planFn 生成步骤，状态置 planned。
func (r *Runner) Plan(ctx context.Context, id string, planFn PlanFunc) (*Task, error) {
	t, err := r.store.Load(id)
	if err != nil {
		return nil, err
	}
	if isTerminal(t.Status) {
		return nil, ErrTerminal
	}
	titles, err := planFn(ctx, t)
	if err != nil {
		return nil, err
	}
	if len(titles) == 0 {
		titles = []string{t.Title}
		if t.Title == "" {
			titles = []string{"执行目标"}
		}
	}
	t.Plan = make([]Step, 0, len(titles))
	for i, title := range titles {
		t.Plan = append(t.Plan, Step{Index: i, Title: title, Status: StatusPending})
	}
	t.Cursor = 0
	t.Results = nil
	t.Status = StatusPlanned
	t.UpdatedAt = r.now()
	if err := r.store.Save(t); err != nil {
		return nil, err
	}
	return t, nil
}

// Step 执行下一个待执行步骤并落库（checkpoint）。
func (r *Runner) Step(ctx context.Context, id string, stepFn StepFunc) (*Task, error) {
	t, err := r.store.Load(id)
	if err != nil {
		return nil, err
	}
	if isTerminal(t.Status) {
		return nil, ErrTerminal
	}
	if len(t.Plan) == 0 {
		// 未规划则把目标作为单步。
		t.Plan = []Step{{Index: 0, Title: fallbackTitle(t), Status: StatusPending}}
		t.Cursor = 0
	}
	if t.Cursor >= len(t.Plan) {
		t.Status = StatusDone
		t.UpdatedAt = r.now()
		_ = r.store.Save(t)
		return t, nil
	}
	step := &t.Plan[t.Cursor]
	t.Status = StatusRunning
	step.Status = StatusRunning
	t.UpdatedAt = r.now()
	if err := r.store.Save(t); err != nil { // 标记 running 落库
		return nil, err
	}

	out, e := stepFn(ctx, t, *step)
	if errors.Is(e, ErrWaitingApproval) {
		t.Status = StatusWaitingApproval
		step.Status = StatusWaitingApproval
		step.Detail = out
		t.UpdatedAt = r.now()
		_ = r.store.Save(t)
		return t, nil
	}
	if e != nil {
		t.Status = StatusFailed
		step.Status = StatusFailed
		step.Detail = e.Error()
		t.Error = e.Error()
		t.UpdatedAt = r.now()
		_ = r.store.Save(t)
		return t, e
	}
	step.Status = StatusDone
	step.Detail = out
	t.Results = append(t.Results, out)
	t.Cursor++
	if t.Cursor >= len(t.Plan) {
		t.Status = StatusDone
	} else {
		t.Status = StatusRunning
	}
	t.UpdatedAt = r.now()
	if err := r.store.Save(t); err != nil {
		return nil, err
	}
	return t, nil
}

// Run 连续执行直到完成/暂停/等待审批/失败或达到 maxSteps（<=0 默认 50）。
func (r *Runner) Run(ctx context.Context, id string, stepFn StepFunc, maxSteps int) (*Task, error) {
	if maxSteps <= 0 {
		maxSteps = 50
	}
	var t *Task
	for i := 0; i < maxSteps; i++ {
		var err error
		t, err = r.Step(ctx, id, stepFn)
		if err != nil {
			return t, err
		}
		if t.Status != StatusRunning {
			return t, nil
		}
	}
	return t, nil
}

// Pause 暂停（running/planned → paused）。
func (r *Runner) Pause(id string) (*Task, error) {
	t, err := r.store.Load(id)
	if err != nil {
		return nil, err
	}
	if isTerminal(t.Status) {
		return nil, ErrTerminal
	}
	t.Status = StatusPaused
	t.UpdatedAt = r.now()
	if err := r.store.Save(t); err != nil {
		return nil, err
	}
	return t, nil
}

// Resume 恢复（paused/waiting_approval → running 前的状态，仅改状态，不改游标）。
func (r *Runner) Resume(id string) (*Task, error) {
	t, err := r.store.Load(id)
	if err != nil {
		return nil, err
	}
	if isTerminal(t.Status) {
		return nil, ErrTerminal
	}
	if t.Status == StatusWaitingApproval && t.Cursor < len(t.Plan) {
		t.Plan[t.Cursor].Status = StatusPending
	}
	t.Status = StatusRunning
	t.UpdatedAt = r.now()
	if err := r.store.Save(t); err != nil {
		return nil, err
	}
	return t, nil
}

// Cancel 取消（→ failed，error=canceled）。
func (r *Runner) Cancel(id string) (*Task, error) {
	t, err := r.store.Load(id)
	if err != nil {
		return nil, err
	}
	if isTerminal(t.Status) {
		return nil, ErrTerminal
	}
	t.Status = StatusFailed
	t.Error = "canceled"
	t.UpdatedAt = r.now()
	if err := r.store.Save(t); err != nil {
		return nil, err
	}
	return t, nil
}

// Get 取任务。
func (r *Runner) Get(id string) (*Task, error) { return r.store.Load(id) }

// List 列表。
func (r *Runner) List(sessionID string, limit int) ([]*Task, error) {
	return r.store.List(sessionID, limit)
}

// Delete 删除。
func (r *Runner) Delete(id string) error { return r.store.Delete(id) }

func isTerminal(s Status) bool { return s == StatusDone || s == StatusFailed }

func fallbackTitle(t *Task) string {
	if t.Title != "" {
		return t.Title
	}
	if t.Goal != "" {
		return t.Goal
	}
	return "执行目标"
}

func defaultID() string {
	return "task-" + time.Now().Format("20060102150405.000000000")
}

// ---- MemStore 内存实现（测试/演示） ----

// MemStore 内存任务仓储（并发安全）。
type MemStore struct {
	mu sync.Mutex
	m  map[string]*Task
}

// NewMemStore 建内存仓储。
func NewMemStore() *MemStore { return &MemStore{m: make(map[string]*Task)} }

func cloneTask(t *Task) *Task {
	cp := *t
	cp.Plan = append([]Step(nil), t.Plan...)
	cp.Results = append([]string(nil), t.Results...)
	return &cp
}

// Save 保存（深拷贝）。
func (s *MemStore) Save(t *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[t.ID] = cloneTask(t)
	return nil
}

// Load 读取（返回副本）。
func (s *MemStore) Load(id string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.m[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneTask(t), nil
}

// List 列表（按 updated 倒序）。
func (s *MemStore) List(sessionID string, limit int) ([]*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Task, 0, len(s.m))
	for _, t := range s.m {
		if sessionID == "" || t.SessionID == sessionID {
			out = append(out, cloneTask(t))
		}
	}
	// 简单按 updated 倒序
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].UpdatedAt > out[j-1].UpdatedAt; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Delete 删除。
func (s *MemStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, id)
	return nil
}
