package hippocampus

import (
	"math"
)

// dayMillis 一天的毫秒数（衰减以「天」为时间单位）。
const dayMillis = int64(24 * 60 * 60 * 1000)

// Memory 一条记忆。时间戳 UnixMilli（core/types 契约）。S 与 Strength 为双精度。
type Memory struct {
	ID               string
	Text             string  // 原文/摘要（检索与展示用；embedding 与原文分离）
	Importance       float64 // 重要度 [0,1]
	EmotionIntensity float64 // 情感强度 [0,1]
	Stability        float64 // 稳定度 S（天）；初始值由编码时 importance/emotion 决定
	Strength         float64 // 强度（命中累计，单调增，用于排序/展示）
	ReviewCount      int
	CreatedAt        int64 // UnixMilli
	LastReviewAt     int64 // UnixMilli（衰减计时起点）
	NextReviewAt     int64 // UnixMilli（预计 R 降至复习阈值的时间点）
}

// daysBetween 返回两 UnixMilli 之间的天数（浮点）。
func daysBetween(from, to int64) float64 {
	return float64(to-from) / float64(dayMillis)
}

// Retention R = exp(-t/S)：当前留存度。t=距上次复习天数，S=稳定度（天）。
// S<=0 视为将遗忘（返回 0）；t<=0（未来/刚复习）返回 1。
func Retention(m Memory, now int64) float64 {
	if m.Stability <= 0 {
		return 0
	}
	t := daysBetween(m.LastReviewAt, now)
	if t <= 0 {
		return 1
	}
	return math.Exp(-t / m.Stability)
}

// initialStability 编码初始稳定度：基准 stabilityBase，按 importance 与 emotion 加成。
// 高重要/高情绪事件记得更牢（T-M4-02 情绪→记忆编码耦合在此预留接口）。
func initialStability(importance, emotion, base float64) float64 {
	// base * (1 + 2*importance + 2*emotion)：两项都把 S 提升，最高约 5×base。
	return base * (1 + 2*importance + 2*emotion)
}

// reinforceStability 命中强化：S' = S * (1 + k*(1-R))。R 越低（越将忘）强化越多。
// 用当前 R 计算（调用方传入 before）。k 默认见 Engine.k。
func reinforceStability(s, r, k float64) float64 {
	if r < 0 {
		r = 0
	}
	if r > 1 {
		r = 1
	}
	return s * (1 + k*(1-r))
}

// nextReviewAt 计算 R 降到 threshold 所需天数，返回绝对时刻（UnixMilli）。
// R = exp(-t/S) => t = S * ln(1/threshold)。threshold∈(0,1)。
func nextReviewAt(lastReview int64, s, threshold float64) int64 {
	if s <= 0 || threshold <= 0 || threshold >= 1 {
		return lastReview
	}
	days := s * math.Log(1.0/threshold)
	return lastReview + int64(days*float64(dayMillis))
}

// recallScore 召回打分 = sim × (α+βR) × (γ+importance+emotion)。
func recallScore(sim, r, importance, emotion, alpha, beta, gamma float64) float64 {
	f1 := alpha + beta*r
	f2 := gamma + importance + emotion
	return sim * f1 * f2
}

// clamp01 夹到 [0,1]。
func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// nowMilliOf 从 Clock 取 UnixMilli。
func nowMilliOf(c Clock) int64 { return c.Now().UnixMilli() }
