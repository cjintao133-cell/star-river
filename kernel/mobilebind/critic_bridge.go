package mobilebind

// P1 发前自检桥：对草稿做启发式 + 可选模型评审，返回 Verdict JSON。

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cjintao133-cell/star-river/kernel/critic"
)

// criticLLM 用一次性补全做评审：约定输出含 "不通过"/"FAIL" 表示不通过。
var criticLLM = func(ctx context.Context, baseURL, apiKey, model, draft, ctxText string) (bool, string, error) {
	system := "你是严格的中文内容评审员。判断给出的回答是否准确、完整、无自相矛盾、无凭空编造。仅回复一行：以 PASS 或 FAIL 开头，后面用一句话给出理由。"
	user := "用户问题/目标：" + ctxText + "\n\n待评审回答：\n" + draft
	out, err := taskStepLLM(ctx, baseURL, apiKey, model, system, user)
	if err != nil {
		return true, "", err
	}
	up := strings.ToUpper(out)
	pass := !strings.Contains(up, "FAIL") && !strings.Contains(out, "不通过")
	return pass, strings.TrimSpace(out), nil
}

// CriticReview 评审草稿。baseURL/model 为空则只跑启发式。返回 Verdict JSON。
func CriticReview(draft, ctxText, baseURL, apiKey, model string) string {
	useLLM := strings.TrimSpace(baseURL) != "" && strings.TrimSpace(model) != ""
	var llm critic.LLMCritic
	if useLLM {
		llm = func(ctx context.Context, d, c string) (bool, string, error) {
			return criticLLM(ctx, baseURL, apiKey, model, d, c)
		}
	}
	e := critic.New(critic.Config{UseLLM: useLLM}, llm)
	v := e.Review(context.Background(), draft, ctxText)
	b, _ := json.Marshal(v)
	return string(b)
}
