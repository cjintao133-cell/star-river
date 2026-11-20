package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/cjintao133-cell/star-river/kernel/adapter/router"
	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

// T-M3-07 闭环：StatsRepo 写入真统计 → 适配 StatsProvider → router 按统计排序/标灰。
func TestStatsToRouterClosedLoop(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "loop.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	repo, _ := NewStatsRepo(st)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		repo.Record(ctx, "m-good", true, 100, 10, 5)
	}
	for i := 0; i < 5; i++ {
		repo.Record(ctx, "m-bad", false, 100, 10, 0)
	}

	provider := router.StatsProviderFunc(func(model string) (float64, bool) {
		rate, known, _ := repo.FailureRate(ctx, model)
		return rate, known
	})
	caller := router.CallerFunc(func(context.Context, router.Endpoint, func(types.StreamEvent)) (string, error) {
		return "", nil
	})
	ep := func(name, model string) router.Endpoint {
		return router.Endpoint{Name: name, BaseURL: "x", APIKey: "k", Model: model, Tier: router.TierStandard}
	}

	r, err := router.New([]router.Endpoint{
		ep("primary", "m-primary"),
		ep("bad", "m-bad"),
		ep("good", "m-good"),
	}, caller, router.WithStats(provider, 0))
	if err != nil {
		t.Fatal(err)
	}
	plan := r.Plan()
	if len(plan) != 3 || plan[1].Name != "good" || plan[2].Name != "bad" {
		t.Errorf("应按失败率升序 good->bad: %+v", plan)
	}

	r2, _ := router.New([]router.Endpoint{
		ep("primary", "m-primary"),
		ep("bad", "m-bad"),
		ep("good", "m-good"),
	}, caller, router.WithStats(provider, 0.5))
	for _, e := range r2.Plan() {
		if e.Name == "bad" {
			t.Errorf("m-bad 失败率 1.0>0.5 应标灰: %+v", r2.Plan())
		}
	}
}
