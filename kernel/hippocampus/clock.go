// Package hippocampus 是记忆引擎（伴侣插件包，T-M3-02 遗忘曲线）。
//
// 依赖方向（红线 2）：底座不得 import 本包；本包可 import 底座开源包（core/types、core/clock、native/vector）。
//
// 记忆生命模型（艾宾浩斯遗忘曲线 + 强化）：
//   - 每条记忆有 stability S（稳定度，天）与 lastReviewAt（上次复习时刻）。
//   - 留存度 R(t) = exp(-(now-lastReviewAt)/S)，随未复习时间指数衰减。
//   - 命中（被复习/被召回确认）强化：S' = S * (1 + k*(1-R))，R 越低（越将遗忘）强化越多。
//   - 召回打分 = 语义相似度 × (α+βR) × (γ+importance+emotion_intensity)，低于阈值不进上下文。
//   - 强线索"突然想起"：高 importance/情绪或强线索命中时，即使 R 低也强制进入候选（惊喜回忆）。
//
// 时钟经 clock.Clock 注入（T-M3-06 假时钟贯穿 hippocampus/proactive/endocrine），全逻辑可确定性测试。
package hippocampus

import (
	"time"

	"star-river/kernel/core/clock"
)

// Clock / RealClock / FakeClock 复用内核统一时钟（T-M3-06）。
type (
	Clock     = clock.Clock
	RealClock = clock.RealClock
	FakeClock = clock.FakeClock
)

// NewFakeClock 以给定起点建假时钟（转发 core/clock，保留包内旧 API）。
func NewFakeClock(start time.Time) *FakeClock { return clock.NewFakeClock(start) }
