package discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cjintao133-cell/star-river/kernel/adapter/openai"
)

// 集成：真实 openai.Client.Models() 打到 httptest 的 /models，经 Lister 适配进 discovery，
// 验证"发现→三层分类"整条缝跑通（M2/M3 集成时用同样几行映射）。
func TestDiscoverWithRealOpenAIModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[
            {"id":"gpt-4o","owned_by":"openai"},
            {"id":"gpt-4o-mini","owned_by":"openai"},
            {"id":"o3-mini","owned_by":"openai"},
            {"id":"some-mystery-model","owned_by":"x"}
        ]}`))
	}))
	defer srv.Close()

	c := openai.New(srv.URL+"/v1", "sk-test-key")
	lister := ListerFunc(func(ctx context.Context) ([]RawModel, error) {
		ms, err := c.Models(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]RawModel, 0, len(ms))
		for _, m := range ms {
			out = append(out, RawModel{ID: m.ID, Provider: "openai"})
		}
		return out, nil
	})

	cat, err := Discover(context.Background(), lister)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if cat.Degraded || len(cat.Models) != 4 {
		t.Fatalf("catalog = %+v", cat)
	}
	want := map[string]Category{
		"gpt-4o":             CategoryFlagship,
		"gpt-4o-mini":        CategoryFast,
		"o3-mini":            CategoryReasoning,
		"some-mystery-model": CategoryUnknown,
	}
	for _, m := range cat.Models {
		if m.Category != want[m.ModelID] {
			t.Errorf("%s => %s, want %s", m.ModelID, m.Category, want[m.ModelID])
		}
		if m.Provider != "openai" {
			t.Errorf("%s provider = %q", m.ModelID, m.Provider)
		}
	}
}
