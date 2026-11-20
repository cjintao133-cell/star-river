package endocrine

import (
	"strings"
	"testing"

	"star-river/kernel/core/clock"
)

func cptr() *clock.FakeClock { return clock.NewFakeClock(clock.RealClock{}.Now()) }

// 基线状态 → 措辞参数 = 基线。
func TestStyleAtBaseline(t *testing.T) {
	reg := DefaultRegistry()
	c := DefaultCoupler(reg)
	state := map[string]float64{}
	for _, d := range reg.All() {
		state[d.Key] = d.Baseline
	}
	s := c.Style(state)
	base := DefaultStyleBase()
	if s != base {
		t.Errorf("基线态措辞应=base %+v, got %+v", base, s)
	}
}

// 高亲密 → 更亲昵、更随意、更俏皮（方向正确）。
func TestStyleHighIntimacy(t *testing.T) {
	reg := DefaultRegistry()
	c := DefaultCoupler(reg)
	base := map[string]float64{}
	for _, d := range reg.All() {
		base[d.Key] = d.Baseline
	}
	hi := map[string]float64{}
	for k, v := range base {
		hi[k] = v
	}
	hi[KeyIntimacy] = 1.0 // 拉满亲密
	s := c.Style(hi)
	b := c.Style(base)
	if !(s.Warmth > b.Warmth) {
		t.Errorf("高亲密应更热情: %v vs %v", s.Warmth, b.Warmth)
	}
	if !(s.Formality < b.Formality) {
		t.Errorf("高亲密应更不正式: %v vs %v", s.Formality, b.Formality)
	}
	if !(s.Playfulness > b.Playfulness) {
		t.Errorf("高亲密应更俏皮: %v vs %v", s.Playfulness, b.Playfulness)
	}
}

// 高疲劳 → 话量降、热情降。
func TestStyleHighFatigue(t *testing.T) {
	reg := DefaultRegistry()
	c := DefaultCoupler(reg)
	base := map[string]float64{}
	for _, d := range reg.All() {
		base[d.Key] = d.Baseline
	}
	ft := map[string]float64{}
	for k, v := range base {
		ft[k] = v
	}
	ft[KeyFatigue] = 1.0
	s := c.Style(ft)
	b := c.Style(base)
	if !(s.Verbosity < b.Verbosity) || !(s.Warmth < b.Warmth) {
		t.Errorf("高疲劳应话少淡: v=%v/%v w=%v/%v", s.Verbosity, b.Verbosity, s.Warmth, b.Warmth)
	}
}

// 可配置：换一套规则，映射随之变；未知维度/目标容错。
func TestStyleCustomRules(t *testing.T) {
	reg := DefaultRegistry()
	// 自定义：只用效价影响温度，权重 1.0，base 温度 1.0。
	c := NewCoupler(reg, []Rule{{Dimension: KeyValence, Target: "temperature", Weight: 1.0}},
		StyleParams{Temperature: 1.0, Verbosity: 0.5, Warmth: 0.4, Playfulness: 0.3, Formality: 0.4})
	state := map[string]float64{KeyValence: 0.5} // 基线 0 → +0.5
	s := c.Style(state)
	if !approx(s.Temperature, 1.5, 1e-9) {
		t.Errorf("自定义规则温度应 1.5, got %v", s.Temperature)
	}
	// 未知维度/未知目标不 panic。
	c2 := NewCoupler(reg, []Rule{{Dimension: "ghost", Target: "temperature", Weight: 1}, {Dimension: KeyValence, Target: "bogus", Weight: 1}}, DefaultStyleBase())
	_ = c2.Style(map[string]float64{KeyValence: 1})
}

// 夹取：极端状态不越界。
func TestStyleClamp(t *testing.T) {
	reg := DefaultRegistry()
	c := DefaultCoupler(reg)
	state := map[string]float64{KeyValence: 1, KeyArousal: 1, KeyIntimacy: 1, KeyFatigue: 0, KeyCuriosity: 1}
	s := c.Style(state)
	if s.Verbosity < 0 || s.Verbosity > 1 || s.Warmth < 0 || s.Warmth > 1 || s.Temperature < 0 || s.Temperature > 2 {
		t.Errorf("越界: %+v", s)
	}
}

// 情绪强度：高唤醒+强效价 → 接近 1；全静息 → 0。
func TestEncodingStrength(t *testing.T) {
	reg := DefaultRegistry()
	c := DefaultCoupler(reg)
	if got := c.EncodingStrength(map[string]float64{KeyArousal: 1, KeyValence: 1}); !approx(got, 1.0, 1e-9) {
		t.Errorf("高情绪强度应 1.0, got %v", got)
	}
	if got := c.EncodingStrength(map[string]float64{KeyArousal: 0, KeyValence: 0}); got != 0 {
		t.Errorf("静息应 0, got %v", got)
	}
	// 负效价也计入强度（绝对值）。
	if got := c.EncodingStrength(map[string]float64{KeyArousal: 0, KeyValence: -1}); !approx(got, 0.5, 1e-9) {
		t.Errorf("强负效价强度应 0.5, got %v", got)
	}
}

// PromptDirective：极端参数产生可区分指令；中间态非空。
func TestPromptDirective(t *testing.T) {
	warm := StyleParams{Warmth: 0.9, Playfulness: 0.9, Formality: 0.1, Verbosity: 0.9, Temperature: 1.5}
	cold := StyleParams{Warmth: 0.1, Playfulness: 0.1, Formality: 0.9, Verbosity: 0.1, Temperature: 0.3}
	dw := warm.PromptDirective()
	dc := cold.PromptDirective()
	if !strings.Contains(dw, "热情") || !strings.Contains(dw, "俏皮") {
		t.Errorf("热情指令缺关键: %q", dw)
	}
	if !strings.Contains(dc, "正式") || !strings.Contains(dc, "简短") {
		t.Errorf("冷淡指令缺关键: %q", dc)
	}
	if dw == dc {
		t.Error("冷热指令应可区分")
	}
}

// 端到端：Engine 交互把情绪拉高 → Coupler 读到该状态给出对应措辞。
func TestEngineToCouplerE2E(t *testing.T) {
	reg := DefaultRegistry()
	eng := New(reg, cptr())
	cpl := DefaultCoupler(reg)
	before := cpl.Style(eng.State())

	// 大幅提升亲密与效价。
	eng.Interact(map[string]float64{KeyIntimacy: 0.8, KeyValence: 0.9})
	after := cpl.Style(eng.State())
	if !(after.Warmth > before.Warmth) {
		t.Errorf("情绪拉高后热情应升: %v -> %v", before.Warmth, after.Warmth)
	}
}

func TestNewCouplerNilRegistry(t *testing.T) {
	c := NewCoupler(nil, DefaultRules(), DefaultStyleBase())
	if c.reg == nil {
		t.Fatal("nil reg 应回退默认")
	}
	// 用默认五维基线态可算出参数。
	s := c.Style(map[string]float64{KeyValence: 0, KeyArousal: 0.3, KeyIntimacy: 0.1, KeyFatigue: 0, KeyCuriosity: 0.5})
	if s == (StyleParams{}) {
		t.Error("应产出非零参数")
	}
}
