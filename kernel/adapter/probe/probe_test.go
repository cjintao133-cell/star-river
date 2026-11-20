package probe

import (
	"context"
	"errors"
	"testing"
)

func TestCaps(t *testing.T) {
	c := Caps("m", Signal{Streamed: true, AcceptedImage: true, AcceptedTools: true, EmbeddingOK: true, SawReasoning: true})
	if !c.Streaming || !c.Vision || !c.Tools || !c.Embedding || !c.Reasoning {
		t.Errorf("全信号应全能力位: %+v", c)
	}
	if c.ModelID != "m" {
		t.Errorf("modelID 不对: %s", c.ModelID)
	}
	// 空信号 → 全 false
	c = Caps("x", Signal{})
	if c.Streaming || c.Vision || c.Tools || c.Embedding || c.Reasoning {
		t.Errorf("空信号应全 false: %+v", c)
	}
}

func TestAsProbeFn(t *testing.T) {
	s := SenderFunc(func(ctx context.Context, id string) (Signal, error) {
		return Signal{Streamed: true, AcceptedTools: true}, nil
	})
	fn := AsProbeFn(s)
	caps, err := fn(context.Background(), "m")
	if err != nil || !caps.Streaming || !caps.Tools || caps.Vision {
		t.Errorf("AsProbeFn 不对: %+v %v", caps, err)
	}
	// 错误透传
	sErr := SenderFunc(func(context.Context, string) (Signal, error) { return Signal{}, errors.New("boom") })
	if _, err := AsProbeFn(sErr)(context.Background(), "m"); err == nil {
		t.Error("错误应透传")
	}
}
