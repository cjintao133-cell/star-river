package hippocampus_test

import (
	"fmt"
	"time"

	"github.com/cjintao133-cell/star-river/kernel/hippocampus"
)

// Example demonstrates the two behaviours that make this a human-like memory
// rather than a plain key-value store: meaningful memories decay slowly, and
// reviewing a nearly-forgotten one reinforces it back to full retention.
//
// The fake clock makes the forgetting curve deterministic, so the output below
// is reproducible byte-for-byte.
func Example() {
	clk := hippocampus.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	mem := hippocampus.New(hippocampus.Options{Clock: clk})

	// An important, emotional memory gets high initial stability (S);
	// a trivial one gets a low S and fades much faster.
	mem.Encode("birthday", "user's birthday is March 3", 0.9, 0.8)
	mem.Encode("milk", "buy milk", 0.1, 0.0)

	// Three days pass with no review: retention decays as exp(-t/S).
	clk.Advance(3 * 24 * time.Hour)
	fmt.Printf("after 3 days:  birthday R=%.2f   milk R=%.2f\n",
		mem.Retention("birthday"), mem.Retention("milk"))

	// Reviewing reinforces stability (S' = S*(1+k*(1-R))): retention is restored.
	mem.Review("milk")
	fmt.Printf("after review:  milk R=%.2f\n", mem.Retention("milk"))

	// Output:
	// after 3 days:  birthday R=0.51   milk R=0.08
	// after review:  milk R=1.00
}
