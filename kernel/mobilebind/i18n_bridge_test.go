package mobilebind

import (
	"strings"
	"testing"
)

func TestI18nBridge(t *testing.T) {
	if got := I18nT("zh", "chat.send"); got != "发送" {
		t.Fatalf("zh chat.send = %q", got)
	}
	if got := I18nT("en", "chat.send"); got != "Send" {
		t.Fatalf("en chat.send = %q", got)
	}
	// 回退：未知 key 返回 key 本身
	if got := I18nT("zh", "no.such.key"); got != "no.such.key" {
		t.Fatalf("unknown key = %q", got)
	}
	if !strings.Contains(I18nLanguages(), "zh") {
		t.Fatalf("langs 应含 zh")
	}
}
