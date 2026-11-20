package orchestrator

import (
	"strings"
	"testing"

	"star-river/kernel/core/types"
)

// MaxUserChars：超长用户输入被截断到上限（不可裁层也要有封顶）。
func TestBuildMinimalMaxUserChars(t *testing.T) {
	long := strings.Repeat("用户", 100) // 100 个 rune
	user := txt(types.RoleUser, long)
	got := textsOf(BuildMinimal("P", nil, user, Limits{MaxUserChars: 10}))
	total := 0
	for _, s := range got {
		total += len([]rune(s))
	}
	// persona(1) + 截断后 user(10 + 省略号1) ≈ 12；断言用户消息确实被截短。
	userGot := got[len(got)-1]
	if n := len([]rune(userGot)); n > 11 {
		t.Fatalf("用户消息未截断到上限: %d rune", n)
	}
	if _ = total; total > 20 {
		t.Fatalf("总量失控: %d", total)
	}
}

// MaxUserChars=0 不截断。
func TestBuildMinimalNoUserCap(t *testing.T) {
	long := strings.Repeat("x", 50)
	got := textsOf(BuildMinimal("", nil, txt(types.RoleUser, long), Limits{}))
	if got[0] != long {
		t.Fatalf("不应截断")
	}
}
