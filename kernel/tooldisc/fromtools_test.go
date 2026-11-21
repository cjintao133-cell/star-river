package tooldisc

import (
	"testing"

	"github.com/cjintao133-cell/star-river/kernel/tool"
)

func TestFromTools(t *testing.T) {
	if got := FromTools(nil); len(got) != 0 {
		t.Fatalf("FromTools(nil) = %d items, want 0", len(got))
	}
	in := []tool.Tool{
		{Name: "a", Description: "desc a"},
		{Name: "b"},
	}
	got := FromTools(in)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	if got[0].Tool.Name != "a" || got[0].Category != "" || got[1].Tool.Name != "b" {
		t.Fatalf("unexpected: %+v", got)
	}
}
