package orchestrator

import "github.com/cjintao133-cell/star-river/kernel/core/types"

// Limits 最小上下文预算。<=0 表示该项不限制。
type Limits struct {
	MaxMessages int // 含人设与本次用户消息在内的总消息条数上限
	MaxChars    int // 文本总字符预算（只统计 text / tool_result / tool_use 参数）
	// MaxPersonaChars persona/system 自身字符上限（<=0 不截断）。防止超大角色卡/记忆注入撑爆请求。
	MaxPersonaChars int
	// MaxUserChars 本次用户消息文本字符上限（<=0 不截断）。防止超长用户输入把"不可裁层"顶穿预算。
	// 设了它后，请求总量的硬上限 ≈ MaxPersonaChars + MaxUserChars + MaxChars，可预测。
	MaxUserChars int
}

// msgChars 估算一条消息的文本占用（块文本 + 工具调用参数）。
func msgChars(m types.Message) int {
	n := 0
	for _, b := range m.Blocks {
		n += len(b.Text)
		if b.Call != nil {
			n += len(b.Call.Arguments)
		}
	}
	return n
}

// BuildMinimal 七层上下文最小版（T-M1-08 验收）：人设（system）+ 近期消息 + 本次用户消息。
// 历史从最近往前取，受 Limits 约束；**人设与本次用户消息保底不裁**（预算只裁历史）。
// 返回可直接喂适配器的 []types.Message。不做持久化/摘要/分层配额（完整版 T-M3-03）。
func BuildMinimal(persona string, history []types.Message, user types.Message, lim Limits) []types.Message {
	out := make([]types.Message, 0, len(history)+2)

	// 用户消息文本封顶（不可裁层也要有上限，否则超大输入顶穿预算）。
	if lim.MaxUserChars > 0 {
		user = truncateUserText(user, lim.MaxUserChars)
	}

	var personaMsg *types.Message
	if persona != "" {
		if lim.MaxPersonaChars > 0 {
			persona = truncateRunes(persona, lim.MaxPersonaChars)
		}
		m := types.Message{Role: types.RoleSystem, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: persona}}}
		personaMsg = &m
		out = append(out, m)
	}

	// 历史可选条数：总上限 - 人设 - 本次user。
	maxHist := len(history)
	if lim.MaxMessages > 0 {
		room := lim.MaxMessages - 1 // 本次 user 占 1
		if personaMsg != nil {
			room--
		}
		if room < 0 {
			room = 0
		}
		if maxHist > room {
			maxHist = room
		}
	}

	// 字符预算：先扣除人设与本次 user 的保底占用。
	remainChars := -1 // -1 = 不限
	if lim.MaxChars > 0 {
		base := msgChars(user)
		if personaMsg != nil {
			base += msgChars(*personaMsg)
		}
		remainChars = lim.MaxChars - base
		if remainChars < 0 {
			remainChars = 0
		}
	}

	// 从最近往前挑历史，受条数与字符双约束。
	picked := make([]types.Message, 0, maxHist)
	for i := len(history) - 1; i >= 0 && len(picked) < maxHist; i-- {
		h := history[i]
		c := msgChars(h)
		if remainChars >= 0 && c > remainChars {
			break // 再往前只会更长，停止（保持时间连续）
		}
		if remainChars >= 0 {
			remainChars -= c
		}
		picked = append(picked, h)
	}
	// 反转为时间正序。
	for i := len(picked) - 1; i >= 0; i-- {
		out = append(out, picked[i])
	}

	out = append(out, user)
	return out
}

// truncateUserText 把用户消息的文本块累计截断到 maxChars（保留图片等非文本块），返回副本。
func truncateUserText(m types.Message, maxChars int) types.Message {
	if maxChars <= 0 || msgChars(m) <= maxChars {
		return m
	}
	cp := m
	cp.Blocks = make([]types.ContentBlock, len(m.Blocks))
	copy(cp.Blocks, m.Blocks)
	budget := maxChars
	for i := range cp.Blocks {
		b := &cp.Blocks[i]
		if b.Call != nil {
			// 工具调用参数也计入文本预算：超限截断。
			if len(b.Call.Arguments) > budget {
				b.Call.Arguments = string([]byte(b.Call.Arguments)[:clampNonNeg(budget)]) + "…"
				budget = 0
			} else {
				budget -= len(b.Call.Arguments)
			}
			continue
		}
		if b.Text == "" {
			continue
		}
		r := []rune(b.Text)
		if len(r) > budget {
			b.Text = string(r[:clampNonNeg(budget)]) + "…"
			budget = 0
		} else {
			budget -= len(r)
		}
	}
	return cp
}

func clampNonNeg(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// truncateRunes 按字符（rune）截断，超限时加省略号。
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
