package store

import (
	"context"
	"path/filepath"
	"testing"
)

func newStatsRepo(t *testing.T) (*StatsRepo, *Store) {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "stats.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	r, err := NewStatsRepo(st)
	if err != nil {
		t.Fatal(err)
	}
	return r, st
}

func TestStatsRecordAggregate(t *testing.T) {
	r, _ := newStatsRepo(t)
	ctx := context.Background()
	// 3 成功 1 失败。
	r.Record(ctx, "m1", true, 100, 10, 5)
	r.Record(ctx, "m1", true, 200, 20, 10)
	r.Record(ctx, "m1", true, 300, 30, 15)
	r.Record(ctx, "m1", false, 400, 40, 0)

	s, ok, err := r.Get(ctx, "m1")
	if err != nil || !ok {
		t.Fatalf("Get: %v ok=%v", err, ok)
	}
	if s.Requests != 4 || s.Success != 3 || s.Fail != 1 {
		t.Errorf("counts = %+v", s)
	}
	if s.PromptTokens != 100 || s.CompletionTokens != 30 {
		t.Errorf("tokens = %+v", s)
	}
	if !approxEq(s.FailureRate(), 0.25) {
		t.Errorf("failure rate = %v", s.FailureRate())
	}
	if !approxEq(s.AvgLatencyMs(), 250) {
		t.Errorf("avg latency = %v", s.AvgLatencyMs())
	}
	if s.LastLatencyMs != 400 {
		t.Errorf("last latency = %d, 应最近一次 400", s.LastLatencyMs)
	}
}

func TestStatsEmptyAndMissing(t *testing.T) {
	r, _ := newStatsRepo(t)
	ctx := context.Background()
	if _, ok, _ := r.Get(ctx, "nope"); ok {
		t.Error("不存在应 ok=false")
	}
	rate, known, _ := r.FailureRate(ctx, "nope")
	if known || rate != 0 {
		t.Errorf("无记录 rate=%v known=%v", rate, known)
	}
	if err := r.Record(ctx, "", true, 1, 1, 1); err == nil {
		t.Error("空 model id 应报错")
	}
}

func TestStatsAll(t *testing.T) {
	r, _ := newStatsRepo(t)
	ctx := context.Background()
	r.Record(ctx, "b", true, 1, 1, 1)
	r.Record(ctx, "a", false, 1, 1, 1)
	all, err := r.All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].ModelID != "a" || all[1].ModelID != "b" {
		t.Errorf("all = %+v", all)
	}
}

func approxEq(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}

func TestStatsZeroValueRates(t *testing.T) {
	var s ModelStats
	if s.FailureRate() != 0 {
		t.Error("空统计失败率应 0")
	}
	if s.AvgLatencyMs() != 0 {
		t.Error("空统计平均延迟应 0")
	}
}

func TestNewStatsRepoNil(t *testing.T) {
	if _, err := NewStatsRepo(nil); err == nil {
		t.Error("nil store 应报错")
	}
}

func TestStatsRecordNegativeClamp(t *testing.T) {
	r, _ := newStatsRepo(t)
	ctx := context.Background()
	if err := r.Record(ctx, "m", true, -5, -10, -3); err != nil {
		t.Fatal(err)
	}
	s, _, _ := r.Get(ctx, "m")
	if s.TotalLatencyMs != 0 || s.PromptTokens != 0 || s.CompletionTokens != 0 {
		t.Errorf("负值应夹 0: %+v", s)
	}
}
