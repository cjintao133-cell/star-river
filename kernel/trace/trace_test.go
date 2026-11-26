package trace

import (
	"encoding/hex"
	"strings"
	"testing"
)

type fakeRec struct {
	started  []Span
	finished []Span
}

func (f *fakeRec) StartSpan(s Span) error { f.started = append(f.started, s); return nil }
func (f *fakeRec) FinishSpan(traceID, spanID string, endMs int64, status string, attrs map[string]any) error {
	f.finished = append(f.finished, Span{TraceID: traceID, SpanID: spanID, EndMs: endMs, Status: status, Attrs: attrs})
	return nil
}

func TestNewIDHex(t *testing.T) {
	id := NewID()
	if len(id) != 32 {
		t.Fatalf("ID 应为 32 hex, got %q", id)
	}
	if _, err := hex.DecodeString(id); err != nil {
		t.Fatalf("非合法 hex: %v", err)
	}
	a, b := NewID(), NewID()
	if a == b {
		t.Fatal("ID 应不重复")
	}
}

func TestBeginEnd(t *testing.T) {
	rec := &fakeRec{}
	seq := 0
	tick := int64(1000)
	tr := NewTracerWith(rec, func() string { seq++; return "id" + string(rune('0'+seq)) }, func() int64 { tick += 10; return tick })

	s, err := tr.Begin("", "", "llm", "chat", "s1")
	if err != nil {
		t.Fatal(err)
	}
	if s.TraceID == "" || s.SpanID == "" || s.Status != "running" {
		t.Fatalf("span 初值不对: %+v", s)
	}
	if len(rec.started) != 1 {
		t.Fatalf("应落 1 条 started")
	}
	if err := tr.End(s, "ok", map[string]any{"tokens": 5}); err != nil {
		t.Fatal(err)
	}
	if len(rec.finished) != 1 {
		t.Fatalf("应落 1 条 finished")
	}
	f := rec.finished[0]
	if f.TraceID != s.TraceID || f.SpanID != s.SpanID || f.Status != "ok" {
		t.Fatalf("finish 不对: %+v", f)
	}
	if f.EndMs < s.StartMs {
		t.Fatalf("end<start: %d<%d", f.EndMs, s.StartMs)
	}
}

func TestBeginJoinsParentTrace(t *testing.T) {
	rec := &fakeRec{}
	tr := NewTracerWith(rec, func() string { return "x" }, func() int64 { return 1 })
	s, _ := tr.Begin("TRACE-A", "PARENT", "tool", "search", "")
	if s.TraceID != "TRACE-A" || s.ParentID != "PARENT" {
		t.Fatalf("应并入父 trace: %+v", s)
	}
}

func TestEndDefaultStatus(t *testing.T) {
	rec := &fakeRec{}
	tr := NewTracerWith(rec, nil, nil)
	s, _ := tr.Begin("", "", "memory", "recall", "")
	_ = tr.End(s, "", nil)
	if rec.finished[0].Status != "ok" {
		t.Fatalf("空状态应默认 ok, got %q", rec.finished[0].Status)
	}
}

func TestBeginNoRecorder(t *testing.T) {
	tr := NewTracerWith(nil, nil, nil)
	s, err := tr.Begin("", "", "k", "n", "")
	if err != nil || s.SpanID == "" {
		t.Fatalf("nil recorder 下 Begin 应可返回 span: %v %+v", err, s)
	}
	if !strings.Contains(s.Kind, "k") {
		t.Fatalf("kind 保留")
	}
}
