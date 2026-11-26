package mobilebind

// 消息编辑/删除（持久化）：给 App 侧「编辑重发」「删除某条」提供持久写能力。
// 仅做薄门面，真正的加解密与落库在 store.Repo。

import (
	"context"

	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

// UpdateMessage 原地改写某条消息文本（仅纯文本块）。sessionID+messageID 必须匹配。
// 返回 "OK" / "ERR:<原因>"。
func UpdateMessage(sessionID, messageID, text string) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	if sessionID == "" || messageID == "" {
		return "ERR:empty session/message id"
	}
	if len([]rune(text)) > maxTextRunes {
		return "ERR:text too long"
	}
	blocks := []types.ContentBlock{{Type: types.BlockText, Text: text}}
	if err := kbRepo.UpdateMessage(context.Background(), messageID, sessionID, blocks); err != nil {
		return "ERR:" + err.Error()
	}
	return "OK"
}

// DeleteMessage 删除某条消息（sessionID+messageID 匹配）。返回 "OK" / "ERR:<原因>"。
func DeleteMessage(sessionID, messageID string) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	if sessionID == "" || messageID == "" {
		return "ERR:empty session/message id"
	}
	if err := kbRepo.DeleteMessage(context.Background(), messageID, sessionID); err != nil {
		return "ERR:" + err.Error()
	}
	return "OK"
}
