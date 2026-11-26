package policy

import "testing"

func TestPlainToolAllowed(t *testing.T) {
	e := New(Config{})
	if d, _, _ := e.Check(Action{Kind: KindTool, Name: "search", Reversible: true}); d != Allow {
		t.Fatalf("普通工具应放行, got %s", d)
	}
}

func TestSpendBudget(t *testing.T) {
	e := New(Config{TotalBudget: 100, AutoApproveSpend: true})
	if d, _, _ := e.Check(Action{Kind: KindSpend, Name: "buy", Amount: 60}); d != Allow {
		t.Fatalf("预算内应放行, got %s", d)
	}
	e.Commit(Action{Kind: KindSpend, Amount: 60})
	if d, r, _ := e.Check(Action{Kind: KindSpend, Name: "buy", Amount: 50}); d != OverBudget {
		t.Fatalf("超预算应拦截, got %s (%s)", d, r)
	}
	spent, total := e.Budget()
	if spent != 60 || total != 100 {
		t.Fatalf("记账不对: spent=%v total=%v", spent, total)
	}
}

func TestSpendRequiresConfirm(t *testing.T) {
	e := New(Config{TotalBudget: 100}) // 未授权自动消费
	d, _, pid := e.Check(Action{Kind: KindSpend, Name: "buy", Amount: 10})
	if d != RequireConfirm || pid == "" {
		t.Fatalf("未授权消费应要求确认: %s %q", d, pid)
	}
	if len(e.ListPending()) != 1 {
		t.Fatalf("应登记 1 条待确认")
	}
	p, ok, approved := e.Resolve(pid, true)
	if !ok || !approved || p.Action.Amount != 10 {
		t.Fatalf("确认处理不对: %+v %v %v", p, ok, approved)
	}
	if len(e.ListPending()) != 0 {
		t.Fatalf("确认后应移除")
	}
}

func TestSendAndDangerousRequireConfirm(t *testing.T) {
	e := New(Config{})
	if d, _, _ := e.Check(Action{Kind: KindSend, Name: "send_email"}); d != RequireConfirm {
		t.Errorf("对外发送应确认, got %s", d)
	}
	if d, _, _ := e.Check(Action{Kind: KindTool, Name: "rm_rf", Reversible: false}); d != RequireConfirm {
		t.Errorf("不可逆危险应确认, got %s", d)
	}
	if d, _, _ := e.Check(Action{Kind: KindTool, Name: "fetch_url", External: true}); d != RequireConfirm {
		t.Errorf("外部触达应确认, got %s", d)
	}
}

func TestAllowlistDenylist(t *testing.T) {
	e := New(Config{ToolDenylist: []string{"bad_tool"}})
	if d, _, _ := e.Check(Action{Kind: KindTool, Name: "bad_tool", Reversible: true}); d != Deny {
		t.Errorf("黑名单应拒绝, got %s", d)
	}
	e2 := New(Config{ToolAllowlist: []string{"search"}})
	if d, _, _ := e2.Check(Action{Kind: KindTool, Name: "search", Reversible: true}); d != Allow {
		t.Errorf("白名单内应放行, got %s", d)
	}
	if d, _, _ := e2.Check(Action{Kind: KindTool, Name: "other", Reversible: true}); d != Deny {
		t.Errorf("白名单外应拒绝, got %s", d)
	}
}

func TestResolveUnknown(t *testing.T) {
	e := New(Config{})
	if _, ok, _ := e.Resolve("nope", true); ok {
		t.Error("未知 id 应返回未命中")
	}
}

func TestSetConfigKeepsSpent(t *testing.T) {
	e := New(Config{TotalBudget: 100, AutoApproveSpend: true})
	e.Commit(Action{Kind: KindSpend, Amount: 30})
	e.SetConfig(Config{TotalBudget: 200, AutoApproveSpend: true})
	spent, total := e.Budget()
	if spent != 30 || total != 200 {
		t.Fatalf("改配置应保留花费: spent=%v total=%v", spent, total)
	}
	e.ResetSpent()
	if s, _ := e.Budget(); s != 0 {
		t.Fatalf("重置后应为 0, got %v", s)
	}
}
