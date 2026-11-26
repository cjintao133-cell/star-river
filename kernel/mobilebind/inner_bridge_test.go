package mobilebind

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cjintao133-cell/star-river/kernel/hippocampus"
)

func TestIdentityBridge(t *testing.T) {
	initTestKernel(t)
	var id map[string]any
	if err := json.Unmarshal([]byte(GetIdentity("小星", "助手")), &id); err != nil {
		t.Fatalf("GetIdentity: %v raw=%s", err, GetIdentity("小星", "助手"))
	}
	if id["name"] != "小星" || id["agent_id"] == "" {
		t.Fatalf("身份不对: %+v", id)
	}
	agentID := id["agent_id"].(string)
	// 幂等
	var id2 map[string]any
	_ = json.Unmarshal([]byte(GetIdentity("别的", "x")), &id2)
	if id2["agent_id"] != agentID {
		t.Fatalf("应保持同一身份")
	}
	// Set
	if r := SetIdentity(`{"agent_id":"` + agentID + `","name":"改名","role":"助手"}`); r != "OK" {
		t.Fatalf("SetIdentity: %s", r)
	}
	// 轨迹
	if r := RecordTrajectory("task_created", "开始整理资料", "t1"); r != "OK" {
		t.Fatalf("Record: %s", r)
	}
	if r := RecordTrajectory("step_done", "完成第一步", "t1"); r != "OK" {
		t.Fatalf("Record2: %s", r)
	}
	var evs []map[string]any
	_ = json.Unmarshal([]byte(GetTrajectory(10)), &evs)
	if len(evs) != 2 {
		t.Fatalf("轨迹应 2 条: %+v", evs)
	}
	sum := SummarizeTrajectory(10)
	if !strings.Contains(sum, "开始整理资料") || !strings.Contains(sum, "完成第一步") {
		t.Fatalf("摘要不对: %q", sum)
	}
}

func TestHygieneAnalyzeBridge(t *testing.T) {
	initTestKernel(t)
	dayMs := int64(24 * 60 * 60 * 1000)
	now := int64(100 * dayMs)
	mems := []hippocampus.Memory{
		{ID: "a", Text: "废信息", Importance: 0.1, Stability: 1, LastReviewAt: now - 190*dayMs},
		{ID: "b", Text: "关键信息", Importance: 0.9, Stability: 1, LastReviewAt: now - 190*dayMs},
		{ID: "c", Text: "关键信息", Importance: 0.5, Stability: 1, LastReviewAt: now},
	}
	mj, _ := json.Marshal(mems)
	var rep map[string]any
	if err := json.Unmarshal([]byte(HygieneAnalyze(string(mj), "", now)), &rep); err != nil {
		t.Fatalf("analyze: %v raw=%s", err, HygieneAnalyze(string(mj), "", now))
	}
	prune := rep["prune"].(map[string]any)
	if int(prune["dropped"].(float64)) != 1 {
		t.Fatalf("应丢 1 条: %+v", prune)
	}
	dups := rep["duplicates"].([]any)
	if len(dups) != 1 {
		t.Fatalf("应 1 组重复: %+v", dups)
	}
	// 坏 json
	if r := HygieneAnalyze("{bad", "", now); !strings.HasPrefix(r, "ERR") {
		t.Fatalf("坏 json 应 ERR")
	}
}

func TestCronNextBridge(t *testing.T) {
	initTestKernel(t)
	// 2026-01-01 08:07 UTC = ms
	base := int64(1767254820000) // approx; just need monotonic check
	var r map[string]any
	out := CronNext("*/15 * * * *", base)
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("CronNext: %v raw=%s", err, out)
	}
	if r["next_ms"].(float64) <= float64(base) {
		t.Fatalf("下次应晚于基准: %+v", r)
	}
	if !strings.HasPrefix(CronNext("bad expr", 0), "ERR") {
		t.Fatalf("坏表达式应 ERR")
	}
}

func TestHooksBridgeAndMemoryHook(t *testing.T) {
	initTestKernel(t)
	hooksReg.Clear()
	t.Cleanup(func() { hooksReg.Clear() })

	hit := 0
	hooksReg.Register("demo", 1, func(p map[string]any) (any, bool) { hit++; return "ok", false })
	out := HooksFire("demo", `{"x":1}`)
	if !strings.Contains(out, "ok") || hit != 1 {
		t.Fatalf("HooksFire 不对: %s hit=%d", out, hit)
	}
	if HooksCount("demo") != 1 {
		t.Fatalf("count 不对")
	}
	// abort
	hooksReg.Register("block", 1, func(p map[string]any) (any, bool) { return nil, true })
	if out := HooksFire("block", ""); !strings.Contains(out, `"aborted":true`) {
		t.Fatalf("应 aborted: %s", out)
	}
	HooksClear()
	if HooksCount("demo") != 0 {
		t.Fatalf("Clear 失效")
	}

	// 记忆写入触发 on_memory_write
	memHit := 0
	hooksReg.Register("on_memory_write", 1, func(p map[string]any) (any, bool) { memHit++; return nil, false })
	_ = AddMemory("semantic", "测试记忆内容", 0.5)
	if memHit != 1 {
		t.Fatalf("AddMemory 应触发 on_memory_write, got %d", memHit)
	}
}
