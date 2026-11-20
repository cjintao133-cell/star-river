package mobilebind

// 主动能力桥（proactive，第三个插件）：把 kernel/proactive 调度器接进 App。
// 触发源：记忆复习到期(hippocampus) + 人设日程 + 情绪驱动(endocrine)，全部由装配层喂真实数据。
// 出厂默认关机（可撤回权限）；状态持久化到 app_meta。
// 红线：入口层 import 插件，开源区不依赖（check-open-split 守卫）。

import (
	"context"
	"encoding/json"
	"sync"

	"star-river/kernel/core/clock"
	"star-river/kernel/hippocampus"
	"star-river/kernel/proactive"
)

const metaProactiveKey = "proactive.snapshot.v1"

var (
	proMu    sync.Mutex
	proSched *proactive.Scheduler
	proNudge []proactive.Nudge // 最近一次 Tick 产生的 nudges（供 UI 拉取）
)

// proBaseCfg 主动能力基配置（测试可覆盖，如关静默以便确定性断言）。
var proBaseCfg = proactive.DefaultConfig()

func ensureProactive() *proactive.Scheduler {
	if proSched != nil {
		return proSched
	}
	cfg := proBaseCfg
	cfg.Enabled = false // 出厂默认关（可撤回权限）
	proSched = proactive.New(cfg, clock.RealClock{}, proactive.MemoryReviewSource{}, proactive.ScheduleSource{Entries: proactive.DefaultScheduleEntries()}, proactive.EmotionSource{Rules: proactive.DefaultEmotionRules()})
	proSched.SetEmitter(proactive.EmitterFunc(func(proactive.Nudge) {})) // Tick 已返回结果，emitter 空实现
	if st := storeRef(); st != nil {
		if v := metaGet(st, metaProactiveKey); v != "" {
			var sn proactive.Snapshot
			if json.Unmarshal([]byte(v), &sn) == nil {
				proSched.Restore(sn)
			}
		}
	}
	return proSched
}

func resetProactive() {
	proMu.Lock()
	proSched = nil
	proNudge = nil
	proMu.Unlock()
}

func persistProactiveLocked() {
	if st := storeRef(); st != nil && proSched != nil {
		if b, err := json.Marshal(proSched.Snapshot()); err == nil {
			metaSet(st, metaProactiveKey, string(b))
		}
	}
}

// ProactiveStatus 返回 {enabled, sources}。
func ProactiveStatus() string {
	proMu.Lock()
	s := ensureProactive()
	out := map[string]any{
		"enabled": s.Enabled(),
		"sources": map[string]bool{
			string(proactive.SourceMemoryReview): s.SourceEnabled(proactive.SourceMemoryReview),
			string(proactive.SourceSchedule):     s.SourceEnabled(proactive.SourceSchedule),
			string(proactive.SourceEmotion):      s.SourceEnabled(proactive.SourceEmotion),
		},
	}
	proMu.Unlock()
	b, _ := json.Marshal(out)
	return string(b)
}

// ProactiveSetEnabled 总开关（默认关→用户显式打开）。
func ProactiveSetEnabled(v bool) string {
	proMu.Lock()
	s := ensureProactive()
	s.SetEnabled(v)
	persistProactiveLocked()
	proMu.Unlock()
	return "OK"
}

// ProactiveSetSource 开关单触发源：memory_review | schedule | emotion。
func ProactiveSetSource(kind string, v bool) string {
	proMu.Lock()
	s := ensureProactive()
	switch proactive.SourceKind(kind) {
	case proactive.SourceMemoryReview, proactive.SourceSchedule, proactive.SourceEmotion:
		s.SetSourceEnabled(proactive.SourceKind(kind), v)
	default:
		proMu.Unlock()
		return "ERR:unknown source: " + kind
	}
	persistProactiveLocked()
	proMu.Unlock()
	return "OK"
}

// ProactiveTick 评估一次（喂真实记忆复习 + 情绪），返回本轮触发的 nudges JSON。
func ProactiveTick() string {
	// 从装配层取真实数据（锁顺序：assembly 后 proactive）。
	asmMu.Lock()
	ensureAssembly()
	var reviews []proactive.ReviewItem
	for _, sc := range asmMem.RecallFromStore(func(h hippocampus.Memory) float64 { return 1.0 }) {
		reviews = append(reviews, proactive.ReviewItem{
			ID: sc.Memory.ID, NextReviewAt: sc.Memory.NextReviewAt, Summary: truncate(sc.Memory.Text, 60),
		})
	}
	emo := proactive.EmotionState(asmMood.State())
	asmMu.Unlock()

	proMu.Lock()
	s := ensureProactive()
	nudges := s.Tick(reviews, emo)
	proNudge = nudges
	persistProactiveLocked()
	proMu.Unlock()

	if nudges == nil {
		nudges = []proactive.Nudge{}
	} else if len(nudges) > 0 {
		_ = BusPublish("proactive.fired", "proactive")
	}
	b, _ := json.Marshal(nudges)
	return string(b)
}

// ProactiveRecent 最近一次 Tick 的 nudges（供 UI 拉取展示/通知）。
func ProactiveRecent() string {
	proMu.Lock()
	n := proNudge
	proMu.Unlock()
	if n == nil {
		n = []proactive.Nudge{}
	}
	b, _ := json.Marshal(n)
	return string(b)
}

var _ = context.Background
