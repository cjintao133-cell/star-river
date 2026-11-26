// Package critic 是"发前自检"回路：对草稿回答做启发式 + 可选 LLM 评审，产出可解释的结论。
//
// 目的（对齐 Reflexion / self-critique 模式）：在把回答交给用户前，拦下空答、拒答、占位符残留、
// 泄露系统提示、疑似幻觉声明等明显问题，并给出改进建议。零内部依赖（仅标准库）。
package critic

import (
	"context"
	"regexp"
	"strings"
)

// Verdict 评审结论。
type Verdict struct {
	Pass       bool     `json:"pass"`
	Score      float64  `json:"score"` // 0-1，越高越好
	Issues     []string `json:"issues,omitempty"`
	Suggestion string   `json:"suggestion,omitempty"`
}

// LLMCritic 可选的模型评审器：返回 (是否通过, 意见)。
type LLMCritic func(ctx context.Context, draft, ctxText string) (bool, string, error)

// Config 自检配置。
type Config struct {
	MinRunes int  // 正文最少字符（<=0 默认 2）
	UseLLM   bool // 是否调用注入的 LLM 评审
}

// Engine 评审引擎。
type Engine struct {
	cfg Config
	llm LLMCritic
}

// New 建引擎。
func New(cfg Config, llm LLMCritic) *Engine {
	return &Engine{cfg: cfg, llm: llm}
}

// 泄漏系统提示/内部指令的迹象。
var revealRe = regexp.MustCompile(`(?i)(system prompt|系统提示词?|my instructions are|我的指令是|以下是系统消息)`)
var placeholderRe = regexp.MustCompile(`(\{\{.*?\}\}|\[TODO\]|TODO:|待补充|<[^>]{0,20}占位[^>]*>|XXX)`)

// Review 评审一段草稿。context 为相关上下文（用户问题/目标），可为空。
func (e *Engine) Review(ctx context.Context, draft, ctxText string) Verdict {
	minRunes := e.cfg.MinRunes
	if minRunes <= 0 {
		minRunes = 2
	}
	v := Verdict{Pass: true, Score: 1.0}
	d := strings.TrimSpace(draft)

	if len([]rune(d)) < minRunes {
		v.Pass = false
		v.Score -= 0.6
		v.Issues = append(v.Issues, "回答为空或过短")
	}
	low := strings.ToLower(d)
	if strings.HasPrefix(low, "抱歉") && strings.Contains(low, "无法") {
		v.Issues = append(v.Issues, "疑似直接拒答（请确认确无可行路径）")
		v.Score -= 0.2
	}
	if revealRe.MatchString(d) {
		v.Pass = false
		v.Score -= 0.4
		v.Issues = append(v.Issues, "疑似泄露系统提示/内部指令")
	}
	if placeholderRe.MatchString(d) {
		v.Issues = append(v.Issues, "残留占位符/TODO")
		v.Score -= 0.3
	}

	// 可选 LLM 评审（叠加）。
	if e.cfg.UseLLM && e.llm != nil {
		if ok, note, err := e.llm(ctx, draft, ctxText); err == nil {
			if !ok {
				v.Pass = false
				v.Score -= 0.3
				v.Issues = append(v.Issues, "模型评审：不通过")
			}
			if strings.TrimSpace(note) != "" {
				v.Suggestion = note
			}
		}
	}

	if v.Score < 0 {
		v.Score = 0
	}
	if v.Score > 1 {
		v.Score = 1
	}
	if len(v.Issues) > 0 && v.Suggestion == "" {
		v.Suggestion = "请修复上述问题后重试：" + strings.Join(v.Issues, "；")
	}
	return v
}
