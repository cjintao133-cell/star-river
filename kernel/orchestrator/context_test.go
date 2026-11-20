package orchestrator

import (
	"testing"

	"star-river/kernel/core/types"
)

func txt(role types.Role, s string) types.Message {
	return types.Message{Role: role, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: s}}}
}

func textsOf(msgs []types.Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		for _, b := range m.Blocks {
			out[i] += b.Text
		}
	}
	return out
}

func TestBuildMinimalPersonaAndUser(t *testing.T) {
	user := txt(types.RoleUser, "hello")
	got := BuildMinimal("你是示例角色", nil, user, Limits{})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Role != types.RoleSystem || got[0].Blocks[0].Text != "你是示例角色" {
		t.Errorf("persona = %+v", got[0])
	}
	if got[1].Role != types.RoleUser || got[1].Blocks[0].Text != "hello" {
		t.Errorf("user = %+v", got[1])
	}
}

func TestBuildMinimalNoPersona(t *testing.T) {
	got := BuildMinimal("", nil, txt(types.RoleUser, "hi"), Limits{})
	if len(got) != 1 || got[0].Role != types.RoleUser {
		t.Errorf("got %+v", got)
	}
}

func TestBuildMinimalMaxMessagesKeepsRecent(t *testing.T) {
	hist := []types.Message{
		txt(types.RoleUser, "m1"), txt(types.RoleAssistant, "m2"),
		txt(types.RoleUser, "m3"), txt(types.RoleAssistant, "m4"),
	}
	user := txt(types.RoleUser, "m5")
	// 总上限 3：persona(1) + user(1) 保底 → 历史只留最近 1 条 (m4)。
	got := BuildMinimal("P", hist, user, Limits{MaxMessages: 3})
	want := []string{"P", "m4", "m5"}
	if gotTexts := textsOf(got); !eq(gotTexts, want) {
		t.Errorf("got %v, want %v", gotTexts, want)
	}
}

func TestBuildMinimalMaxMessagesNoPersona(t *testing.T) {
	hist := []types.Message{txt(types.RoleUser, "a"), txt(types.RoleAssistant, "b"), txt(types.RoleUser, "c")}
	user := txt(types.RoleUser, "d")
	// 上限 2：user 保底 1，历史留最近 1 条 (c)。
	got := textsOf(BuildMinimal("", hist, user, Limits{MaxMessages: 2}))
	if !eq(got, []string{"c", "d"}) {
		t.Errorf("got %v", got)
	}
}

func TestBuildMinimalMaxCharsTrims(t *testing.T) {
	hist := []types.Message{
		txt(types.RoleUser, "aaaa"),      // 4
		txt(types.RoleAssistant, "bbbb"), // 4
		txt(types.RoleUser, "cccc"),      // 4
	}
	user := txt(types.RoleUser, "dd") // 2
	// 预算 8：user(2) 保底 → 历史可 6。从最近 cccc(4)，再加 bbbb 会 8>6 停 → 只留 cccc。
	got := textsOf(BuildMinimal("", hist, user, Limits{MaxChars: 8}))
	if !eq(got, []string{"cccc", "dd"}) {
		t.Errorf("got %v, want [cccc dd]", got)
	}
}

func TestBuildMinimalPersonaUserNeverDropped(t *testing.T) {
	// 预算极小，历史全裁，但 persona 与 user 必须保留。
	hist := []types.Message{txt(types.RoleUser, "long-history-text-xxxxxxxxxxxx")}
	user := txt(types.RoleUser, "u")
	got := textsOf(BuildMinimal("P", hist, user, Limits{MaxChars: 3}))
	if len(got) != 2 || got[0] != "P" || got[1] != "u" {
		t.Errorf("persona/user 必须保底: %v", got)
	}
}

func TestBuildMinimalUnlimited(t *testing.T) {
	hist := []types.Message{txt(types.RoleUser, "1"), txt(types.RoleAssistant, "2"), txt(types.RoleUser, "3")}
	got := textsOf(BuildMinimal("P", hist, txt(types.RoleUser, "u"), Limits{}))
	if !eq(got, []string{"P", "1", "2", "3", "u"}) {
		t.Errorf("got %v", got)
	}
}

// tool_use 参数计入字符预算。
func TestMsgCharsCountsToolArgs(t *testing.T) {
	m := types.Message{Role: types.RoleAssistant, Blocks: []types.ContentBlock{
		{Type: types.BlockToolUse, Call: &types.ToolCall{ID: "c", Name: "f", Arguments: `{"x":123}`}},
		{Type: types.BlockText, Text: "ok"},
	}}
	// len(`{"x":123}`)=9 + len("ok")=2 = 11
	if got := msgChars(m); got != 11 {
		t.Errorf("msgChars = %d, want 11", got)
	}
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
