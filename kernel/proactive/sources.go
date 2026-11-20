package proactive

import "time"

// defaultScheduleWindowMinutes 日程条目未指定 WindowMinutes 时的默认触发窗口。
const defaultScheduleWindowMinutes = 30

// —— 触发源 1：记忆复习到期 ——

// MemoryReviewSource 记忆复习到期触发源：NextReviewAt<=now 的条目产出提醒。
type MemoryReviewSource struct{}

// Kind 实现 Source。
func (MemoryReviewSource) Kind() SourceKind { return SourceMemoryReview }

// Candidates 实现 Source：逐条判到期，Key 以记忆 id 固定（同一记忆不重复打扰，去重交调度器冷却）。
func (MemoryReviewSource) Candidates(ctx Context) []Candidate {
	now := ctx.Now.UnixMilli()
	out := make([]Candidate, 0, len(ctx.Reviews))
	for _, r := range ctx.Reviews {
		if r.ID == "" || r.NextReviewAt > now {
			continue
		}
		out = append(out, Candidate{
			Source:   SourceMemoryReview,
			Key:      "memory_review:" + r.ID,
			Priority: PriorityNormal,
			Title:    "该复习了",
			Body:     r.Summary,
		})
	}
	return out
}

// —— 触发源 2：人设日程 ——

// ScheduleEntry 一条人设日程：每天 AtMinutes 起、持续 WindowMinutes 的窗口内触发一次。
// WindowMinutes<=0 用默认 30；Weekdays 空表示每天。
type ScheduleEntry struct {
	Key           string
	Title         string
	Body          string
	Priority      Priority
	AtMinutes     int            // 每天触发起点（零点起分钟，自动取模）
	WindowMinutes int            // 触发窗口长度（分钟）
	Weekdays      []time.Weekday // 允许的星期；空=每天
	Cooldown      time.Duration  // 0 用调度器默认
}

// ScheduleSource 人设日程触发源（数据驱动：加一条日程=加一条数据）。
type ScheduleSource struct {
	Entries []ScheduleEntry
}

// Kind 实现 Source。
func (ScheduleSource) Kind() SourceKind { return SourceSchedule }

// Candidates 实现 Source：命中星期 + 落在 At..At+Window 窗口内则产出。
func (s ScheduleSource) Candidates(ctx Context) []Candidate {
	t := ctx.Now
	cur := t.Hour()*60 + t.Minute()
	wd := t.Weekday()
	out := make([]Candidate, 0, len(s.Entries))
	for _, e := range s.Entries {
		if e.Key == "" || !weekdayAllowed(e.Weekdays, wd) {
			continue
		}
		win := e.WindowMinutes
		if win <= 0 {
			win = defaultScheduleWindowMinutes
		}
		if !inWindow(cur, normMinutes(e.AtMinutes), win) {
			continue
		}
		out = append(out, Candidate{
			Source:   SourceSchedule,
			Key:      "schedule:" + e.Key,
			Priority: e.Priority,
			Title:    e.Title,
			Body:     e.Body,
			Cooldown: e.Cooldown,
		})
	}
	return out
}

// inWindow 报告 cur 是否落在以 start 为起点、长度 win 的窗口内（按日循环，支持跨零点）。
func inWindow(cur, start, win int) bool {
	if win <= 0 {
		return false
	}
	d := (cur - start + 1440) % 1440
	return d < win
}

// weekdayAllowed 报告 d 是否在允许集合内；空集合=每天。
func weekdayAllowed(days []time.Weekday, d time.Weekday) bool {
	if len(days) == 0 {
		return true
	}
	for _, x := range days {
		if x == d {
			return true
		}
	}
	return false
}

// DefaultScheduleEntries 一批示例日程（吃饭/休息）；宿主可整表替换或置空。
func DefaultScheduleEntries() []ScheduleEntry {
	return []ScheduleEntry{
		{Key: "meal_lunch", Title: "午餐提醒", Body: "到点了，记得按时吃饭。", Priority: PriorityNormal, AtMinutes: 12 * 60},
		{Key: "sleep", Title: "休息提醒", Body: "夜深了，注意休息。", Priority: PriorityNormal, AtMinutes: 23 * 60, WindowMinutes: 60},
	}
}

// —— 触发源 3：情绪驱动 ——

// CompareOp 情绪阈值比较算子。
type CompareOp string

const (
	OpGE CompareOp = ">="
	OpLE CompareOp = "<="
	OpGT CompareOp = ">"
	OpLT CompareOp = "<"
)

// EmotionRule 一条情绪驱动规则：某维度满足比较条件即产出主动提醒。
// Dimension 取值应匹配 endocrine 维度 key（"valence"/"arousal"/"intimacy"/"fatigue"/"curiosity"），
// 但本包不 import endocrine——仅按字符串读取快照，保持插件间零耦合。
type EmotionRule struct {
	Key       string
	Dimension string
	Op        CompareOp
	Threshold float64
	Priority  Priority
	Title     string
	Body      string
	Cooldown  time.Duration
}

// EmotionSource 情绪驱动触发源（数据驱动：加一条规则=加一条数据）。
type EmotionSource struct {
	Rules []EmotionRule
}

// Kind 实现 Source。
func (EmotionSource) Kind() SourceKind { return SourceEmotion }

// Candidates 实现 Source：维度存在且满足条件则产出；缺维度跳过。
func (s EmotionSource) Candidates(ctx Context) []Candidate {
	out := make([]Candidate, 0, len(s.Rules))
	for _, r := range s.Rules {
		if r.Key == "" {
			continue
		}
		v, ok := ctx.Emotion[r.Dimension]
		if !ok || !compareOp(v, r.Op, r.Threshold) {
			continue
		}
		out = append(out, Candidate{
			Source:   SourceEmotion,
			Key:      "emotion:" + r.Key,
			Priority: r.Priority,
			Title:    r.Title,
			Body:     r.Body,
			Cooldown: r.Cooldown,
		})
	}
	return out
}

// compareOp 按算子比较；未知算子返回 false（不触发）。
func compareOp(v float64, op CompareOp, th float64) bool {
	switch op {
	case OpGE:
		return v >= th
	case OpLE:
		return v <= th
	case OpGT:
		return v > th
	case OpLT:
		return v < th
	default:
		return false
	}
}

// DefaultEmotionRules 示例情绪规则（疲劳高→劝休息）；宿主可整表替换或置空。
func DefaultEmotionRules() []EmotionRule {
	return []EmotionRule{
		{Key: "fatigue_rest", Dimension: "fatigue", Op: OpGE, Threshold: 0.8, Priority: PriorityHigh,
			Title: "休息一下", Body: "看起来有些疲劳，起身活动一下会好些。"},
	}
}
