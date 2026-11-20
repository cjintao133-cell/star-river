// Command hello-kernel is a ~5-second, offline, zero-API-key demo of the kernel's core loop:
// event bus + memory (Ebbinghaus forgetting curve) + emotion (5-D state machine).
//
// Run:
//
//	go run ./examples/hello-kernel
//
// No network, no key, no config. Good for a screenshot/GIF in the README.
package main

import (
	"fmt"
	"time"

	"github.com/cjintao133-cell/star-river/kernel/bus"
	"github.com/cjintao133-cell/star-river/kernel/core/types"
	"github.com/cjintao133-cell/star-river/kernel/endocrine"
	"github.com/cjintao133-cell/star-river/kernel/hippocampus"
)

func main() {
	fmt.Println("== Star River Microkernel · hello-kernel ==")
	fmt.Println()

	// 1) Event bus: publish a doorbell, a subscriber receives it.
	b := bus.New(8)
	sub, _ := b.Subscribe()
	defer b.Close()
	_ = b.Publish(types.Event{Topic: "kernel.started", Source: "hello-kernel"})
	if e := <-sub.Events(); true {
		fmt.Printf("[bus]      received doorbell  topic=%q source=%q\n", e.Topic, e.Source)
	}

	// 2) Memory: encode, then recall with a forgetting curve.
	mem := hippocampus.New(hippocampus.Options{})
	m1 := mem.Encode("m1", "the user is building a local-first AI kernel", 0.9, 0.7)
	m2 := mem.Encode("m2", "buy milk and eggs", 0.2, 0.1)
	fmt.Printf("[memory]   encoded 2 memories\n")
	hits := mem.RecallFromStore(func(m hippocampus.Memory) float64 {
		if m.ID == "m1" {
			return 0.9 // strong semantic match
		}
		return 0.1
	})
	for _, h := range hits {
		fmt.Printf("[memory]   recall   \"%s\"  score=%.2f retention=%.2f\n",
			trunc(h.Memory.Text, 44), h.Score, h.Retention)
	}
	// Reinforce the recalled one; retention stays high.
	mem.Review(m1.ID)
	fmt.Printf("[memory]   review   m1 retention=%.2f (reinforced)\n", mem.Retention(m1.ID))
	_ = m2

	// 3) Emotion: a 5-dimensional state machine; interaction nudges it, then it decays.
	emo := endocrine.New(nil, nil)
	before := emo.State()
	emo.Interact(map[string]float64{"arousal": 0.4, "valence": 0.3})
	after := emo.State()
	fmt.Printf("[emotion]  arousal %.2f -> %.2f   valence %.2f -> %.2f\n",
		before["arousal"], after["arousal"], before["valence"], after["valence"])
	fmt.Printf("[emotion]  state: %v\n", sortedBrief(after))

	fmt.Println()
	fmt.Println("OK — bus + memory + emotion all ran locally, no network, no key.")
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func sortedBrief(m map[string]float64) string {
	keys := []string{"valence", "arousal", "intimacy", "fatigue", "curiosity"}
	out := ""
	for i, k := range keys {
		if i > 0 {
			out += " "
		}
		out += fmt.Sprintf("%s=%.2f", k, m[k])
	}
	_ = time.Now
	return out
}
