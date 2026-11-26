package mobilebind

// P0-2 上下文工程 + 分层记忆桥：
//   - BuildChatContext：把会话历史折叠成「摘要 + 近期窗口」（内核 Compactor），替代 App 侧硬拼最近 N 条。
//   - CompactSession：与 BuildChatContext 同压缩，并把摘要沉淀为一条 summary 记忆（持久、可召回）。
//   - AddMemory：外部写入带 kind 的分层记忆（semantic/episodic/procedural）。
//   - SearchMemory：语义/关键词召回记忆。
// 只依赖内核底座（orchestrator/core.types）与装配层（assembly），不 import 能力插件。

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cjintao133-cell/star-river/kernel/core/types"
	"github.com/cjintao133-cell/star-river/kernel/orchestrator"
)

// loadHistory 拉会话历史为时间正序的 []types.Message。
func loadHistory(sessionID string, limit int) ([]types.Message, string) {
	if !kernelReady() {
		return nil, "ERR:kernel not initialized"
	}
	if limit <= 0 {
		limit = 1000
	}
	page, err := kbRepo.ListMessages(context.Background(), sessionID, limit, 0)
	if err != nil {
		return nil, "ERR:" + err.Error()
	}
	hist := make([]types.Message, 0, len(page.Records))
	for i := len(page.Records) - 1; i >= 0; i-- { // repo 返回 desc，翻正
		hist = append(hist, page.Records[i].Message)
	}
	return hist, ""
}

func msgText(m types.Message) string {
	var b strings.Builder
	for _, blk := range m.Blocks {
		if blk.Type == types.BlockText {
			b.WriteString(blk.Text)
		}
	}
	return b.String()
}

type ctxMsgOut struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// BuildChatContext 返回压缩后的聊天上下文（JSON）。
// JSON: {digest, systemAddendum, keptCount, foldedCount, charsBefore, charsAfter, messages:[{role,text}]}
func BuildChatContext(sessionID string, keepRecent, budgetChars int) string {
	hist, e := loadHistory(sessionID, 0)
	if e != "" {
		return e
	}
	res := orchestrator.Compact(hist, keepRecent, budgetChars, 0, nil)
	msgs := make([]ctxMsgOut, 0, len(res.Kept))
	for _, m := range res.Kept {
		msgs = append(msgs, ctxMsgOut{Role: string(m.Role), Text: msgText(m)})
	}
	out := map[string]any{
		"digest":         res.Digest,
		"systemAddendum": orchestrator.CompactSystem(res.Digest),
		"keptCount":      res.KeptCount,
		"foldedCount":    res.FoldedCount,
		"charsBefore":    res.CharsBefore,
		"charsAfter":     res.CharsAfter,
		"messages":       msgs,
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// CompactSession 同 BuildChatContext 压缩，并把摘要沉淀为一条 summary 记忆。
// 返回 JSON：BuildChatContext 的字段 + memory_id。
func CompactSession(sessionID string, keepRecent, budgetChars int) string {
	hist, e := loadHistory(sessionID, 0)
	if e != "" {
		return e
	}
	res := orchestrator.Compact(hist, keepRecent, budgetChars, 0, nil)
	memID := ""
	if strings.TrimSpace(res.Digest) != "" {
		asmMu.Lock()
		ensureAssembly()
		memID = addMemoryLocked("【对话摘要】"+res.Digest, "summary", 0.6, 0.0)
		asmMu.Unlock()
	}
	msgs := make([]ctxMsgOut, 0, len(res.Kept))
	for _, m := range res.Kept {
		msgs = append(msgs, ctxMsgOut{Role: string(m.Role), Text: msgText(m)})
	}
	out := map[string]any{
		"digest":         res.Digest,
		"systemAddendum": orchestrator.CompactSystem(res.Digest),
		"keptCount":      res.KeptCount,
		"foldedCount":    res.FoldedCount,
		"charsBefore":    res.CharsBefore,
		"charsAfter":     res.CharsAfter,
		"messages":       msgs,
		"memory_id":      memID,
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// AddMemory 写入一条分层记忆（kind: semantic|episodic|procedural）。返回记忆 id / "ERR:..."。
func AddMemory(kind, text string, importance float64) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	if strings.TrimSpace(text) == "" {
		return "ERR:empty text"
	}
	if importance < 0 {
		importance = 0
	} else if importance > 1 {
		importance = 1
	}
	asmMu.Lock()
	ensureAssembly()
	id := addMemoryLocked(text, kind, importance, 0.0)
	asmMu.Unlock()
	if id == "" {
		return "ERR:encode failed"
	}
	return id
}

// SearchMemory 召回记忆（语义向量优先、回退关键词）。返回 JSON 数组 [{id,text,score,kind}]。
func SearchMemory(query string, k int) string {
	asmMu.Lock()
	ensureAssembly()
	if k <= 0 {
		k = 5
	}
	hits := recallLocked(query, k)
	kindByID := make(map[string]string, len(asmRecs))
	for _, r := range asmRecs {
		kindByID[r.ID] = r.Kind
	}
	asmMu.Unlock()
	out := make([]map[string]any, 0, len(hits))
	for _, h := range hits {
		out = append(out, map[string]any{
			"id":    h.Memory.ID,
			"text":  h.Memory.Text,
			"score": h.Score,
			"kind":  kindByID[h.Memory.ID],
		})
	}
	b, _ := json.Marshal(out)
	return string(b)
}
