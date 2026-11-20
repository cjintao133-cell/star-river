package discovery

import (
	"context"
	"errors"
	"testing"

	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

func TestMemoryStorePutGet(t *testing.T) {
	s := NewMemoryStore()
	key := ChannelKey{BaseURL: "https://api.x/v1", ModelID: "gpt-4o"}
	if _, ok := s.Get(key); ok {
		t.Error("空库不应有该键")
	}
	s.Put(Capability{Key: key, Available: true, Caps: types.ModelCapabilities{ModelID: "gpt-4o", Streaming: true}})
	got, ok := s.Get(key)
	if !ok || !got.Available || !got.Caps.Streaming {
		t.Errorf("Get = %+v, ok=%v", got, ok)
	}
}

func TestLazyValidateSuccess(t *testing.T) {
	s := NewMemoryStore()
	key := ChannelKey{BaseURL: "https://api.x/v1", ModelID: "claude-3-5-sonnet"}
	var gotModel string
	prober := ProberFunc(func(ctx context.Context, modelID string) (types.ModelCapabilities, error) {
		gotModel = modelID
		return types.ModelCapabilities{Streaming: true, Tools: true, Vision: true, Reasoning: false}, nil
	})
	cap := LazyValidate(context.Background(), s, key, prober)
	if gotModel != "claude-3-5-sonnet" {
		t.Errorf("prober 收到 modelID = %q", gotModel)
	}
	if !cap.Available {
		t.Error("探测成功应 Available=true")
	}
	if cap.Caps.ModelID != "claude-3-5-sonnet" {
		t.Errorf("Caps.ModelID = %q 应回填", cap.Caps.ModelID)
	}
	if !cap.Caps.Streaming || !cap.Caps.Tools || !cap.Caps.Vision || cap.Caps.Reasoning {
		t.Errorf("能力位 = %+v", cap.Caps)
	}
	if cap.LastError != "" {
		t.Errorf("成功时 LastError 应为空, got %q", cap.LastError)
	}
	stored, ok := s.Get(key)
	if !ok || !stored.Available || !stored.Caps.Tools {
		t.Errorf("落库 = %+v, ok=%v", stored, ok)
	}
}

// 探测失败属预期分支：标记不可用并记原因，绝不静默点亮可用。
func TestLazyValidateFailure(t *testing.T) {
	s := NewMemoryStore()
	key := ChannelKey{BaseURL: "https://api.x/v1", ModelID: "o3-mini"}
	prober := ProberFunc(func(ctx context.Context, modelID string) (types.ModelCapabilities, error) {
		return types.ModelCapabilities{}, errors.New("anthropic: auth status=401: invalid key")
	})
	cap := LazyValidate(context.Background(), s, key, prober)
	if cap.Available {
		t.Error("探测失败不得标记可用")
	}
	if cap.LastError == "" {
		t.Error("失败时须记 LastError")
	}
	if cap.Caps.ModelID != "o3-mini" {
		t.Errorf("失败时 Caps.ModelID 也应回填供展示, got %q", cap.Caps.ModelID)
	}
	stored, ok := s.Get(key)
	if !ok || stored.Available {
		t.Errorf("落库应为不可用, got %+v ok=%v", stored, ok)
	}
}

// 同一 model 不同 base_url = 独立渠道，台账分别记录（增补 P2）。
func TestChannelKeyIndependence(t *testing.T) {
	s := NewMemoryStore()
	k1 := ChannelKey{BaseURL: "https://a/v1", ModelID: "gpt-4o"}
	k2 := ChannelKey{BaseURL: "https://b/v1", ModelID: "gpt-4o"}
	ok1 := ProberFunc(func(ctx context.Context, m string) (types.ModelCapabilities, error) {
		return types.ModelCapabilities{Streaming: true}, nil
	})
	fail2 := ProberFunc(func(ctx context.Context, m string) (types.ModelCapabilities, error) {
		return types.ModelCapabilities{}, errors.New("no stream support")
	})
	c1 := LazyValidate(context.Background(), s, k1, ok1)
	c2 := LazyValidate(context.Background(), s, k2, fail2)
	if !c1.Available || c2.Available {
		t.Errorf("不同渠道应独立: c1.avail=%v c2.avail=%v", c1.Available, c2.Available)
	}
	g1, _ := s.Get(k1)
	g2, _ := s.Get(k2)
	if !g1.Available || g2.Available {
		t.Errorf("落库应独立: g1=%+v g2=%+v", g1, g2)
	}
}
