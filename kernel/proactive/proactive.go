package proactive

import "time"

// Priority 触发优先级，数值越大越紧急。Urgent 可穿透静默时段。
type Priority int

const (
	PriorityLow    Priority = 0
	PriorityNormal Priority = 1
	PriorityHigh   Priority = 2
	PriorityUrgent Priority = 3
)

// String 便于日志/排障。
func (p Priority) String() string {
	switch p {
	case PriorityLow:
		return "low"
	case PriorityNormal:
		return "normal"
	case PriorityHigh:
		return "high"
	case PriorityUrgent:
		return "urgent"
	default:
		return "unknown"
	}
}

// SourceKind 触发源种类。
type SourceKind string

const (
	SourceMemoryReview SourceKind = "memory_review"
	SourceSchedule     SourceKind = "schedule"
	SourceEmotion      SourceKind = "emotion"
)

// ReviewItem 记忆复习条目（应用层从 hippocampus 只读喂入；本包不依赖 hippocampus）。
type ReviewItem struct {
	ID           string // 记忆 id（去重身份）
	NextReviewAt int64  // UnixMilli，预计留存度降到复习阈值的时刻
	Summary      string // 展示用摘要（可空）
}

// EmotionState 情绪维度快照（key→值；应用层从 endocrine 喂入，本包不依赖 endocrine）。
type EmotionState map[string]float64

// Candidate 触发源产出的候选（尚未过闸门）。
type Candidate struct {
	Source   SourceKind
	Key      string // 去重身份（同一逻辑事件跨轮稳定不变）
	Priority Priority
	Title    string
	Body     string
	Cooldown time.Duration // 0 表示使用调度器默认冷却
}

// Nudge 通过全部闸门后真正发出的主动提醒。
type Nudge struct {
	Source      SourceKind `json:"source"`
	Key         string     `json:"key"`
	Priority    Priority   `json:"priority"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	AtUnixMilli int64      `json:"at_unix_milli"`
}

// Context 一次评估的只读输入。
type Context struct {
	Now     time.Time
	Reviews []ReviewItem
	Emotion EmotionState
}

// Source 触发源接口（可插拔扩展点，约束#8 数据驱动）。实现须为纯函数：
// 只读 Context 与自身配置，不改内部可变状态（去重/限频由调度器统一负责）。
type Source interface {
	Kind() SourceKind
	Candidates(ctx Context) []Candidate
}

// QuietHours 静默时段（本地时刻，分钟自零点；支持跨零点如 23:00–07:00）。
type QuietHours struct {
	Enabled      bool `json:"enabled"`
	StartMinutes int  `json:"start_minutes"` // [0,1440)，越界自动取模
	EndMinutes   int  `json:"end_minutes"`   // [0,1440)，越界自动取模
}

// contains 报告 t 的本地时刻是否落在静默时段；零长度（起点=终点）视为不静默。
func (q QuietHours) contains(t time.Time) bool {
	if !q.Enabled {
		return false
	}
	s := normMinutes(q.StartMinutes)
	e := normMinutes(q.EndMinutes)
	if s == e {
		return false
	}
	m := t.Hour()*60 + t.Minute()
	if s < e {
		return m >= s && m < e
	}
	return m >= s || m < e // 跨零点
}

// normMinutes 把任意分钟取模到 [0,1440)。
func normMinutes(m int) int {
	m %= 1440
	if m < 0 {
		m += 1440
	}
	return m
}
