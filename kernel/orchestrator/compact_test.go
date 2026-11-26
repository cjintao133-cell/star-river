package orchestrator

import (
	"errors"
	"strings"
	"testing"

	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

func mkMsg(role types.Role, text string) types.Message {
	return types.Message{Role: role, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: text}}}
}

func TestCompactNoFold(t *testing.T) {
	h := []types.Message{mkMsg(types.RoleUser, "a"), mkMsg(types.RoleAssistant, "b")}
	res := Compact(h, 5, 0, 0, nil)
	if res.FoldedCount != 0 || len(res.Kept) != 2 || res.Digest != "" {
		t.Fatalf("不该折叠: %+v", res)
	}
}

func TestCompactExtractive(t *testing.T) {
	h := []types.Message{
		mkMsg(types.RoleUser, "第一句。第二句。"),
		mkMsg(types.RoleAssistant, "回答一"),
		mkMsg(types.RoleUser, "问题二"),
		mkMsg(types.RoleAssistant, "回答二"),
		mkMsg(types.RoleUser, "最近一句"),
	}
	res := Compact(h, 2, 0, 0, nil)
	if res.FoldedCount != 3 || len(res.Kept) != 2 {
		t.Fatalf("折叠计数不对: %+v", res)
	}
	if !strings.Contains(res.Digest, "user: 第一句。") {
		t.Errorf("抽取式摘要应保留首句: %q", res.Digest)
	}
	if strings.Contains(res.Digest, "第二句") {
		t.Errorf("首句后不应进入摘要: %q", res.Digest)
	}
	if res.Kept[0].Blocks[0].Text != "回答二" || res.Kept[1].Blocks[0].Text != "最近一句" {
		t.Errorf("保留窗口不对: %+v", res.Kept)
	}
}

func TestCompactCustomSummarizer(t *testing.T) {
	h := []types.Message{mkMsg(types.RoleUser, "x1"), mkMsg(types.RoleUser, "x2"), mkMsg(types.RoleUser, "x3")}
	called := 0
	res := Compact(h, 1, 0, 0, func(old []types.Message) (string, error) {
		called = len(old)
		return "LLM摘要", nil
	})
	if called != 2 {
		t.Errorf("摘要器应收到 2 条旧消息, got %d", called)
	}
	if res.Digest != "LLM摘要" {
		t.Errorf("应用自定义摘要, got %q", res.Digest)
	}
}

func TestCompactSummarizerFallback(t *testing.T) {
	h := []types.Message{mkMsg(types.RoleUser, "旧一。"), mkMsg(types.RoleUser, "旧二。"), mkMsg(types.RoleUser, "新")}
	res := Compact(h, 1, 0, 0, func(old []types.Message) (string, error) {
		return "", errors.New("boom")
	})
	if !strings.Contains(res.Digest, "旧一") {
		t.Errorf("摘要器失败应回退抽取式: %q", res.Digest)
	}
}

func TestCompactBudgetTrim(t *testing.T) {
	// 每条 100 字符，保留 5 条 -> 500；预算 250 => 应不断丢到 <=250。
	var h []types.Message
	for i := 0; i < 10; i++ {
		h = append(h, mkMsg(types.RoleUser, strings.Repeat("a", 100)))
	}
	res := Compact(h, 5, 250, 0, nil)
	if len(res.Kept) >= 5 {
		t.Errorf("预算应裁掉部分保留项, kept=%d", len(res.Kept))
	}
	if res.CharsAfter > 250+120 { // 摘要本身可能占一点；给宽松上界
		t.Errorf("裁剪后字符应受控: after=%d", res.CharsAfter)
	}
	if len(res.Kept) < 1 {
		t.Errorf("至少保留 1 条")
	}
}

func TestCompactSystemWrap(t *testing.T) {
	if CompactSystem("  ") != "" {
		t.Error("空摘要应返回空串")
	}
	if got := CompactSystem("摘要内容"); got != "【早期对话摘要】\n摘要内容" {
		t.Errorf("包装格式不对: %q", got)
	}
}
