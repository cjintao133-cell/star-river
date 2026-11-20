package mobilebind

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestComplianceBridge(t *testing.T) {
	f := ComplianceFlavor()
	if f != "international" && f != "domestic" {
		t.Fatalf("flavor 异常: %s", f)
	}
	var p map[string]any
	if err := json.Unmarshal([]byte(CompliancePolicy()), &p); err != nil {
		t.Fatalf("策略应 JSON: %v", err)
	}
	if !strings.Contains(CompliancePolicy(), "\"report_entry_visible\"") {
		t.Fatalf("策略应含 report_entry_visible")
	}
}
