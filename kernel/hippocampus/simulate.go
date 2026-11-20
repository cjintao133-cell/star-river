package hippocampus

import "time"

// T-M3-06 月测：用假时钟推进，模拟记忆衰减与到期复习触发，输出报告。
// 复现"每天：先记下当日留存（衰减曲线），再对 next_review_at 已到的记忆触发复习"。

// DaySample 衰减曲线采样点（某天、某记忆在当日复习前的留存度）。
type DaySample struct {
	Day       int
	Retention float64
}

// MemorySim 单条记忆的月测结果。
type MemorySim struct {
	ID             string
	Reviews        int
	FinalStability float64
	FinalRetention float64
}

// MonthReport 月测报告。
type MonthReport struct {
	Days         int
	TotalReviews int
	Memories     []MemorySim
	DecayCurve   []DaySample // 首个记忆的逐日留存（复习前），呈锯齿=衰减→复习回升
	FinalMeanRet float64
}

// RunMonth 在假时钟上推进 days 天，逐日：采样留存 → 触发到期复习。返回报告。
// fc 为假时钟（须与 Engine 使用同一时钟）。days<=0 视为 30。
func RunMonth(e *Engine, fc *FakeClock, days int) MonthReport {
	if days <= 0 {
		days = 30
	}
	rep := MonthReport{Days: days}
	// 记录的探针记忆（首个）。
	var probeID string
	for id := range e.mem {
		probeID = id
		break
	}
	for d := 1; d <= days; d++ {
		fc.Advance(24 * time.Hour)
		now := fc.Now().UnixMilli()
		// 采样探针留存（复习前）。
		if probeID != "" {
			if m, ok := e.mem[probeID]; ok {
				rep.DecayCurve = append(rep.DecayCurve, DaySample{Day: d, Retention: Retention(*m, now)})
			}
		}
		// 触发到期复习。
		due := make([]string, 0)
		for id, m := range e.mem {
			if m.NextReviewAt <= now {
				due = append(due, id)
			}
		}
		for _, id := range due {
			if _, _, ok := e.Review(id); ok {
				rep.TotalReviews++
			}
		}
	}
	// 汇总每条记忆最终状态。
	sumRet := 0.0
	now := fc.Now().UnixMilli()
	for id, m := range e.mem {
		rep.Memories = append(rep.Memories, MemorySim{
			ID:             id,
			Reviews:        m.ReviewCount,
			FinalStability: m.Stability,
			FinalRetention: Retention(*m, now),
		})
		sumRet += Retention(*m, now)
	}
	if len(e.mem) > 0 {
		rep.FinalMeanRet = sumRet / float64(len(e.mem))
	}
	return rep
}
