// Package router 灾备路由（T-M1-07）。
//
// 职责：按顺序尝试主模型与备用模型，主失败/超时自动切备用；备用价格档默认不得高于主
// （可配置放宽）；切换事件经 bus 广播（UI 可感知）；成功结果携带实际应答模型标记。
//
// 协议无关：真正发请求由 Caller 接口注入（各适配器在集成层包装 openai/anthropic/gemini），
// 本包不 import 任何协议包。事件发布走 Publisher 接口，默认接 kernel/bus。
package router

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"star-river/kernel/core/types"
)

// PriceTier 价格档。数值越大越贵，用于"备用不得高于主"约束。
type PriceTier int

const (
	TierUnknown  PriceTier = 0 // 未标注（无法比较，不参与档位过滤）
	TierCheap    PriceTier = 1
	TierStandard PriceTier = 2
	TierPremium  PriceTier = 3
)

func (t PriceTier) String() string {
	switch t {
	case TierCheap:
		return "cheap"
	case TierStandard:
		return "standard"
	case TierPremium:
		return "premium"
	default:
		return "unknown"
	}
}

// Endpoint 一个候选端点（渠道）。Model 为发给该端点的模型 id。
type Endpoint struct {
	Name    string
	BaseURL string
	APIKey  string
	Model   string
	Tier    PriceTier
}

// Caller 对某端点发起一次流式对话，成功返回实际应答模型（端点回显）。
// emit 收归一化流事件；本包只透传，不解析。集成层用各协议适配器实现。
type Caller interface {
	Chat(ctx context.Context, ep Endpoint, emit func(types.StreamEvent)) (model string, err error)
}

// CallerFunc 把函数适配为 Caller。
type CallerFunc func(ctx context.Context, ep Endpoint, emit func(types.StreamEvent)) (string, error)

// Chat 实现 Caller。
func (f CallerFunc) Chat(ctx context.Context, ep Endpoint, emit func(types.StreamEvent)) (string, error) {
	return f(ctx, ep, emit)
}

// Publisher 事件发布抽象；kernel/bus.Bus 直接满足（Publish(types.Event) error）。
type Publisher interface {
	Publish(e types.Event) error
}

// Result 一次路由调用的结果。Model 为实际应答模型（切换后即备用模型，供消息标注）。
// Switched=true 表示发生过灾备切换；Switches 为切换次数（= 成功的端点索引）。
type Result struct {
	Model    string
	Endpoint string
	Tier     PriceTier
	Switches int
	Switched bool
}

// StatsProvider 提供某模型的失败率（供灾备排序/标灰）。T-M3-07 由 store.StatsRepo 适配。
// known=false 表示无统计（不参与标灰，按原序）。
type StatsProvider interface {
	FailureRate(modelID string) (rate float64, known bool)
}

// StatsProviderFunc 函数适配。
type StatsProviderFunc func(modelID string) (float64, bool)

// FailureRate 实现 StatsProvider。
func (f StatsProviderFunc) FailureRate(modelID string) (float64, bool) { return f(modelID) }

// Router 灾备路由器。并发安全：字段构造后只读（version 原子）。
type Router struct {
	endpoints   []Endpoint
	caller      Caller
	pub         Publisher
	topic       string
	allowHigher bool
	shouldFail  func(error) bool
	stats       StatsProvider
	grayRate    float64 // >0 时启用标灰：失败率超此阈值则该备用跳过
	now         func() time.Time
	version     atomic.Int64
}

// Option 配置项。
type Option func(*Router)

// WithPublisher 挂事件发布器（默认 nil=不广播）。
func WithPublisher(p Publisher) Option { return func(r *Router) { r.pub = p } }

// WithTopic 设置切换事件主题（默认 "adapter.route.failover"）。
func WithTopic(t string) Option { return func(r *Router) { r.topic = t } }

// WithAllowHigherTier 放宽"备用不高于主"约束。
func WithAllowHigherTier(b bool) Option { return func(r *Router) { r.allowHigher = b } }

// WithFailoverPredicate 自定义"该错误是否值得切备用"。默认恒 true。
// （context.Canceled 永远不切，见 Chat，调用方取消属主动行为。）
func WithFailoverPredicate(f func(error) bool) Option { return func(r *Router) { r.shouldFail = f } }

// WithClock 注入时钟（测试用），默认 time.Now。
func WithClock(f func() time.Time) Option { return func(r *Router) { r.now = f } }

// WithStats 挂模型统计：备用按失败率升序排序（健康优先），失败率 > grayRate 的备用标灰跳过。
// grayRate<=0 时不标灰，仅排序。主端点始终保留（总会先试主）。
func WithStats(p StatsProvider, grayRate float64) Option {
	return func(r *Router) { r.stats = p; r.grayRate = grayRate }
}

// New 建路由器。endpoints[0] 为主，其余为按序备用；至少一个，且 caller 非空。
func New(endpoints []Endpoint, caller Caller, opts ...Option) (*Router, error) {
	if len(endpoints) == 0 {
		return nil, errors.New("router: at least one endpoint required")
	}
	if caller == nil {
		return nil, errors.New("router: caller required")
	}
	r := &Router{
		endpoints:  append([]Endpoint(nil), endpoints...),
		caller:     caller,
		topic:      "adapter.route.failover",
		shouldFail: func(error) bool { return true },
		now:        time.Now,
	}
	for _, o := range opts {
		o(r)
	}
	return r, nil
}

// attempts 返回本次调用要尝试的端点序列：主 + 通过档位过滤的备用。
// 备用档高于主且未放宽 → 跳过（TierUnknown 不参与比较，一律放行）。
func (r *Router) attempts() []Endpoint {
	primary := r.endpoints[0]
	// 备用：先按档位过滤。
	var fbs []Endpoint
	for _, fb := range r.endpoints[1:] {
		if !r.allowHigher && primary.Tier != TierUnknown && fb.Tier != TierUnknown && fb.Tier > primary.Tier {
			continue
		}
		fbs = append(fbs, fb)
	}
	// T-M3-07：按统计做标灰 + 排序（仅当挂了 stats）。
	if r.stats != nil && len(fbs) > 0 {
		type scored struct {
			ep   Endpoint
			rate float64
			have bool
		}
		ss := make([]scored, 0, len(fbs))
		for _, fb := range fbs {
			rate, have := r.stats.FailureRate(fb.Model)
			// 标灰：有统计且失败率超阈值 → 跳过该备用。
			if have && r.grayRate > 0 && rate > r.grayRate {
				continue
			}
			ss = append(ss, scored{ep: fb, rate: rate, have: have})
		}
		// 稳定排序：有统计的按失败率升序在前；无统计的保持原序排后。
		sort.SliceStable(ss, func(i, j int) bool {
			if ss[i].have != ss[j].have {
				return ss[i].have // 有统计优先
			}
			if !ss[i].have {
				return false
			}
			return ss[i].rate < ss[j].rate
		})
		fbs = fbs[:0]
		for _, s := range ss {
			fbs = append(fbs, s.ep)
		}
	}
	out := make([]Endpoint, 0, 1+len(fbs))
	out = append(out, primary)
	out = append(out, fbs...)
	return out
}

// contentEvent 报告是否为会产生可见输出的流事件（用于"已吐字则不切"保护）。
func contentEvent(e types.StreamEvent) bool {
	switch e.Kind {
	case types.StreamDelta, types.StreamReasoning, types.StreamToolCall:
		return true
	}
	return false
}

// Chat 依次尝试主与备用端点。规则：
//   - 成功：返回 Result（Model=实际应答模型，Switches=所用端点索引，Switched=索引>0）。
//   - 失败且尚未产生可见输出、且该错误值得切换（默认是，context.Canceled 除外）→ 切下一个，
//     并在切换时广播门铃事件。
//   - **已产生可见输出后失败 → 不切换**（避免向 UI 重复吐字，与适配器"响应体开始前才重试"同源）。
//   - 全部失败 → 返回最后一次错误。
func (r *Router) Chat(ctx context.Context, emit func(types.StreamEvent)) (*Result, error) {
	plan := r.attempts()
	emitted := false
	wrapped := func(e types.StreamEvent) {
		if contentEvent(e) {
			emitted = true
		}
		if emit != nil {
			emit(e)
		}
	}
	var lastErr error
	for i, ep := range plan {
		model, err := r.caller.Chat(ctx, ep, wrapped)
		if err == nil {
			return &Result{Model: model, Endpoint: ep.Name, Tier: ep.Tier, Switches: i, Switched: i > 0}, nil
		}
		lastErr = err
		// 用户取消：主动行为，不切、不广播。
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		// 已吐字后失败：不切，避免重复输出。
		if emitted {
			return nil, err
		}
		// 不满足切换条件或没有下一个：停。
		if !r.shouldFail(err) || i == len(plan)-1 {
			return nil, err
		}
		r.publishSwitch(ep, plan[i+1])
	}
	return nil, lastErr
}

// publishSwitch 广播一次切换门铃。发布失败不影响路由结果（门铃可容忍丢铃）。
func (r *Router) publishSwitch(from, to Endpoint) {
	if r.pub == nil {
		return
	}
	v := r.version.Add(1)
	_ = r.pub.Publish(types.Event{
		Topic:     r.topic,
		Version:   v,
		Source:    "adapter.router",
		Timestamp: r.now().UnixMilli(),
	})
}

// Plan 暴露当前路由计划（主+过滤后备用），供 UI/排障展示与测试断言。
func (r *Router) Plan() []PlanEntry {
	atts := r.attempts()
	out := make([]PlanEntry, 0, len(atts))
	for _, ep := range atts {
		out = append(out, PlanEntry{Name: ep.Name, BaseURL: ep.BaseURL, Model: ep.Model, Tier: ep.Tier})
	}
	return out
}

// PlanEntry 计划展示条目（无密钥，供 UI/排障）。绝不含 APIKey（红线3）。
type PlanEntry struct {
	Name    string
	BaseURL string
	Model   string
	Tier    PriceTier
}

// Describe 单端点简述（排障用，不含 key）。
func (ep Endpoint) Describe() string {
	return fmt.Sprintf("%s(%s/%s)", ep.Name, ep.Model, ep.Tier)
}
