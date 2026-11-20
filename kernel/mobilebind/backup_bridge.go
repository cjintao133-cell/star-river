package mobilebind

// 备份桥（backup）：把 kernel/backup 的导出/导入暴露给 UI（换机/防丢）。
// 归档含 sessions/messages(model_stats/app_meta) 的密文与元数据；导入为替换式。
// 导入后重置派生单例（情绪/记忆/主动/工具索引/MCP），避免指向旧数据。

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"star-river/kernel/backup"
)

// BackupExport 导出当前库为备份归档字节（zip）。无内核或失败返回 nil。
func BackupExport() []byte {
	st := storeRef()
	if st == nil {
		return nil
	}
	var buf bytes.Buffer
	_, err := backup.WriteArchive(context.Background(), st, &buf, backup.Options{ExportedAt: time.Now().UnixMilli()})
	if err != nil {
		return nil
	}
	return buf.Bytes()
}

// BackupImport 从备份归档字节替换式导入。成功返回 "OK:{...}"，失败 "ERR:..."。
func BackupImport(data []byte) string {
	st := storeRef()
	if st == nil {
		return "ERR:kernel not initialized"
	}
	if len(data) == 0 {
		return "ERR:empty archive"
	}
	res, err := backup.ReadArchive(context.Background(), st, data)
	if err != nil {
		return "ERR:" + err.Error()
	}
	// 数据已替换：重置派生状态，避免旧引擎/索引指向旧库。
	resetAssembly()
	resetProactive()
	tdMu.Lock()
	tdIdx = nil
	tdMu.Unlock()
	mcpMu.Lock()
	mcpMgr = nil
	mcpMu.Unlock()
	info := map[string]any{"format": res.Manifest.Format, "version": res.Manifest.Version, "extras": len(res.Extras)}
	b, _ := json.Marshal(info)
	return "OK:" + string(b)
}
