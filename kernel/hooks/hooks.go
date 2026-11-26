// Package hooks 提供生命周期钩子：在关键节点（聊天前后、工具前后、出错、记忆写入、压缩前）
// 插入可注册的回调，支持优先级排序、结果收集、panic 隔离。零内部依赖（仅标准库）。
//
// 用途：审计、计数、注入校验、自定义拦截、埋点等横切逻辑，而不侵入主流程代码。
package hooks

import (
	"fmt"
	"sort"
	"sync"
)

// 预定义事件名（也可用任意自定义字符串）。
const (
	BeforeChat    = "before_chat"
	AfterChat     = "after_chat"
	BeforeTool    = "before_tool"
	AfterTool     = "after_tool"
	OnError       = "on_error"
	OnMemoryWrite = "on_memory_write"
	PreCompact    = "pre_compact"
	OnTaskStep    = "on_task_step"
)

// Handler 钩子回调：payload 为事件数据；返回 (结果, 是否拦截)。
// 拦截（abort=true）时调用方应中止后续动作（如拒绝执行工具）。
type Handler func(payload map[string]any) (result any, abort bool)

// registered 一个注册项。
type registered struct {
	name     string
	priority int
	fn       Handler
	order    int64
}

// Registry 钩子注册表（并发安全）。
type Registry struct {
	mu    sync.Mutex
	byEvt map[string][]registered
	seq   int64
}

// NewRegistry 建注册表。
func NewRegistry() *Registry { return &Registry{byEvt: map[string][]registered{}} }

// Register 注册钩子，返回取消注册函数。priority 越小越先执行。
func (r *Registry) Register(event string, priority int, fn Handler) (unregister func()) {
	r.mu.Lock()
	r.seq++
	item := registered{name: event, priority: priority, fn: fn, order: r.seq}
	r.byEvt[event] = append(r.byEvt[event], item)
	r.mu.Unlock()
	return func() { r.remove(event, item.order) }
}

func (r *Registry) remove(event string, order int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.byEvt[event]
	out := list[:0]
	for _, it := range list {
		if it.order != order {
			out = append(out, it)
		}
	}
	r.byEvt[event] = out
}

// Result 单个钩子的执行结果。
type Result struct {
	Event  string `json:"event"`
	Result any    `json:"result,omitempty"`
	Abort  bool   `json:"abort"`
	Panic  string `json:"panic,omitempty"`
}

// Fire 触发事件：按优先级顺序执行全部钩子；遇到 abort 提前停止。
// 返回各钩子结果与最终是否被拦截。panic 被隔离记录。
func (r *Registry) Fire(event string, payload map[string]any) ([]Result, bool) {
	r.mu.Lock()
	list := append([]registered(nil), r.byEvt[event]...)
	r.mu.Unlock()
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].priority != list[j].priority {
			return list[i].priority < list[j].priority
		}
		return list[i].order < list[j].order
	})
	var out []Result
	aborted := false
	for _, it := range list {
		res := r.fireOne(event, it.fn, payload)
		out = append(out, res)
		if res.Abort {
			aborted = true
			break
		}
	}
	return out, aborted
}

func (r *Registry) fireOne(event string, fn Handler, payload map[string]any) (res Result) {
	res.Event = event
	defer func() {
		if p := recover(); p != nil {
			res.Panic = fmt.Sprint(p)
		}
	}()
	val, abort := fn(payload)
	res.Result = val
	res.Abort = abort
	return res
}

// Has 是否有该事件的钩子。
func (r *Registry) Has(event string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.byEvt[event]) > 0
}

// Count 该事件钩子数量。
func (r *Registry) Count(event string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.byEvt[event])
}

// Clear 清空全部钩子。
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byEvt = map[string][]registered{}
}
