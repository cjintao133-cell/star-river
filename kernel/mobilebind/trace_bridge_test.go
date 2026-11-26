package mobilebind

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTraceBridgeLifecycle(t *testing.T) {
	initTestKernel(t)

	var root map[string]string
	if err := json.Unmarshal([]byte(TraceStart("", "", "llm", "chat", "s1")), &root); err != nil {
		t.Fatalf("TraceStart: %v raw=%s", err, TraceStart("", "", "llm", "chat", "s1"))
	}
	tid, sid := root["trace_id"], root["span_id"]
	if tid == "" || sid == "" {
		t.Fatalf("缺 trace/span id: %+v", root)
	}
	if r := TraceEnd(tid, sid, "ok", `{"tokens":10}`); r != "OK" {
		t.Fatalf("TraceEnd: %s", r)
	}

	// 子 span 并入同一 trace
	var child map[string]string
	_ = json.Unmarshal([]byte(TraceStart(tid, sid, "tool", "search", "s1")), &child)
	if child["trace_id"] != tid {
		t.Errorf("子 span 应并入同 trace: %+v", child)
	}
	_ = TraceEnd(child["trace_id"], child["span_id"], "ok", "")

	// 列表含该 trace 的 span（至少根 span 状态 ok）
	var list []traceRow
	if err := json.Unmarshal([]byte(ListTraces(50)), &list); err != nil {
		t.Fatalf("ListTraces json: %v", err)
	}
	found := false
	for _, r := range list {
		if r.TraceID == tid && r.SpanID == sid && r.Status == "ok" {
			found = true
		}
	}
	if !found {
		t.Errorf("列表未找到根 span: %+v", list)
	}

	// 整条 trace 有 2 个 span
	var spans []traceRow
	if err := json.Unmarshal([]byte(GetTrace(tid)), &spans); err != nil {
		t.Fatalf("GetTrace json: %v", err)
	}
	if len(spans) != 2 {
		t.Fatalf("期望 2 个 span, got %d", len(spans))
	}
	hasAttrs := false
	for _, s := range spans {
		if strings.Contains(s.AttrsJSON, "tokens") {
			hasAttrs = true
		}
	}
	if !hasAttrs {
		t.Errorf("根 span 应保留 attrs: %+v", spans)
	}
}

func TestTraceBadAttrsAndClear(t *testing.T) {
	initTestKernel(t)
	var root map[string]string
	_ = json.Unmarshal([]byte(TraceStart("", "", "memory", "recall", "")), &root)
	if r := TraceEnd(root["trace_id"], root["span_id"], "ok", `{not json`); !strings.HasPrefix(r, "ERR") {
		t.Errorf("坏 attrs 应 ERR: %q", r)
	}
	if r := ClearTraces(); r != "OK" {
		t.Fatalf("ClearTraces: %s", r)
	}
	var list []traceRow
	_ = json.Unmarshal([]byte(ListTraces(10)), &list)
	if len(list) != 0 {
		t.Errorf("清空后应为空: %+v", list)
	}
}
