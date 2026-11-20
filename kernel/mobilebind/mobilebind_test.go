package mobilebind

import "testing"

func TestVersion(t *testing.T) {
	got := Version()
	want := "star-river-kernel/0.0.1-m0"
	if got != want {
		t.Fatalf("Version() = %q, want %q", got, want)
	}
}

func TestEcho(t *testing.T) {
	cases := []string{
		"",
		"hello",
		"中文与emoji🌌混排",
		"line1\nline2\ttab",
	}
	for _, in := range cases {
		if got := Echo(in); got != in {
			t.Fatalf("Echo(%q) = %q, want round-trip", in, got)
		}
	}
}
