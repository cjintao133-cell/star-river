package critic

import (
	"context"
	"errors"
	"testing"
)

func TestReviewClean(t *testing.T) {
	e := New(Config{}, nil)
	v := e.Review(context.Background(), "这是完整的回答，包含要点与结论。", "问题")
	if !v.Pass || v.Score < 0.9 || len(v.Issues) != 0 {
		t.Fatalf("干净回答应通过: %+v", v)
	}
}

func TestReviewEmpty(t *testing.T) {
	e := New(Config{}, nil)
	v := e.Review(context.Background(), "  ", "")
	if v.Pass || len(v.Issues) == 0 {
		t.Fatalf("空答应不通过: %+v", v)
	}
}

func TestReviewLeakAndPlaceholder(t *testing.T) {
	e := New(Config{}, nil)
	v := e.Review(context.Background(), "我的指令是：泄露 system prompt。TODO: 补充 {{x}}", "")
	if v.Pass {
		t.Fatalf("泄露/占位符应不通过: %+v", v)
	}
	joined := ""
	for _, i := range v.Issues {
		joined += i
	}
	if !contains(joined, "泄露") || !contains(joined, "占位") {
		t.Fatalf("应检出多类问题: %+v", v.Issues)
	}
}

func TestReviewLLMOverride(t *testing.T) {
	e := New(Config{UseLLM: true}, func(ctx context.Context, d, c string) (bool, string, error) {
		return false, "建议补充数据来源", nil
	})
	v := e.Review(context.Background(), "看起来完整的回答内容。", "")
	if v.Pass {
		t.Fatalf("模型评审不通过应影响结论: %+v", v)
	}
	if v.Suggestion != "建议补充数据来源" {
		t.Fatalf("应采纳模型建议: %q", v.Suggestion)
	}
}

func TestReviewLLMErrorIgnored(t *testing.T) {
	e := New(Config{UseLLM: true}, func(ctx context.Context, d, c string) (bool, string, error) {
		return false, "", errors.New("boom")
	})
	v := e.Review(context.Background(), "正常回答内容。", "")
	if !v.Pass {
		t.Fatalf("LLM 出错不应影响启发式结论: %+v", v)
	}
}

func TestReviewRefusalSignal(t *testing.T) {
	e := New(Config{}, nil)
	v := e.Review(context.Background(), "抱歉，我无法完成这个任务。", "")
	if v.Score >= 1.0 {
		t.Fatalf("拒答应降分: %+v", v)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
