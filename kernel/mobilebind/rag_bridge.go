package mobilebind

// 本地 RAG 知识库 + 记忆规模桥。知识库/记忆均落本地 SQLite（装 App 首启即建表）。

import (
	"encoding/json"
)

// MemoryCount 长期记忆总条数（不再是 500 上限）。
func MemoryCount() int { return memDBCount() }

// KbIngest 入库一篇知识文档（自动分块 + FTS 索引）。返回 JSON {doc,chunks}。
func KbIngest(docID, title, source, text string) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	if docID == "" {
		return "ERR:docID empty"
	}
	n := kbIngest(docID, title, source, text)
	b, _ := json.Marshal(map[string]any{"doc": docID, "chunks": n})
	return string(b)
}

// KbSearch 知识库检索（FTS5 trigram，中文可用）。返回 JSON 数组。
func KbSearch(query string, k int) string {
	b, _ := json.Marshal(kbSearch(query, k))
	return string(b)
}

// KbStats 知识库统计 {docs, mem}（mem=记忆数）。
func KbStats() string {
	b, _ := json.Marshal(map[string]int{"docs": kbDocCount(), "mem": memDBCount()})
	return string(b)
}
