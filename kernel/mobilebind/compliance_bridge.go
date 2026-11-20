package mobilebind

// 合规桥（compliance）：暴露当前构建 flavor 的内容策略（国内 strict / 海外 standard）。
// 纯配置层，无副作用；UI 用于展示与按 ReportEntryVisible / ImageGenEnabled 做入口开关。

import (
	"encoding/json"

	"star-river/kernel/compliance"
)

// CompliancePolicy 返回当前内容策略 JSON。
func CompliancePolicy() string {
	b, _ := json.Marshal(compliance.CurrentPolicy())
	return string(b)
}

// ComplianceFlavor 返回当前 flavor 字符串（international/domestic）。
func ComplianceFlavor() string { return string(compliance.Current()) }
