// Package compliance 是内容合规开关（T-M7-09）：国内外 build flavor 默认内容策略不同。
//
// 设计：**同一份代码库，两个 SKU 靠构建参数区分**（不 fork 代码）：
//   - 海外（international，默认）：标准敏感过滤，默认英文界面。
//   - 国内（domestic）：更严格敏感过滤，屏蔽若干分类，默认中文界面，强制展示举报入口。
//
// 选择 flavor：构建期用 -ldflags 注入，例如：
//
//	go build -ldflags "-X star-river/kernel/compliance.currentFlavor=domestic" ...
//	不注入则为 international（默认）。
//
// 本包只提供**默认策略配置**与判定（内核可测）；「举报入口 UI 可达」与「双 SKU 真实构建产出」
// 属界面/构建层（押后），内核侧已用 ReportEntryVisible 位暴露"是否应展示举报入口"，平台能力表补记差异。
//
// 依赖方向：纯标准库，无插件依赖。
package compliance

import "fmt"

// Flavor 构建风味。
type Flavor string

const (
	// International 海外风味（默认）。
	International Flavor = "international"
	// Domestic 国内风味。
	Domestic Flavor = "domestic"
)

// currentFlavor 由 -ldflags -X 在构建期覆盖；默认 international。
var currentFlavor = string(International)

// Valid 报告 flavor 是否合法。
func (f Flavor) Valid() bool { return f == International || f == Domestic }

// Current 返回当前构建的 flavor（非法值回退 international）。
func Current() Flavor {
	f := Flavor(currentFlavor)
	if !f.Valid() {
		return International
	}
	return f
}

// ContentPolicy 默认内容策略。
type ContentPolicy struct {
	Flavor             Flavor   `json:"flavor"`
	SensitiveFilter    string   `json:"sensitive_filter"`     // "strict" | "standard"
	BlockedCategories  []string `json:"blocked_categories"`   // 默认关闭的生成/内容分类
	ReportEntryVisible bool     `json:"report_entry_visible"` // 是否展示举报入口
	DefaultLanguage    string   `json:"default_language"`     // 默认界面语言（zh / en）
	ImageGenEnabled    bool     `json:"image_gen_enabled"`    // 视觉生成总开关（合规定位）
}

// Policy 返回某 flavor 的默认策略。未知 flavor 回落 international 并置错误由调用方判断。
func Policy(f Flavor) ContentPolicy {
	switch f {
	case Domestic:
		return ContentPolicy{
			Flavor:             Domestic,
			SensitiveFilter:    "strict",
			BlockedCategories:  []string{"nsfw", "violence", "gambling"},
			ReportEntryVisible: true,
			DefaultLanguage:    "zh",
			ImageGenEnabled:    true,
		}
	default: // International
		return ContentPolicy{
			Flavor:             International,
			SensitiveFilter:    "standard",
			BlockedCategories:  []string{"nsfw"},
			ReportEntryVisible: true,
			DefaultLanguage:    "en",
			ImageGenEnabled:    true,
		}
	}
}

// CurrentPolicy 当前构建的默认策略。
func CurrentPolicy() ContentPolicy { return Policy(Current()) }

// CategoryBlocked 报告某分类是否被策略默认屏蔽。
func (p ContentPolicy) CategoryBlocked(category string) bool {
	for _, c := range p.BlockedCategories {
		if c == category {
			return true
		}
	}
	return false
}

// String 便于展示。
func (p ContentPolicy) String() string {
	return fmt.Sprintf("flavor=%s filter=%s blocked=%v report=%v lang=%s",
		p.Flavor, p.SensitiveFilter, p.BlockedCategories, p.ReportEntryVisible, p.DefaultLanguage)
}
