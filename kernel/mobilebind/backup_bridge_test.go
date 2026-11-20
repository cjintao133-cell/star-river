package mobilebind

// 备份导出/导入往返：建会话 → 导出 → 清库 → 导入 → 会话恢复；坏归档报错。

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupExportImportRoundtrip(t *testing.T) {
	db := filepath.Join(t.TempDir(), "bk.db")
	InitKernel(db, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	sid := ""
	if js := CreateSession("", "备份测试会话", "m"); strings.HasPrefix(js, "{") {
		var m map[string]string
		_ = json.Unmarshal([]byte(js), &m)
		sid = m["session_id"]
	}
	if sid == "" { t.Fatalf("create session failed") }
	AppendMessage(sid, "user", "要备份的消息内容")

	arch := BackupExport()
	if len(arch) == 0 { t.Fatalf("导出为空") }

	// 模拟换机：新库导入
	db2 := filepath.Join(t.TempDir(), "bk2.db")
	InitKernel(db2, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if r := BackupImport(arch); !strings.HasPrefix(r, "OK:") {
		t.Fatalf("import 应成功: %s", r)
	}
	list := ListSessions()
	if !strings.Contains(list, "备份测试会话") {
		t.Fatalf("导入后会话应恢复: %s", list)
	}
	msgs := ListMessages(sid, 50)
	if !strings.Contains(msgs, "要备份的消息内容") {
		t.Fatalf("导入后消息应恢复: %s", msgs)
	}
}

func TestBackupImportBadArchive(t *testing.T) {
	InitKernel(filepath.Join(t.TempDir(), "b3.db"), base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if r := BackupImport([]byte("not a zip")); !strings.HasPrefix(r, "ERR:") {
		t.Fatalf("坏归档应报错: %s", r)
	}
	if r := BackupImport(nil); !strings.HasPrefix(r, "ERR:") {
		t.Fatalf("空归档应报错: %s", r)
	}
}

