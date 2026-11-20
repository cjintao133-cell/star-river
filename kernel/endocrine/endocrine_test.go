package endocrine

import (
	"math"
	"testing"
	"time"

	"star-river/kernel/core/clock"
)

func approx(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func testClock() *clock.FakeClock {
	return clock.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
}

// 五维默认注册齐全，初始值=基线。
func TestDefaultFiveDimensions(t *testing.T) {
	reg := DefaultRegistry()
	keys := reg.Keys()
	if len(keys) != 5 {
		t.Fatalf("默认应 5 维, got %v", keys)
	}
	e := New(reg, testClock())
	for _, d := range reg.All() {
		v, ok := e.Value(d.Key)
		if !ok || !approx(v, d.Baseline, 1e-9) {
			t.Errorf("维度 %s 初始应=基线 %v, got %v", d.Key, d.Baseline, v)
		}
	}
}

// 注册表校验：非法定义拒绝。
func TestRegisterInvalid(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(Dimension{Key: "x", Min: 1, Max: 0, Baseline: 0.5}); err != ErrBadDimension {
		t.Error("min>=max 应报错")
	}
	if err := r.Register(Dimension{Key: "y", Min: 0, Max: 1, Baseline: 2}); err != ErrBadDimension {
		t.Error("baseline 越界应报错")
	}
	if err := r.Register(Dimension{Key: "", Min: 0, Max: 1, Baseline: 0.5}); err != ErrBadDimension {
		t.Error("空 key 应报错")
	}
}

// 衰减（假时钟）：效价从 1 出发，1 天后按 exp(-0.5) 向基线 0 收敛。
func TestDecayWithFakeClock(t *testing.T) {
	clk := testClock()
	reg := NewRegistry()
	_ = reg.Register(Dimension{Key: "v", Label: "v", Min: -1, Max: 1, Baseline: 0, DecayPerDay: 0.5})
	e := New(reg, clk)
	e.Interact(map[string]float64{"v": 1.0}) // 拉到 1
	if v, _ := e.Value("v"); !approx(v, 1.0, 1e-9) {
		t.Fatalf("交互后应 1.0, got %v", v)
	}
	clk.Advance(24 * time.Hour)
	e.Tick()
	want := math.Exp(-0.5) // baseline0 + (1-0)*exp(-0.5*1)
	if v, _ := e.Value("v"); !approx(v, want, 1e-6) {
		t.Errorf("1 天后 = %v, want %v", v, want)
	}
}

// 交互增量叠加 + 夹取范围。
func TestInteractClamp(t *testing.T) {
	clk := testClock()
	reg := NewRegistry()
	_ = reg.Register(Dimension{Key: "a", Min: 0, Max: 1, Baseline: 0, DecayPerDay: 0})
	e := New(reg, clk)
	e.Interact(map[string]float64{"a": 5.0}) // 超上限
	if v, _ := e.Value("a"); v != 1.0 {
		t.Errorf("应夹到上限 1, got %v", v)
	}
	e.Interact(map[string]float64{"a": -3.0}) // 超下限
	if v, _ := e.Value("a"); v != 0.0 {
		t.Errorf("应夹到下限 0, got %v", v)
	}
	// 未知维度忽略，不 panic。
	e.Interact(map[string]float64{"ghost": 9})
}

// 可扩展：注册 Dominance 后自动以基线出现；注销后消失。
func TestExtendWithDominance(t *testing.T) {
	clk := testClock()
	reg := DefaultRegistry()
	e := New(reg, clk)
	if _, ok := e.Value("dominance"); ok {
		t.Fatal("扩展前不应有 dominance")
	}
	if err := reg.Register(DominanceDimension()); err != nil {
		t.Fatal(err)
	}
	e.Tick() // 触发 syncDims 补新维度
	if v, ok := e.Value("dominance"); !ok || !approx(v, 0.4, 1e-9) {
		t.Errorf("注册 dominance 后应出现基线 0.4, got %v ok=%v", v, ok)
	}
	// 可交互。
	e.Interact(map[string]float64{"dominance": 0.3})
	if v, _ := e.Value("dominance"); !approx(v, 0.7, 1e-9) {
		t.Errorf("dominance 交互后 = %v, 应 0.7", v)
	}
	// 注销后消失。
	reg.Unregister("dominance")
	e.Tick()
	if _, ok := e.Value("dominance"); ok {
		t.Error("注销后不应再有 dominance")
	}
}

// 持久化往返：快照→新引擎恢复→状态一致，且能继续按快照时刻衰减。
func TestSnapshotRestore(t *testing.T) {
	clk := testClock()
	reg := DefaultRegistry()
	e := New(reg, clk)
	e.Interact(map[string]float64{"valence": 0.8, "intimacy": 0.6})
	snap := e.Snapshot()

	// 新引擎（同注册表），先时间跳跃再恢复。
	clk2 := clock.NewFakeClock(clk.Now().Add(10 * 24 * time.Hour))
	e2 := New(reg, clk2)
	e2.Restore(snap)
	// 恢复后（未推进 e2 时钟）值应等于快照值。
	if v, _ := e2.Value("valence"); !approx(v, 0.8, 1e-6) {
		t.Errorf("恢复 valence = %v, 应 0.8", v)
	}
	if v, _ := e2.Value("intimacy"); !approx(v, 0.7, 1e-6) {
		t.Errorf("恢复 intimacy = %v, 应 0.7", v)
	}
	// 从快照时刻再推进 1 天并 Tick，应发生衰减（证明 lastTick 恢复到快照时刻而非 now）。
	clk2.Set(time.UnixMilli(snap.AtUnixMilli).Add(24 * time.Hour))
	e2.Tick()
	if v, _ := e2.Value("valence"); v >= 0.8 {
		t.Errorf("恢复后 1 天应衰减 <0.8, got %v", v)
	}
}

// State/SortedState 维度数与顺序。
func TestStateAndSorted(t *testing.T) {
	e := New(DefaultRegistry(), testClock())
	st := e.State()
	if len(st) != 5 {
		t.Errorf("State 应 5 维, got %d", len(st))
	}
	ss := e.SortedState()
	if len(ss) != 5 || ss[0].Key != KeyValence {
		t.Errorf("SortedState 应按注册序 valence 起, got %+v", ss)
	}
}

// 恢复时越界值按维度夹取、未知维度忽略。
func TestRestoreClampAndUnknown(t *testing.T) {
	e := New(DefaultRegistry(), testClock())
	e.Restore(Snapshot{State: map[string]float64{"valence": 99, "ghost": 5}, AtUnixMilli: 0})
	if v, _ := e.Value("valence"); v != 1.0 {
		t.Errorf("越界 valence 应夹到 1, got %v", v)
	}
	if _, ok := e.Value("ghost"); ok {
		t.Error("未知维度不应被恢复")
	}
}

func TestDecayTowardNoDecay(t *testing.T) {
	d := Dimension{Key: "k", Min: 0, Max: 1, Baseline: 0, DecayPerDay: 0}
	if v := d.decayToward(0.8, 5); v != 0.8 {
		t.Errorf("DecayPerDay=0 应不动, got %v", v)
	}
	if v := d.decayToward(1.5, 5); v != 1.0 {
		t.Errorf("应夹取 1.0, got %v", v)
	}
}

func TestUnregisterIdempotent(t *testing.T) {
	r := NewRegistry()
	r.Unregister("ghost") // 不存在，不 panic
	_ = r.Register(Dimension{Key: "a", Min: 0, Max: 1, Baseline: 0.5})
	r.Unregister("a")
	if _, ok := r.Get("a"); ok {
		t.Error("注销后应不存在")
	}
	r.Unregister("a") // 再次幂等
}

func TestNewDefaults(t *testing.T) {
	e := New(nil, nil) // nil reg/clock → 默认
	if len(e.State()) != 5 {
		t.Errorf("nil 注册表应用默认五维, got %d", len(e.State()))
	}
}
