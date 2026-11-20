package discovery

import (
	"context"
	"errors"
	"testing"
)

func TestDiscoverSuccess(t *testing.T) {
	l := ListerFunc(func(ctx context.Context) ([]RawModel, error) {
		return []RawModel{
			{ID: "gpt-4o", Provider: "openai"},
			{ID: "gemini-1.5-flash", Provider: "gemini"},
			{ID: "acme-reasoner", Provider: "custom"},
			{ID: "mystery-model", Provider: "custom"},
		}, nil
	})
	cat, err := Discover(context.Background(), l)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if cat.Degraded {
		t.Error("成功发现不应 Degraded")
	}
	if len(cat.Models) != 4 {
		t.Fatalf("models = %d, want 4", len(cat.Models))
	}
	want := []struct {
		id       string
		cat      Category
		provider string
	}{
		{"gpt-4o", CategoryFlagship, "openai"},
		{"gemini-1.5-flash", CategoryFast, "gemini"},
		{"acme-reasoner", CategoryReasoning, "custom"},
		{"mystery-model", CategoryUnknown, "custom"},
	}
	for i, w := range want {
		if cat.Models[i].ModelID != w.id || cat.Models[i].Category != w.cat || cat.Models[i].Provider != w.provider {
			t.Errorf("models[%d] = %+v, want %+v", i, cat.Models[i], w)
		}
	}
}

// /models 不可用 → 降级：Degraded=true，给出内置候选，返回原始错误。
func TestDiscoverDegraded(t *testing.T) {
	boom := errors.New("models endpoint 500")
	l := ListerFunc(func(ctx context.Context) ([]RawModel, error) {
		return nil, boom
	})
	cat, err := Discover(context.Background(), l)
	if err == nil {
		t.Fatal("降级时应返回原始错误")
	}
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
	if !cat.Degraded {
		t.Error("应 Degraded=true")
	}
	if len(cat.Models) != 0 {
		t.Errorf("降级时 Models 应为空, got %d", len(cat.Models))
	}
	if len(cat.CandidateIDs) == 0 {
		t.Error("降级时应给内置候选供手动选择")
	}
}

func TestDiscoverEmpty(t *testing.T) {
	l := ListerFunc(func(ctx context.Context) ([]RawModel, error) { return []RawModel{}, nil })
	cat, err := Discover(context.Background(), l)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if cat.Degraded || cat.Models == nil || len(cat.Models) != 0 {
		t.Errorf("空发现应为非 nil 空切片且非降级: %+v", cat)
	}
}
