package identity

import (
	"testing"
)

func TestEnsureAndGet(t *testing.T) {
	tr := New(NewMemStore())
	if _, err := tr.Get(); err == nil {
		t.Fatal("初始应无身份")
	}
	id, err := tr.Ensure("小星", "助手")
	if err != nil || id.Name != "小星" || id.AgentID == "" {
		t.Fatalf("Ensure 不对: %+v %v", id, err)
	}
	// 再次 Ensure 应返回同一身份
	id2, _ := tr.Ensure("other", "x")
	if id2.AgentID != id.AgentID {
		t.Fatalf("Ensure 应幂等: %s vs %s", id2.AgentID, id.AgentID)
	}
}

func TestSetAndRecordSummarize(t *testing.T) {
	seq := int64(1000)
	tr := New(NewMemStore(), WithClock(func() int64 { seq += 10; return seq }))
	_ = tr.Set(Identity{Name: "小星", Role: "助手"})
	_ = tr.Record(Event{Kind: "task_created", Summary: "开始整理资料"})
	_ = tr.Record(Event{Kind: "step_done", Summary: "完成第1步"})
	evs, err := tr.Recent(10)
	if err != nil || len(evs) != 2 {
		t.Fatalf("Recent 不对: %+v %v", evs, err)
	}
	// 正序
	if evs[0].Kind != "task_created" {
		t.Fatalf("应按时间正序: %+v", evs)
	}
	sum, _ := tr.Summarize(10)
	if sum == "" || !contains(sum, "开始整理资料") || !contains(sum, "完成第1步") {
		t.Fatalf("Summary 不对: %q", sum)
	}
}

func TestRecentLimit(t *testing.T) {
	tr := New(NewMemStore(), WithClock(func() int64 { return 1 }))
	for i := 0; i < 5; i++ {
		_ = tr.Record(Event{Kind: "note", Summary: "n"})
	}
	evs, _ := tr.Recent(2)
	if len(evs) != 2 {
		t.Fatalf("limit 失效: %d", len(evs))
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
