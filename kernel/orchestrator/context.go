package orchestrator

import "star-river/kernel/core/types"

// Limits 最小上下文预算。<=0 表示该项不限制。
type Limits struct {
	MaxMessages int // 含人设与本次用户消息在内的总消息条数上限
	MaxChars    int // 文本总字符预算（只统计 text / tool_result / tool_use 参数）
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

	var personaMsg *types.Message
	if persona != "" {
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
