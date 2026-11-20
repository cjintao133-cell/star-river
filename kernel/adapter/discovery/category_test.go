package discovery

import (
	"sort"
	"testing"
)

func TestClassifyMetadata(t *testing.T) {
	cases := []struct {
		id      string
		wantCat Category
		wantSrc string
	}{
		{"gpt-4o", CategoryFlagship, SourceMetadata},
		{"gpt-4o-mini", CategoryFast, SourceMetadata},            // 最长匹配胜过 gpt-4o
		{"gpt-4o-mini-2024-07-18", CategoryFast, SourceMetadata}, // 日期后缀吸收
		{"gpt-4.1-mini", CategoryFast, SourceMetadata},
		{"o3-mini", CategoryReasoning, SourceMetadata},
		{"o1-preview", CategoryReasoning, SourceMetadata},
		{"claude-3-5-sonnet-20241022", CategoryFlagship, SourceMetadata},
		{"claude-3-5-haiku-20241022", CategoryFast, SourceMetadata},
		{"models/gemini-1.5-pro-002", CategoryFlagship, SourceMetadata}, // models/ 前缀与版本后缀
		{"gemini-1.5-flash-8b", CategoryFast, SourceMetadata},
		{"deepseek-reasoner", CategoryReasoning, SourceMetadata},
		{"GPT-4O", CategoryFlagship, SourceMetadata}, // 大小写归一
		{"  gpt-4o  ", CategoryFlagship, SourceMetadata},
	}
	for _, c := range cases {
		got := Classify(c.id)
		if got.Category != c.wantCat || got.Source != c.wantSrc {
			t.Errorf("Classify(%q) = (%s,%s), want (%s,%s)", c.id, got.Category, got.Source, c.wantCat, c.wantSrc)
		}
		if got.ModelID != c.id {
			t.Errorf("Classify(%q).ModelID = %q, 应保留原始输入", c.id, got.ModelID)
		}
	}
}

func TestClassifyHeuristic(t *testing.T) {
	cases := []struct {
		id      string
		wantCat Category
	}{
		{"acme-flash-v2", CategoryFast},       // flash
		{"acme-reasoner", CategoryReasoning},  // reason
		{"mega-ultra-9000", CategoryFlagship}, // ultra
		{"some-haiku-model", CategoryFast},
		{"my-thinking-model", CategoryReasoning},
	}
	for _, c := range cases {
		got := Classify(c.id)
		if got.Category != c.wantCat || got.Source != SourceHeuristic {
			t.Errorf("Classify(%q) = (%s,%s), want (%s,heuristic)", c.id, got.Category, got.Source, c.wantCat)
		}
	}
}

func TestClassifyFallback(t *testing.T) {
	for _, id := range []string{"totally-unknown-xyz", "", "abc123"} {
		got := Classify(id)
		if got.Category != CategoryUnknown || got.Source != SourceFallback {
			t.Errorf("Classify(%q) = (%s,%s), want (unknown,fallback)", id, got.Category, got.Source)
		}
	}
}

// 元数据优先于启发式：gpt-4.1-nano 含 "nano"(启发式 fast)，但元数据也是 fast；
// 关键是 source 必须是 metadata（证明是精确表命中而非启发式）。
func TestClassifyMetadataPrecedesHeuristic(t *testing.T) {
	got := Classify("gpt-4.1-nano")
	if got.Source != SourceMetadata || got.Category != CategoryFast {
		t.Errorf("got (%s,%s), want (fast,metadata)", got.Category, got.Source)
	}
}

func TestModelMismatch(t *testing.T) {
	cases := []struct {
		req, ret string
		want     bool
	}{
		{"gpt-4o", "gpt-4o-2024-08-06", false}, // 同基名，日期后缀不算掉包
		{"claude-3-5-sonnet", "claude-3-5-sonnet-20241022", false},
		{"models/gemini-1.5-pro", "gemini-1.5-pro-002", false},
		{"gpt-4o", "gpt-4o-mini-2024-08-06", true}, // 不同基名
		{"gpt-4o", "", false},                      // 空应答=无信号
		{"gpt-4o", "   ", false},
		{"deepseek-chat", "deepseek-reasoner", true},
	}
	for _, c := range cases {
		if got := ModelMismatch(c.req, c.ret); got != c.want {
			t.Errorf("ModelMismatch(%q,%q) = %v, want %v", c.req, c.ret, got, c.want)
		}
	}
}

func TestBuiltinCandidates(t *testing.T) {
	cs := BuiltinCandidates()
	if len(cs) == 0 {
		t.Fatal("BuiltinCandidates 不应为空")
	}
	if !sort.StringsAreSorted(cs) {
		t.Errorf("BuiltinCandidates 应升序: %v", cs)
	}
	found := false
	for _, s := range cs {
		if s == "gpt-4o" {
			found = true
		}
	}
	if !found {
		t.Error("BuiltinCandidates 应包含 gpt-4o")
	}
}
