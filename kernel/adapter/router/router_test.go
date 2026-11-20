package router

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"star-river/kernel/bus"
	"star-river/kernel/core/types"
)

// fakeCaller 按端点名脚本化行为，记录调用顺序（并发安全）。
type fakeCaller struct {
	mu     sync.Mutex
	order  []string
	behave map[string]func(emit func(types.StreamEvent)) (string, error)
}

func newFake(behave map[string]func(emit func(types.StreamEvent)) (string, error)) *fakeCaller {
	return &fakeCaller{behave: behave}
}

func (f *fakeCaller) Chat(ctx context.Context, ep Endpoint, emit func(types.StreamEvent)) (string, error) {
	f.mu.Lock()
	f.order = append(f.order, ep.Name)
	f.mu.Unlock()
	if b, ok := f.behave[ep.Name]; ok {
		return b(emit)
	}
	return "", errors.New("no script for " + ep.Name)
}

func (f *fakeCaller) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.order...)
}

func drain(t *testing.T, sub *bus.Subscription) []types.Event {
	t.Helper()
	var evs []types.Event
	for {
		select {
		case e := <-sub.Events():
			evs = append(evs, e)
		default:
			return evs
		}
	}
}

func ep(name, model string, tier PriceTier) Endpoint {
	return Endpoint{Name: name, BaseURL: "https://" + name + "/v1", APIKey: "k", Model: model, Tier: tier}
}

func TestPrimarySuccess(t *testing.T) {
	fc := newFake(map[string]func(func(types.StreamEvent)) (string, error){
		"p": func(emit func(types.StreamEvent)) (string, error) {
			emit(types.StreamEvent{Kind: types.StreamDelta, Text: "hi"})
			return "auto-req", nil
		},
	})
	r, _ := New([]Endpoint{ep("p", "m1", TierStandard), ep("f", "m2", TierStandard)}, fc)
	res, err := r.Chat(context.Background(), func(types.StreamEvent) {})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if res.Switched || res.Switches != 0 || res.Endpoint != "p" || res.Model != "auto-req" {
		t.Errorf("result = %+v", res)
	}
	if len(fc.calls()) != 1 {
		t.Errorf("calls = %v", fc.calls())
	}
}

func TestFailoverOnPrimaryError(t *testing.T) {
	b := bus.New(64)
	sub, _ := b.Subscribe()
	defer b.Close()

	fc := newFake(map[string]func(func(types.StreamEvent)) (string, error){
		"p": func(emit func(types.StreamEvent)) (string, error) { return "", errors.New("primary 500") },
		"f": func(emit func(types.StreamEvent)) (string, error) {
			emit(types.StreamEvent{Kind: types.StreamDelta, Text: "from fallback"})
			return "fallback-model", nil
		},
	})
	r, _ := New([]Endpoint{ep("p", "m1", TierStandard), ep("f", "m2", TierStandard)}, fc,
		WithPublisher(b), WithClock(func() time.Time { return time.UnixMilli(12345) }))
	var evs []types.StreamEvent
	res, err := r.Chat(context.Background(), func(e types.StreamEvent) { evs = append(evs, e) })
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if !res.Switched || res.Switches != 1 || res.Endpoint != "f" || res.Model != "fallback-model" {
		t.Errorf("result = %+v", res)
	}
	if got := fc.calls(); len(got) != 2 || got[0] != "p" || got[1] != "f" {
		t.Errorf("calls = %v", got)
	}
	// 备用吐字透传给上层。
	if len(evs) != 1 || evs[0].Text != "from fallback" {
		t.Errorf("events = %+v", evs)
	}
	// 广播一次门铃，字段齐。
	events := drain(t, sub)
	if len(events) != 1 {
		t.Fatalf("switch events = %d, want 1", len(events))
	}
	e := events[0]
	if e.Topic != "adapter.route.failover" || e.Source != "adapter.router" || e.Version != 1 || e.Timestamp != 12345 {
		t.Errorf("event = %+v", e)
	}
	if err := e.Validate(); err != nil {
		t.Errorf("event invalid: %v", err)
	}
}

// 备用档高于主且未放宽 → 过滤掉，不尝试；主失败直接返回错误。
func TestTierFilterExcludesExpensive(t *testing.T) {
	fc := newFake(map[string]func(func(types.StreamEvent)) (string, error){
		"p": func(emit func(types.StreamEvent)) (string, error) { return "", errors.New("p down") },
		"f": func(emit func(types.StreamEvent)) (string, error) { return "should-not-call", nil },
	})
	primary := ep("p", "m1", TierCheap)
	expensive := ep("f", "m2", TierPremium)
	r, _ := New([]Endpoint{primary, expensive}, fc)
	_, err := r.Chat(context.Background(), func(types.StreamEvent) {})
	if err == nil {
		t.Fatal("应返回主错误")
	}
	if got := fc.calls(); len(got) != 1 || got[0] != "p" {
		t.Errorf("高价备用应被过滤, calls=%v", got)
	}
}

// 放宽档位后同一场景应切到高价备用。
func TestAllowHigherTier(t *testing.T) {
	fc := newFake(map[string]func(func(types.StreamEvent)) (string, error){
		"p": func(emit func(types.StreamEvent)) (string, error) { return "", errors.New("p down") },
		"f": func(emit func(types.StreamEvent)) (string, error) { return "fb", nil },
	})
	r, _ := New([]Endpoint{ep("p", "m1", TierCheap), ep("f", "m2", TierPremium)}, fc, WithAllowHigherTier(true))
	res, err := r.Chat(context.Background(), func(types.StreamEvent) {})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if !res.Switched || res.Model != "fb" {
		t.Errorf("result = %+v", res)
	}
}

// 主档 Unknown 时不参与比较，备用一律放行。
func TestUnknownTierNoFilter(t *testing.T) {
	fc := newFake(map[string]func(func(types.StreamEvent)) (string, error){
		"p": func(emit func(types.StreamEvent)) (string, error) { return "", errors.New("p down") },
		"f": func(emit func(types.StreamEvent)) (string, error) { return "fb", nil },
	})
	r, _ := New([]Endpoint{ep("p", "m1", TierUnknown), ep("f", "m2", TierPremium)}, fc)
	res, err := r.Chat(context.Background(), func(types.StreamEvent) {})
	if err != nil || !res.Switched {
		t.Fatalf("Unknown 档应放行备用: res=%+v err=%v", res, err)
	}
}

// context.Canceled 属主动取消：不切、不广播。
func TestCanceledNoFailover(t *testing.T) {
	b := bus.New(64)
	sub, _ := b.Subscribe()
	defer b.Close()
	fc := newFake(map[string]func(func(types.StreamEvent)) (string, error){
		"p": func(emit func(types.StreamEvent)) (string, error) { return "", context.Canceled },
		"f": func(emit func(types.StreamEvent)) (string, error) { return "fb", nil },
	})
	r, _ := New([]Endpoint{ep("p", "m1", TierStandard), ep("f", "m2", TierStandard)}, fc, WithPublisher(b))
	_, err := r.Chat(context.Background(), func(types.StreamEvent) {})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
	if got := fc.calls(); len(got) != 1 {
		t.Errorf("取消不应切备用, calls=%v", got)
	}
	if events := drain(t, sub); len(events) != 0 {
		t.Errorf("取消不应广播, events=%d", len(events))
	}
}

// predicate 返回 false → 不切。
func TestFailoverPredicateStops(t *testing.T) {
	fc := newFake(map[string]func(func(types.StreamEvent)) (string, error){
		"p": func(emit func(types.StreamEvent)) (string, error) { return "", errors.New("boom") },
		"f": func(emit func(types.StreamEvent)) (string, error) { return "fb", nil },
	})
	r, _ := New([]Endpoint{ep("p", "m1", TierStandard), ep("f", "m2", TierStandard)}, fc,
		WithFailoverPredicate(func(error) bool { return false }))
	_, err := r.Chat(context.Background(), func(types.StreamEvent) {})
	if err == nil {
		t.Fatal("应返回错误")
	}
	if got := fc.calls(); len(got) != 1 {
		t.Errorf("predicate=false 不应切, calls=%v", got)
	}
}

// 已产生可见输出后失败 → 不切（防重复吐字）。
func TestEmittedThenErrorNoFailover(t *testing.T) {
	fc := newFake(map[string]func(func(types.StreamEvent)) (string, error){
		"p": func(emit func(types.StreamEvent)) (string, error) {
			emit(types.StreamEvent{Kind: types.StreamDelta, Text: "partial"})
			return "", errors.New("stream broke mid-way")
		},
		"f": func(emit func(types.StreamEvent)) (string, error) { return "fb", nil },
	})
	r, _ := New([]Endpoint{ep("p", "m1", TierStandard), ep("f", "m2", TierStandard)}, fc)
	var evs []types.StreamEvent
	_, err := r.Chat(context.Background(), func(e types.StreamEvent) { evs = append(evs, e) })
	if err == nil {
		t.Fatal("应返回错误")
	}
	if got := fc.calls(); len(got) != 1 {
		t.Errorf("已吐字后不得切备用(防重复), calls=%v", got)
	}
	if len(evs) != 1 || evs[0].Text != "partial" {
		t.Errorf("只应输出主的部分内容, evs=%+v", evs)
	}
}

// 三端点：前两失败、第三成功 → 广播两次切换，Switches=2。
func TestMultipleSwitches(t *testing.T) {
	b := bus.New(64)
	sub, _ := b.Subscribe()
	defer b.Close()
	fc := newFake(map[string]func(func(types.StreamEvent)) (string, error){
		"a": func(emit func(types.StreamEvent)) (string, error) { return "", errors.New("a down") },
		"b": func(emit func(types.StreamEvent)) (string, error) { return "", errors.New("b down") },
		"c": func(emit func(types.StreamEvent)) (string, error) { return "cmodel", nil },
	})
	r, _ := New([]Endpoint{ep("a", "m1", TierStandard), ep("b", "m2", TierStandard), ep("c", "m3", TierStandard)}, fc, WithPublisher(b))
	res, err := r.Chat(context.Background(), func(types.StreamEvent) {})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if res.Switches != 2 || res.Endpoint != "c" || res.Model != "cmodel" {
		t.Errorf("result = %+v", res)
	}
	if events := drain(t, sub); len(events) != 2 {
		t.Errorf("switch events = %d, want 2", len(events))
	}
}

// 全部失败 → 返回最后错误。
func TestAllFail(t *testing.T) {
	e1 := errors.New("p fail")
	e2 := errors.New("f fail")
	fc := newFake(map[string]func(func(types.StreamEvent)) (string, error){
		"p": func(func(types.StreamEvent)) (string, error) { return "", e1 },
		"f": func(func(types.StreamEvent)) (string, error) { return "", e2 },
	})
	r, _ := New([]Endpoint{ep("p", "m1", TierStandard), ep("f", "m2", TierStandard)}, fc)
	_, err := r.Chat(context.Background(), func(types.StreamEvent) {})
	if !errors.Is(err, e2) {
		t.Errorf("应返回最后一次错误 e2, got %v", err)
	}
}

func TestNewValidation(t *testing.T) {
	if _, err := New(nil, newFake(nil)); err == nil {
		t.Error("空端点应报错")
	}
	if _, err := New([]Endpoint{ep("p", "m", TierCheap)}, nil); err == nil {
		t.Error("nil caller 应报错")
	}
}

func TestPlanAndDescribe(t *testing.T) {
	r, _ := New([]Endpoint{ep("p", "m1", TierStandard), ep("cheap", "m2", TierCheap), ep("prem", "m3", TierPremium)}, newFake(nil))
	plan := r.Plan()
	if len(plan) != 2 || plan[0].Name != "p" || plan[1].Name != "cheap" {
		t.Errorf("plan 应过滤掉高价备用: %+v", plan)
	}
	if got := ep("x", "mm", TierPremium).Describe(); got != "x(mm/premium)" {
		t.Errorf("Describe = %q", got)
	}
}
