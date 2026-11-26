package hooks

import "testing"

func TestFireOrderAndResult(t *testing.T) {
	r := NewRegistry()
	var seq []string
	r.Register(BeforeTool, 5, func(p map[string]any) (any, bool) { seq = append(seq, "late"); return nil, false })
	r.Register(BeforeTool, 1, func(p map[string]any) (any, bool) { seq = append(seq, "early"); return "ok", false })
	res, aborted := r.Fire(BeforeTool, map[string]any{"tool": "x"})
	if aborted {
		t.Fatal("不应拦截")
	}
	if len(seq) != 2 || seq[0] != "early" || seq[1] != "late" {
		t.Fatalf("优先级顺序不对: %v", seq)
	}
	if len(res) != 2 || res[0].Result != "ok" {
		t.Fatalf("结果收集不对: %+v", res)
	}
}

func TestFireAbortStops(t *testing.T) {
	r := NewRegistry()
	called := 0
	r.Register(BeforeTool, 1, func(p map[string]any) (any, bool) { called++; return "block", true })
	r.Register(BeforeTool, 2, func(p map[string]any) (any, bool) { called++; return nil, false })
	_, aborted := r.Fire(BeforeTool, nil)
	if !aborted || called != 1 {
		t.Fatalf("abort 应提前停止: aborted=%v called=%d", aborted, called)
	}
}

func TestPanicIsolated(t *testing.T) {
	r := NewRegistry()
	r.Register(OnError, 1, func(p map[string]any) (any, bool) { panic("boom") })
	r.Register(OnError, 2, func(p map[string]any) (any, bool) { return "after", false })
	res, aborted := r.Fire(OnError, nil)
	if aborted {
		t.Fatal("panic 不应变成 abort")
	}
	if len(res) != 2 || res[0].Panic != "boom" {
		t.Fatalf("panic 未隔离: %+v", res)
	}
	if res[1].Result != "after" {
		t.Fatalf("panic 后应继续执行: %+v", res[1])
	}
}

func TestUnregisterAndCount(t *testing.T) {
	r := NewRegistry()
	u1 := r.Register(AfterChat, 1, func(p map[string]any) (any, bool) { return nil, false })
	_ = r.Register(AfterChat, 2, func(p map[string]any) (any, bool) { return nil, false })
	if r.Count(AfterChat) != 2 || !r.Has(AfterChat) {
		t.Fatalf("count 不对")
	}
	u1()
	if r.Count(AfterChat) != 1 {
		t.Fatalf("注销后应 1: %d", r.Count(AfterChat))
	}
	r.Clear()
	if r.Has(AfterChat) {
		t.Fatal("Clear 后应为空")
	}
}
