package secret

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// T-M1-09 验收核心：key 不落 SQLite 明文（单测 + 数据库文件字符串扫描双重断言）。
// store（SQLite）尚未建，本测试用临时文件模拟"数据库文件"，
// 验证按契约（只存 SecretRef 引用、secret 只活 Vault）落盘的记录里绝无 secret 明文。

// persistedConfig 模拟将来落库的会话/端点配置：只存 KeyRef（id），不存 secret 本体。
type persistedConfig struct {
	Name      string    `json:"name"`
	BaseURL   string    `json:"base_url"`
	Model     string    `json:"model"`
	APIKeyRef SecretRef `json:"api_key_ref"`
}

func TestNoSecretPlaintextOnDisk(t *testing.T) {
	secretStr := "sk-SECRET-DO-NOT-PERSIST-1234567890"
	v := NewMemVault()
	if err := v.Set("endpoint.openai.key", []byte(secretStr)); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// 按契约构造要落盘的记录：只写 SecretRef（id），不写 secret。
	rec := persistedConfig{
		Name:      "openai-main",
		BaseURL:   "https://api.example/v1",
		Model:     "gpt-4o",
		APIKeyRef: SecretRef("endpoint.openai.key"),
	}
	blob, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}

	// 写入临时"数据库文件"。
	dbPath := filepath.Join(t.TempDir(), "star-river.db")
	if err := os.WriteFile(dbPath, blob, 0o600); err != nil {
		t.Fatal(err)
	}

	// 断言 1：扫描"数据库文件"原始字节，绝不能含 secret 明文。
	onDisk, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(onDisk, []byte(secretStr)) {
		t.Fatalf("红线4 违规：secret 明文落盘！文件内容=%s", onDisk)
	}
	if !bytes.Contains(onDisk, []byte("endpoint.openai.key")) {
		t.Errorf("引用 id 应在盘上（仅引用，非 secret）：got %s", onDisk)
	}

	// 断言 2：secret 只活在 Vault 里，经 id 能取回。
	got, err := v.Get(rec.APIKeyRef.ID())
	if err != nil || string(got) != secretStr {
		t.Fatalf("Vault 取回失败: %q, %v", got, err)
	}
}

// 反向对照：证明上面的"扫描断言"不是空断言——若真的把 secret 塞进落盘结构，扫描必须抓到。
func TestScannerCatchesPlantedSecret(t *testing.T) {
	secretStr := "sk-PLANTED-LEAK"
	// 故意违规：把 secret 明文写进"落盘"结构（错误示范）。
	bad := map[string]string{"api_key": secretStr}
	blob, _ := json.Marshal(bad)
	dbPath := filepath.Join(t.TempDir(), "bad.db")
	os.WriteFile(dbPath, blob, 0o600)

	onDisk, _ := os.ReadFile(dbPath)
	if !bytes.Contains(onDisk, []byte(secretStr)) {
		t.Fatal("反向对照失败：扫描器应能抓到明文")
	}
}

// 日志/错误落盘前 Scrub 兜底：即使某条错误字符串意外带了 secret，Scrub 后扫盘也无明文。
func TestScrubBeforePersist(t *testing.T) {
	secretStr := "sk-SECRET-IN-ERROR"
	logline := []byte("request failed: Authorization=Bearer " + secretStr + " (retry)")
	cleaned := Scrub(logline, secretStr)

	logPath := filepath.Join(t.TempDir(), "app.log")
	os.WriteFile(logPath, cleaned, 0o600)
	onDisk, _ := os.ReadFile(logPath)
	if bytes.Contains(onDisk, []byte(secretStr)) {
		t.Fatalf("红线3 违规：secret 落进日志！%s", onDisk)
	}
	if !bytes.Contains(onDisk, []byte("[REDACTED]")) {
		t.Errorf("应替换为 [REDACTED]: %s", onDisk)
	}
}
