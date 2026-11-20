package store

import (
	"bytes"
	"errors"
	"testing"

	"star-river/kernel/secret"
)

func newCipher(t *testing.T) *FieldCipher {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	c, err := NewFieldCipher(key)
	if err != nil {
		t.Fatalf("NewFieldCipher: %v", err)
	}
	return c
}

func TestFieldCipherRoundTrip(t *testing.T) {
	c := newCipher(t)
	pt := []byte("这是一条机密消息 secret-content-123")
	sealed, err := c.SealField(pt, "msg-1", "content")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(sealed, pt) {
		t.Fatal("密文不应包含明文")
	}
	got, err := c.OpenField(sealed, "msg-1", "content")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(got, pt) {
		t.Errorf("往返不一致: %q", got)
	}
}

// nonce 随机：同明文同 AAD 两次加密密文不同（防重放/模式分析）。
func TestFieldCipherNonceRandom(t *testing.T) {
	c := newCipher(t)
	pt := []byte("same plaintext")
	s1, _ := c.SealField(pt, "r", "f")
	s2, _ := c.SealField(pt, "r", "f")
	if bytes.Equal(s1, s2) {
		t.Error("两次加密密文相同，nonce 未随机")
	}
}

// AAD 绑定：把 A 行密文搬到 B 行（或换字段）解密必须失败（防跨行/跨字段搬运，批准优化#5）。
func TestFieldCipherAADBindsRowAndField(t *testing.T) {
	c := newCipher(t)
	sealed, _ := c.SealField([]byte("row-A secret"), "row-A", "content")

	if _, err := c.OpenField(sealed, "row-B", "content"); !errors.Is(err, ErrDecrypt) {
		t.Errorf("跨行搬运应失败: %v", err)
	}
	if _, err := c.OpenField(sealed, "row-A", "title"); !errors.Is(err, ErrDecrypt) {
		t.Errorf("跨字段搬运应失败: %v", err)
	}
	// 正确 AAD 才行。
	if _, err := c.OpenField(sealed, "row-A", "content"); err != nil {
		t.Errorf("正确 AAD 应成功: %v", err)
	}
}

func TestFieldCipherTamperDetected(t *testing.T) {
	c := newCipher(t)
	sealed, _ := c.SealField([]byte("tamper me"), "r", "f")
	sealed[len(sealed)-1] ^= 0xFF // 翻转 tag 一字节
	if _, err := c.OpenField(sealed, "r", "f"); !errors.Is(err, ErrDecrypt) {
		t.Errorf("篡改应被检出: %v", err)
	}
}

func TestFieldCipherBadKeyAndShort(t *testing.T) {
	if _, err := NewFieldCipher(make([]byte, 16)); !errors.Is(err, ErrBadKeyLen) {
		t.Errorf("16 字节 key 应报 ErrBadKeyLen")
	}
	c := newCipher(t)
	if _, err := c.OpenField([]byte("short"), "r", "f"); !errors.Is(err, ErrCipherTooShort) {
		t.Errorf("短密文应报 ErrCipherTooShort")
	}
}

// 密钥经 Vault 派生：首次生成并持久化，重开用同一 id 得到同一密钥（能解密旧密文）。
func TestFieldCipherFromVaultPersistsKey(t *testing.T) {
	v := secret.NewMemVault()
	c1, err := FieldCipherFromVault(v, "store.fieldkey")
	if err != nil {
		t.Fatalf("c1: %v", err)
	}
	sealed, _ := c1.SealField([]byte("persist me"), "r", "content")

	// 第二次从同一 Vault 取 → 同一密钥。
	c2, err := FieldCipherFromVault(v, "store.fieldkey")
	if err != nil {
		t.Fatalf("c2: %v", err)
	}
	got, err := c2.OpenField(sealed, "r", "content")
	if err != nil || string(got) != "persist me" {
		t.Errorf("重取密钥应能解密: %q, %v", got, err)
	}
}

func TestRedact(t *testing.T) {
	got := string(Redact([]byte("err key=sk-SECRET-1 bad"), "sk-SECRET-1"))
	if got != "err key=[REDACTED] bad" {
		t.Errorf("Redact = %q", got)
	}
	if string(Redact([]byte("abc"), "")) != "abc" {
		t.Error("空 secret 不应改动")
	}
}
