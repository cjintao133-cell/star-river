// Package policy 是"动作总管"：对智能体将要执行的每个动作做准入判定。
//
// 三类真正危险的动作默认拦截或要求确认（其余放行）：
//   - 花钱（spend）：受总预算约束，超预算直接拒绝；未授权自动消费则要求确认。
//   - 对外发送（send/external）：默认要求人工确认。
//   - 危险/不可逆工具（dangerous）：默认要求人工确认。
//
// 本包零内部依赖（仅标准库），便于任何入口层复用与单测。
package policy

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
)

// ActionKind 动作大类。
type ActionKind string

const (
	KindSpend    ActionKind = "spend"
	KindSend     ActionKind = "send"
	KindExternal ActionKind = "external"
	KindTool     ActionKind = "tool"
	KindWrite    ActionKind = "write"
)

// Action 一个待判定动作。
type Action struct {
	Kind       ActionKind `json:"kind"`
	Name       string     `json:"name"`
	Amount     float64    `json:"amount,omitempty"`   // 花钱金额（货币单位）
	External   bool       `json:"external,omitempty"` // 是否触达第三方/网络出口
	Reversible bool       `json:"reversible"`         // 是否可逆（危险/删除类应设 false）
}

// Decision 判定结果。
type Decision string

const (
	Allow          Decision = "allow"
	Deny           Decision = "deny"
	RequireConfirm Decision = "require_confirm"
	OverBudget     Decision = "over_budget"
)

// Config 策略配置。零值即"保守默认"：花钱/对外/危险都要确认，普通工具放行。
type Config struct {
	TotalBudget      float64  `json:"total_budget"`       // <=0 表示不限
	AutoApproveSpend bool     `json:"auto_approve_spend"` // true 才免确认（仍受预算约束）
	ToolAllowlist    []string `json:"tool_allowlist"`     // 非空则只允许这些工具名
	ToolDenylist     []string `json:"tool_denylist"`      // 命中即拒绝
}

// Pending 一条待确认动作。
type Pending struct {
	ID     string `json:"id"`
	Action Action `json:"action"`
	Reason string `json:"reason"`
}

// Engine 策略引擎（并发安全）。
type Engine struct {
	mu      sync.Mutex
	cfg     Config
	spent   float64
	pending map[string]Pending
	order   []string // pending 插入顺序
}

// New 建引擎（深拷贝 allow/deny 列表，避免外部改写）。
func New(cfg Config) *Engine {
	return &Engine{cfg: cloneCfg(cfg), pending: make(map[string]Pending)}
}

// SetConfig 更新配置（保留已累计花费与待确认）。
func (e *Engine) SetConfig(cfg Config) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cfg = cloneCfg(cfg)
}

// Config 返回当前配置副本。
func (e *Engine) Config() Config {
	e.mu.Lock()
	defer e.mu.Unlock()
	return cloneCfg(e.cfg)
}

// Budget 返回 (已花费, 总预算)。
func (e *Engine) Budget() (float64, float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.spent, e.cfg.TotalBudget
}

// ResetSpent 清零已累计花费（便于按周期重置预算）。
func (e *Engine) ResetSpent() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spent = 0
}

// Check 判定一个动作。返回 (决策, 原因, 待确认 ID)。
// RequireConfirm 时会登记一条 pending 并返回其 ID；否则 ID 为空。
func (e *Engine) Check(a Action) (Decision, string, string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 黑名单优先。
	if inList(e.cfg.ToolDenylist, a.Name) {
		return Deny, "工具在黑名单: " + a.Name, ""
	}
	// 白名单（非空）外一律拒绝。
	if len(e.cfg.ToolAllowlist) > 0 && !inList(e.cfg.ToolAllowlist, a.Name) {
		return Deny, "工具不在白名单: " + a.Name, ""
	}

	// 花钱：先判预算，再判确认。
	if a.Kind == KindSpend {
		if e.cfg.TotalBudget > 0 && e.spent+a.Amount > e.cfg.TotalBudget {
			return OverBudget, "超出总预算", ""
		}
		if !e.cfg.AutoApproveSpend && a.Amount > 0 {
			return e.addPendingLocked(a, "花费需确认")
		}
		return Allow, "", ""
	}

	// 对外发送 / 外部触达 / 不可逆危险：默认要求确认。
	if a.Kind == KindSend || a.Kind == KindExternal || a.External || !a.Reversible {
		return e.addPendingLocked(a, "对外/不可逆动作需确认")
	}

	return Allow, "", ""
}

// Commit 动作执行成功后记账（仅 spend 影响已花费）。
func (e *Engine) Commit(a Action) {
	if a.Kind != KindSpend || a.Amount <= 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spent += a.Amount
}

// addPendingLocked 登记待确认并生成 ID（调用方持锁）。
func (e *Engine) addPendingLocked(a Action, reason string) (Decision, string, string) {
	id := newID()
	e.pending[id] = Pending{ID: id, Action: a, Reason: reason}
	e.order = append(e.order, id)
	return RequireConfirm, reason, id
}

// ListPending 返回待确认动作（插入顺序）。
func (e *Engine) ListPending() []Pending {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Pending, 0, len(e.order))
	for _, id := range e.order {
		if p, ok := e.pending[id]; ok {
			out = append(out, p)
		}
	}
	return out
}

// Resolve 处理待确认：approve=true 放行并移除，false 拒绝并移除。返回是否命中。
func (e *Engine) Resolve(id string, approve bool) (Pending, bool, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p, ok := e.pending[id]
	if !ok {
		return Pending{}, false, false
	}
	delete(e.pending, id)
	// 清理 order
	for i, x := range e.order {
		if x == id {
			e.order = append(e.order[:i], e.order[i+1:]...)
			break
		}
	}
	return p, true, approve
}

func cloneCfg(c Config) Config {
	c.ToolAllowlist = append([]string(nil), c.ToolAllowlist...)
	c.ToolDenylist = append([]string(nil), c.ToolDenylist...)
	return c
}

func inList(list []string, name string) bool {
	name = strings.TrimSpace(name)
	for _, x := range list {
		if strings.EqualFold(strings.TrimSpace(x), name) {
			return true
		}
	}
	return false
}

func newID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "pending-fallback"
	}
	return "ap-" + hex.EncodeToString(b[:])
}
