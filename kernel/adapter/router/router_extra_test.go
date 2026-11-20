package router

import (
	"context"
	"errors"
	"testing"

	"github.com/cjintao133-cell/star-river/kernel/bus"
	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

func TestPriceTierString(t *testing.T) {
	cases := []struct {
		tier PriceTier
		want string
	}{
		{TierUnknown, "unknown"},
		{TierCheap, "cheap"},
		{TierStandard, "standard"},
		{TierPremium, "premium"},
	}
	for _, c := range cases {
		if got := c.tier.String(); got != c.want {
			t.Errorf("PriceTier(%d).String() = %q, want %q", c.tier, got, c.want)
		}
	}
}

// WithTopic：自定义主题必须真生效（切换门铃带该主题）。
func TestWithTopic(t *testing.T) {
	b := bus.New(64)
	sub, _ := b.Subscribe()
	defer b.Close()
	fc := newFake(map[string]func(func(types.StreamEvent)) (string, error){
		"p": func(func(types.StreamEvent)) (string, error) { return "", errors.New("p down") },
		"f": func(func(types.StreamEvent)) (string, error) { return "fb", nil },
	})
	r, _ := New([]Endpoint{ep("p", "m1", TierStandard), ep("f", "m2", TierStandard)}, fc,
		WithPublisher(b), WithTopic("custom.route.topic"))
	if _, err := r.Chat(context.Background(), func(types.StreamEvent) {}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	events := drain(t, sub)
	if len(events) != 1 || events[0].Topic != "custom.route.topic" {
		t.Errorf("自定义主题未生效: %+v", events)
	}
}
