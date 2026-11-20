// 情绪耦合输出（T-M4-02）：情绪状态 → 措辞参数 映射 + 情绪强度（供记忆编码）。
//
// 红线2：本包不 import 其他插件包。Style/EncodingStrength 为纯函数，由**应用装配层**
// （非底座）在编排时调用：把 Style 拼进 system prompt / 采样参数，把 EncodingStrength
// 作为 emotion 传给 hippocampus.Encode（高情绪 → 记忆初始 stability 更高，见 T-M3-02）。
//
// 映射"可配置"：措辞参数 = 各基线 + Σ 规则权重×(维度值 − 维度基线)，再夹范围。
// 规则表数据驱动，可整体替换（NewCoupler(reg, rules, base)）。
package endocrine

import "math"

// StyleParams 措辞参数（情绪耦合输出）。均为可比/可测标量，供采样与 prompt 使用。
type StyleParams struct {
	Temperature float64 `json:"temperature"` // 0..2 采样温度（高=更发散活泼）
	Verbosity   float64 `json:"verbosity"`   // 0..1 话量（高=更长）
	Warmth      float64 `json:"warmth"`      // 0..1 热情/亲昵
	Playfulness float64 `json:"playfulness"` // 0..1 俏皮/玩笑
	Formality   float64 `json:"formality"`   // 0..1 正式度（高=端着）
}

// Rule 一条耦合规则：dimension 相对其基线每偏移 1，target 参数加 weight。
type Rule struct {
	Dimension string  `json:"dimension"`
	Target    string  `json:"target"` // temperature/verbosity/warmth/playfulness/formality
	Weight    float64 `json:"weight"`
}

// Coupler 情绪→措辞耦合器（由规则表驱动，可配置）。
type Coupler struct {
	reg   *Registry
	rules []Rule
	base  StyleParams
}

// NewCoupler 用规则表 + 基线建耦合器。reg 用于取各维度基线；nil 用 DefaultRegistry。
func NewCoupler(reg *Registry, rules []Rule, base StyleParams) *Coupler {
	if reg == nil {
		reg = DefaultRegistry()
	}
	return &Coupler{reg: reg, rules: append([]Rule(nil), rules...), base: base}
}

// DefaultRules 默认耦合规则（贴合伴侣直觉）。
func DefaultRules() []Rule {
	return []Rule{
		// 效价（基0）：越高越热情、越放松、越不端着。
		{Dimension: KeyValence, Target: "warmth", Weight: 0.5},
		{Dimension: KeyValence, Target: "temperature", Weight: 0.3},
		{Dimension: KeyValence, Target: "formality", Weight: -0.3},
		// 唤醒（基0.3）：越高越发散、越短促（话量降）。
		{Dimension: KeyArousal, Target: "verbosity", Weight: -0.4},
		{Dimension: KeyArousal, Target: "temperature", Weight: 0.4},
		// 亲密（基0.1）：越高越亲昵、越随意、越俏皮。
		{Dimension: KeyIntimacy, Target: "warmth", Weight: 0.5},
		{Dimension: KeyIntimacy, Target: "formality", Weight: -0.5},
		{Dimension: KeyIntimacy, Target: "playfulness", Weight: 0.3},
		// 疲劳（基0）：越累话越少、越淡。
		{Dimension: KeyFatigue, Target: "verbosity", Weight: -0.5},
		{Dimension: KeyFatigue, Target: "warmth", Weight: -0.2},
		// 好奇（基0.5）：越高越健谈、越俏皮。
		{Dimension: KeyCuriosity, Target: "verbosity", Weight: 0.3},
		{Dimension: KeyCuriosity, Target: "playfulness", Weight: 0.4},
	}
}

// DefaultStyleBase 默认基线措辞参数（情绪全处基线时的样子）。
func DefaultStyleBase() StyleParams {
	return StyleParams{Temperature: 0.8, Verbosity: 0.5, Warmth: 0.4, Playfulness: 0.3, Formality: 0.4}
}

// DefaultCoupler 默认配置耦合器。
func DefaultCoupler(reg *Registry) *Coupler {
	return NewCoupler(reg, DefaultRules(), DefaultStyleBase())
}

// Style 由情绪状态计算措辞参数。state 通常取 Engine.State()。
func (c *Coupler) Style(state map[string]float64) StyleParams {
	s := c.base
	for _, r := range c.rules {
		d, ok := c.reg.Get(r.Dimension)
		if !ok {
			continue
		}
		v, ok := state[r.Dimension]
		if !ok {
			continue
		}
		delta := (v - d.Baseline) * r.Weight
		switch r.Target {
		case "temperature":
			s.Temperature += delta
		case "verbosity":
			s.Verbosity += delta
		case "warmth":
			s.Warmth += delta
		case "playfulness":
			s.Playfulness += delta
		case "formality":
			s.Formality += delta
		default:
			// 未知目标忽略（容错）
		}
	}
	s.Temperature = clampRange(s.Temperature, 0, 2)
	s.Verbosity = clampRange(s.Verbosity, 0, 1)
	s.Warmth = clampRange(s.Warmth, 0, 1)
	s.Playfulness = clampRange(s.Playfulness, 0, 1)
	s.Formality = clampRange(s.Formality, 0, 1)
	return s
}

// EncodingStrength 情绪强度 [0,1]：供记忆编码（高值 → 记忆初始 stability 更高）。
// 综合唤醒与效价绝对强度：0.5*arousal + 0.5*|valence|（未知维度按 0 计）。
func (c *Coupler) EncodingStrength(state map[string]float64) float64 {
	ar := state[KeyArousal]
	va := state[KeyValence]
	st := 0.5*ar + 0.5*math.Abs(va)
	return clampRange(st, 0, 1)
}

// PromptDirective 把措辞参数翻成一段中文语气指令（供拼 system prompt）。
func (s StyleParams) PromptDirective() string {
	d := "【当前语气】"
	if s.Warmth >= 0.6 {
		d += "语气热情亲昵；"
	} else if s.Warmth <= 0.25 {
		d += "语气偏淡、克制；"
	}
	if s.Playfulness >= 0.6 {
		d += "带点俏皮玩笑；"
	} else if s.Playfulness <= 0.2 {
		d += "少开玩笑、认真；"
	}
	if s.Formality >= 0.6 {
		d += "措辞正式得体；"
	} else if s.Formality <= 0.3 {
		d += "口语随意、放松；"
	}
	if s.Verbosity >= 0.6 {
		d += "可以多说几句；"
	} else if s.Verbosity <= 0.3 {
		d += "简短别啰嗦；"
	}
	if s.Temperature >= 1.2 {
		d += "表达更发散活泼。"
	} else if s.Temperature <= 0.5 {
		d += "表达更收敛稳重。"
	}
	return d
}

func clampRange(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
