package hippocampus

import "sort"

// Options 引擎参数（零值走默认，见 withDefaults）。
type Options struct {
	Clock           Clock
	StabilityBase   float64 // 初始稳定度基准（天），默认 1.0
	K               float64 // 强化系数，默认 1.0
	Alpha           float64 // 打分 α（R 权重基），默认 0.3
	Beta            float64 // 打分 β，默认 0.7
	Gamma           float64 // 打分 γ（基线），默认 0.1
	Threshold       float64 // 召回分阈值，低于不进上下文，默认 0.15
	ReviewThreshold float64 // R 降到此时算"该复习"，用于 next_review_at，默认 0.5
	StrongImp       float64 // 突然想起：importance 门槛，默认 0.8
	StrongEmo       float64 // 突然想起：emotion 门槛，默认 0.8
	StrongSim       float64 // 突然想起：语义相似度门槛（强线索），默认 0.7
}

// Engine 记忆引擎。非并发安全（上层编排器串行调用；如需并发由调用方加锁）。
type Engine struct {
	clock           Clock
	stabilityBase   float64
	k               float64
	alpha, beta     float64
	gamma           float64
	threshold       float64
	reviewThreshold float64
	strongImp       float64
	strongEmo       float64
	strongSim       float64
	mem             map[string]*Memory
}

// New 建引擎。
func New(o Options) *Engine {
	if o.Clock == nil {
		o.Clock = RealClock{}
	}
	if o.StabilityBase <= 0 {
		o.StabilityBase = 1.0
	}
	if o.K <= 0 {
		o.K = 1.0
	}
	if o.Alpha == 0 && o.Beta == 0 {
		o.Alpha, o.Beta = 0.3, 0.7
	}
	if o.Gamma == 0 {
		o.Gamma = 0.1
	}
	if o.Threshold == 0 {
		o.Threshold = 0.15
	}
	if o.ReviewThreshold == 0 {
		o.ReviewThreshold = 0.5
	}
	if o.StrongImp == 0 {
		o.StrongImp = 0.8
	}
	if o.StrongEmo == 0 {
		o.StrongEmo = 0.8
	}
	if o.StrongSim == 0 {
		o.StrongSim = 0.7
	}
	return &Engine{
		clock: o.Clock, stabilityBase: o.StabilityBase, k: o.K,
		alpha: o.Alpha, beta: o.Beta, gamma: o.Gamma,
		threshold: o.Threshold, reviewThreshold: o.ReviewThreshold,
		strongImp: o.StrongImp, strongEmo: o.StrongEmo, strongSim: o.StrongSim,
		mem: make(map[string]*Memory),
	}
}

// Encode 编码一条新记忆：算初始稳定度、首次复习时刻与 next_review_at。
// importance/emotion 夹到 [0,1]。返回记忆副本。
func (e *Engine) Encode(id, text string, importance, emotion float64) Memory {
	now := nowMilliOf(e.clock)
	imp := clamp01(importance)
	emo := clamp01(emotion)
	s := initialStability(imp, emo, e.stabilityBase)
	m := &Memory{
		ID: id, Text: text,
		Importance: imp, EmotionIntensity: emo,
		Stability: s, Strength: 1.0, ReviewCount: 0,
		CreatedAt: now, LastReviewAt: now,
		NextReviewAt: nextReviewAt(now, s, e.reviewThreshold),
	}
	e.mem[id] = m
	return *m
}

// Get 取记忆副本。
func (e *Engine) Get(id string) (Memory, bool) {
	m, ok := e.mem[id]
	if !ok {
		return Memory{}, false
	}
	return *m, true
}

// Len 记忆条数。
func (e *Engine) Len() int { return len(e.mem) }

// Retention 当前留存度（便捷）。
func (e *Engine) Retention(id string) float64 {
	m, ok := e.mem[id]
	if !ok {
		return 0
	}
	return Retention(*m, nowMilliOf(e.clock))
}

// Review 命中强化：按当前 R 计算 S' = S*(1+k(1-R))，更新时刻与 next_review_at。
// 返回强化后的记忆与强化前的 R。未见 id 返回 false。
func (e *Engine) Review(id string) (Memory, float64, bool) {
	m, ok := e.mem[id]
	if !ok {
		return Memory{}, 0, false
	}
	now := nowMilliOf(e.clock)
	r := Retention(*m, now)
	m.Stability = reinforceStability(m.Stability, r, e.k)
	m.Strength += 1.0
	m.ReviewCount++
	m.LastReviewAt = now
	m.NextReviewAt = nextReviewAt(now, m.Stability, e.reviewThreshold)
	return *m, r, true
}

// Candidate 参与召回的记忆 + 其语义相似度（由调用方/向量检索提供，[0,1]）。
type Candidate struct {
	Memory     Memory
	Similarity float64
}

// Scored 召回结果。
type Scored struct {
	Memory     Memory
	Similarity float64
	Retention  float64
	Score      float64
	Surprise   bool // 突然想起（强线索强制纳入）
}

// Recall 对候选打分、过滤（低于阈值不进上下文；强线索"突然想起"可强制纳入）、按分降序。
// 不改变记忆状态（强化由显式 Review 触发）。
func (e *Engine) Recall(cands []Candidate) []Scored {
	now := nowMilliOf(e.clock)
	out := make([]Scored, 0, len(cands))
	for _, c := range cands {
		m := c.Memory
		r := Retention(m, now)
		sim := clamp01(c.Similarity)
		score := recallScore(sim, r, m.Importance, m.EmotionIntensity, e.alpha, e.beta, e.gamma)
		surprise := (m.Importance >= e.strongImp || m.EmotionIntensity >= e.strongEmo) && sim >= e.strongSim
		if score < e.threshold && !surprise {
			continue
		}
		out = append(out, Scored{Memory: m, Similarity: sim, Retention: r, Score: score, Surprise: surprise})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// RecallFromStore 便捷：对引擎内全部记忆以给定相似度函数打分召回。
func (e *Engine) RecallFromStore(sim func(m Memory) float64) []Scored {
	cands := make([]Candidate, 0, len(e.mem))
	for _, m := range e.mem {
		cands = append(cands, Candidate{Memory: *m, Similarity: sim(*m)})
	}
	return e.Recall(cands)
}
