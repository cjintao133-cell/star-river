// stresssim：全面超高压模拟 + 年尺度（假时钟）推演。全本地、零 API 花费。
// 运行：go -C /root/star-river run ./cmd/stresssim
// 覆盖：记忆年测 / 情绪年测 / 主动年测 / 向量批量 / 超高压并发(mood+memory+bus+vector) / goroutine+RSS 采样。
package main

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"star-river/kernel/bus"
	"star-river/kernel/core/clock"
	"star-river/kernel/core/types"
	"star-river/kernel/endocrine"
	"star-river/kernel/hippocampus"
	"star-river/kernel/native/vector"
	"star-river/kernel/proactive"
)

func rssKB() int {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			f := strings.Fields(line)
			if len(f) >= 2 {
				n, _ := strconv.Atoi(f[1])
				return n
			}
		}
	}
	return 0
}

func memStats() (goroutines int, rss int) { return runtime.NumGoroutine(), rssKB() }

func hr(label string) { fmt.Printf("\n=== %s ===\n", label) }

func main() {
	start := time.Now()
	g0, m0 := memStats()
	fmt.Printf("[baseline] goroutines=%d rss=%dKB\n", g0, m0)

	// —— ① 记忆年测（假时钟 365 天）——
	hr("① 记忆年测（365 天衰减+复习）")
	fc := hippocampus.NewFakeClock(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	mem := hippocampus.New(hippocampus.Options{Clock: fc})
	for i := 0; i < 10; i++ {
		mem.Encode(fmt.Sprintf("m%d", i), fmt.Sprintf("记忆内容 %d 关于用户的日常", i), 0.5+float64(i%5)*0.1, 0.4)
	}
	rep := hippocampus.RunMonth(mem, fc, 365)
	fmt.Printf("  记忆数=%d 总复习=%d 终态均值留存=%.3f 采样点=%d\n", mem.Len(), rep.TotalReviews, rep.FinalMeanRet, len(rep.DecayCurve))
	if mem.Len() != 10 {
		fmt.Printf("  ⚠ 记忆数应恒定 10，实际 %d\n", mem.Len())
	}
	if math.IsNaN(rep.FinalMeanRet) || rep.FinalMeanRet < 0 || rep.FinalMeanRet > 1 {
		fmt.Printf("  ⚠ 留存越界/NaN: %v\n", rep.FinalMeanRet)
	}

	// —— ② 情绪年测（假时钟 365 天 tick+interact）——
	hr("② 情绪年测（365 天演化）")
	ec := clock.NewFakeClock(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	mood := endocrine.New(endocrine.DefaultRegistry(), ec)
	bad := 0
	for d := 0; d < 365; d++ {
		ec.Advance(24 * time.Hour)
		mood.Interact(map[string]float64{endocrine.KeyIntimacy: 0.05, endocrine.KeyFatigue: 0.3, endocrine.KeyCuriosity: 0.1})
		mood.Tick()
		for k, v := range mood.State() {
			if math.IsNaN(v) || math.IsInf(v, 0) || v < -1.0001 || v > 1.0001 {
				bad++
				_ = k
			}
		}
	}
	st := mood.State()
	fmt.Printf("  365 天后状态: valence=%.2f arousal=%.2f intimacy=%.2f fatigue=%.2f curiosity=%.2f\n",
		st["valence"], st["arousal"], st["intimacy"], st["fatigue"], st["curiosity"])
	if bad > 0 {
		fmt.Printf("  ⚠ 情绪越界次数=%d\n", bad)
	} else {
		fmt.Println("  ✓ 全程状态在 [-1,1] 内、无 NaN/Inf")
	}

	// —— ③ 主动年测（365 天三类触发源）——
	hr("③ 主动年测（365 天，记忆到期+日程+情绪）")
	pc := clock.NewFakeClock(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	sched := proactive.New(
		proactive.Config{Enabled: true, DefaultCooldown: time.Hour, Rate: proactive.RateLimit{MaxPerWindow: 100, Window: time.Hour, DailyMax: 1000}, Quiet: proactive.QuietHours{Enabled: false}},
		pc, proactive.MemoryReviewSource{}, proactive.ScheduleSource{Entries: proactive.DefaultScheduleEntries()}, proactive.EmotionSource{Rules: proactive.DefaultEmotionRules()})
	reviews := []proactive.ReviewItem{{ID: "r0", NextReviewAt: 0}, {ID: "r1", NextReviewAt: 0}, {ID: "r2", NextReviewAt: 0}}
	emo := proactive.EmotionState{"fatigue": 0.9}
	total, maxFired := 0, 0
	for d := 1; d <= 365; d++ {
		pc.Advance(24 * time.Hour)
		total += len(sched.Tick(reviews, emo))
		if sn := sched.Snapshot(); len(sn.Fired) > maxFired {
			maxFired = len(sn.Fired)
		}
	}
	fmt.Printf("  全年主动提醒=%d 内部fired峰值=%d（应远小于365×触发数=有界去重/限频）\n", total, maxFired)

	// —— ④ 向量批量（2万条插入+检索）——
	hr("④ 向量批量（20000 条 int8，插入+检索）")
	dim := 16
	ix, err := vector.NewIndex(dim)
	if err != nil {
		fmt.Printf("  ⚠ NewIndex: %v\n", err)
	} else {
		tIns := time.Now()
		for i := 0; i < 20000; i++ {
			v := make([]int8, dim)
			for j := range v {
				v[j] = int8((i*7 + j*13) % 127)
			}
			_ = ix.Insert(int64(i), v)
		}
		insMs := time.Since(tIns).Milliseconds()
		q := make([]int8, dim)
		for j := range q {
			q[j] = int8(j * 3)
		}
		tq := time.Now()
		for i := 0; i < 200; i++ {
			_, _ = ix.Search(q, 5)
		}
		fmt.Printf("  插入 20000 条 %dms（%.0f 条/ms）· 200 次检索 %dms · Len=%d\n", insMs, 20000/float64(max64(insMs, 1)), time.Since(tq).Milliseconds(), ix.Len())
		_ = ix.Close()
	}

	// —— ⑤ 超高压并发（mood+memory+bus+vector 混合）——
	hr("⑤ 超高压并发（64 goroutine 混合压测）")
	busS := bus.New(256)
	sub, _ := busS.Subscribe()
	var subCount int64
	var subMu sync.Mutex
	go func() {
		for range sub.Events() {
			subMu.Lock()
			subCount++
			subMu.Unlock()
		}
	}()
	concMood := endocrine.New(endocrine.DefaultRegistry(), clock.RealClock{})
	concMem := hippocampus.New(hippocampus.Options{})
	var memMu sync.Mutex
	concVec, _ := vector.NewIndex(8)

	workers := 64
	iters := 3000
	if os.Getenv("STRESS_QUICK") != "" {
		workers = 16
		iters = 200
	}
	var wg sync.WaitGroup
	pubOK := int64(0)
	var cntMu sync.Mutex
	gbefore, mbefore := memStats()
	tConc := time.Now()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				concMood.Interact(map[string]float64{endocrine.KeyValence: 0.01})
				_ = concMood.State()
				memMu.Lock()
				concMem.Encode(fmt.Sprintf("c%d-%d", id, i), "并发记忆", 0.5, 0.5)
				memMu.Unlock()
				v := make([]int8, 8)
				for j := range v {
					v[j] = int8((id + i + j) % 100)
				}
				_ = concVec.Insert(int64(id*iters+i), v)
				_, _ = concVec.Search(v, 3)
				if busS.Publish(types.Event{Topic: "stress", Source: "sim", Timestamp: time.Now().UnixMilli()}) == nil {
					cntMu.Lock()
					pubOK++
					cntMu.Unlock()
				}
			}
		}(w)
	}
	wg.Wait()
	concMs := time.Since(tConc).Milliseconds()
	gafter, mafter := memStats()
	subMu.Lock()
	gotSub := subCount
	subMu.Unlock()
	fmt.Printf("  耗时 %dms · 操作=%d goroutine × %d = %d 轮\n", concMs, workers, iters, workers*iters)
	fmt.Printf("  bus 发布成功=%d / 订阅收到=%d（订阅者可能滞后，不丢帧看 Dropped）\n", pubOK, gotSub)
	fmt.Printf("  goroutine: 前=%d 后=%d（应回落到基线附近）· RSS: 前=%dKB 后=%dKB（Δ=%dKB）\n", gbefore, gafter, mbefore, mafter, mafter-mbefore)
	fmt.Printf("  记忆写入=%d 条 · vector Len=%d（应=写入数）\n", workers*iters, concVec.Len())
	_ = concVec.Close()

	// —— 汇总 ——
	hr("汇总")
	gEnd, mEnd := memStats()
	fmt.Printf("总耗时 %s · 最终 goroutine=%d · 最终 RSS=%dKB（较基线 Δ=%dKB）\n", time.Since(start).Round(time.Millisecond), gEnd, mEnd, mEnd-m0)
	if mEnd-m0 > 200*1024 {
		fmt.Println("⚠ RSS 增长 >200MB，需排查泄漏")
	} else {
		fmt.Println("✓ RSS 增长在合理范围")
	}
	fmt.Println("STRESSSIM-DONE")
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
