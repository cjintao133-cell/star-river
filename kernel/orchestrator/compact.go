package orchestrator

import (
	"strings"

	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

// CompactResult 上下文压缩结果。
// Digest 为被折叠旧消息的摘要（可拼进 system）；Kept 为保留下来的近期消息（时间正序）。
type CompactResult struct {
	Digest      string
	Kept        []types.Message
	FoldedCount int
	KeptCount   int
	CharsBefore int
	CharsAfter  int
}

// DefaultKeepRecent Compact 未指定 keepRecent 时的默认保留条数。
const DefaultKeepRecent = 20

// Compact 把历史折叠成「摘要 + 近期窗口」，用于长对话防撑爆（T-M3-03 落地）。
//   - keepRecent：保留最近多少条原样（<=0 用 DefaultKeepRecent）。
//   - budgetChars：折叠后（Kept 文本 + Digest）的字符预算；<=0 表示不按预算裁剪，仅按条数。
//   - perMsgDigestChars：抽取式摘要时每条旧消息保留的字符数（<=0 用 80）。
//   - summarize：可选 LLM 摘要器；nil 时用确定性的抽取式摘要（便于离线测试）。
//
// 纯函数：不读存储、不发网络。summarize 由调用方注入（真机走模型，测试注入假实现）。
func Compact(history []types.Message, keepRecent, budgetChars, perMsgDigestChars int, summarize func([]types.Message) (string, error)) CompactResult {
	if keepRecent <= 0 {
		keepRecent = DefaultKeepRecent
	}
	res := CompactResult{CharsBefore: messagesChars(history)}
	if len(history) <= keepRecent {
		res.Kept = append([]types.Message(nil), history...)
		res.KeptCount = len(res.Kept)
		res.CharsAfter = messagesChars(res.Kept)
		return res
	}
	split := len(history) - keepRecent
	old := history[:split]
	res.Kept = append([]types.Message(nil), history[split:]...)
	res.FoldedCount = len(old)

	// 生成摘要：优先注入的 LLM 摘要器，失败或未注入则回退抽取式。
	if summarize != nil {
		if d, err := summarize(old); err == nil && strings.TrimSpace(d) != "" {
			res.Digest = d
		}
	}
	if res.Digest == "" {
		res.Digest = extractiveDigest(old, perMsgDigestChars)
	}

	// 摘要本身也受预算约束：不超过预算的一半，避免摘要把预算顶穿。
	if budgetChars > 0 {
		if maxD := budgetChars / 2; maxD > 0 && runeLen(res.Digest) > maxD {
			r := []rune(res.Digest)
			res.Digest = string(r[:maxD]) + "…"
		}
	}

	// 预算裁剪：从最旧保留项开始丢，直到进入预算（至少留 1 条）。
	if budgetChars > 0 {
		for len(res.Kept) > 1 && messagesChars(res.Kept)+runeLen(res.Digest) > budgetChars {
			res.Kept = res.Kept[1:]
			res.FoldedCount++
		}
	}
	res.KeptCount = len(res.Kept)
	res.CharsAfter = messagesChars(res.Kept) + runeLen(res.Digest)
	return res
}

// CompactSystem 把摘要包装成可拼进 system 的段落；空摘要返回空串。
func CompactSystem(digest string) string {
	if strings.TrimSpace(digest) == "" {
		return ""
	}
	return "【早期对话摘要】\n" + strings.TrimSpace(digest)
}

// extractiveDigest 确定性抽取式摘要：每条旧消息取角色 + 首句（截断）。
func extractiveDigest(msgs []types.Message, per int) string {
	if per <= 0 {
		per = 80
	}
	var b strings.Builder
	for _, m := range msgs {
		t := strings.TrimSpace(messageText(m))
		if t == "" {
			continue
		}
		t = firstSentence(t, per)
		b.WriteString("- ")
		b.WriteString(string(m.Role))
		b.WriteString(": ")
		b.WriteString(t)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// messageText 拼接一条消息的所有文本块。
func messageText(m types.Message) string {
	var b strings.Builder
	for _, blk := range m.Blocks {
		if blk.Type == types.BlockText {
			b.WriteString(blk.Text)
		}
	}
	return b.String()
}

// firstSentence 取首句（遇到中英句末标点或换行提前收），最多 max 字符。
func firstSentence(s string, max int) string {
	r := []rune(s)
	cut := len(r)
	for i, c := range r {
		if c == '。' || c == '！' || c == '？' || c == '.' || c == '!' || c == '?' || c == '\n' {
			cut = i + 1
			break
		}
	}
	if cut > max {
		cut = max
	}
	if cut >= len(r) {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(string(r[:cut])) + "…"
}

func messagesChars(msgs []types.Message) int {
	n := 0
	for _, m := range msgs {
		n += msgChars(m)
	}
	return n
}

func runeLen(s string) int { return len([]rune(s)) }
