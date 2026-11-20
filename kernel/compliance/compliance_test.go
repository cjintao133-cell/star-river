package compliance

import "testing"

func TestFlavorValid(t *testing.T) {
	if !International.Valid() || !Domestic.Valid() {
		t.Error("两种 flavor 应合法")
	}
	if Flavor("mars").Valid() {
		t.Error("未知 flavor 应非法")
	}
}

func TestCurrentDefaultInternational(t *testing.T) {
	// 未注入 ldflags 时默认 international（currentFlavor 默认值）。
	if Current() != International {
		t.Errorf("默认应为 international, got %s", Current())
	}
	// 非法值回退
	old := currentFlavor
	currentFlavor = "garbage"
	if Current() != International {
		t.Errorf("非法值应回退国际, got %s", Current())
	}
	currentFlavor = old
}

func TestPoliciesDiffer(t *testing.T) {
	dom := Policy(Domestic)
	intl := Policy(International)
	if dom.Flavor != Domestic || intl.Flavor != International {
		t.Fatal("flavor 标注不对")
	}
	// 核心差异：国内更严格、屏蔽更多分类、默认中文；海外标准、默认英文。
	if dom.SensitiveFilter != "strict" || intl.SensitiveFilter != "standard" {
		t.Errorf("敏感过滤应不同: dom=%s intl=%s", dom.SensitiveFilter, intl.SensitiveFilter)
	}
	if dom.DefaultLanguage != "zh" || intl.DefaultLanguage != "en" {
		t.Errorf("默认语言应不同: dom=%s intl=%s", dom.DefaultLanguage, intl.DefaultLanguage)
	}
	if len(dom.BlockedCategories) <= len(intl.BlockedCategories) {
		t.Errorf("国内屏蔽分类应不少于海外: dom=%v intl=%v", dom.BlockedCategories, intl.BlockedCategories)
	}
	// 两者都应展示举报入口（合规底线）。
	if !dom.ReportEntryVisible || !intl.ReportEntryVisible {
		t.Error("举报入口应都可达")
	}
}

func TestCategoryBlocked(t *testing.T) {
	dom := Policy(Domestic)
	if !dom.CategoryBlocked("gambling") || !dom.CategoryBlocked("violence") {
		t.Error("国内应屏蔽 gambling/violence")
	}
	if dom.CategoryBlocked("landscape") {
		t.Error("landscape 不应被屏蔽")
	}
	intl := Policy(International)
	if intl.CategoryBlocked("gambling") {
		t.Error("海外默认不屏蔽 gambling")
	}
}

func TestUnknownFlavorFallsBack(t *testing.T) {
	p := Policy(Flavor("mars"))
	if p.Flavor != International {
		t.Errorf("未知 flavor 策略应回落 international, got %s", p.Flavor)
	}
}

func TestCurrentPolicy(t *testing.T) {
	p := CurrentPolicy()
	if p.Flavor != Current() {
		t.Errorf("CurrentPolicy.flavor 应与 Current 一致")
	}
	if p.String() == "" {
		t.Error("String 不应为空")
	}
}
