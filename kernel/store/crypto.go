// Field-level encryption for store columns (T-M2-02).
//
// AES-256-GCM，标准库实现，零新依赖（红线 1）。
// 红线 4：敏感列（如 messages.content）以此加解密后落库，磁盘只存密文。
// 批准优化#5：AAD 绑定「行 id + 字段名」，防跨行/跨字段密文搬运。
//
// 密钥来源（红线 4）：经 secret.Vault（Android Keystore / Windows DPAPI）取回主密钥；
// 首次不存在则生成 32 字节随机主密钥并写回 Vault。密文格式 nonce(12) || ciphertext || tag。
package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"

	"star-river/kernel/secret"
)

// 加密层哨兵错误。
var (
	ErrBadKeyLen      = errors.New("store: field cipher key must be 32 bytes (AES-256)")
	ErrCipherTooShort = errors.New("store: ciphertext shorter than nonce")
	ErrDecrypt        = errors.New("store: decrypt failed (wrong key or tampered AAD/ciphertext)")
)

// keyLen AES-256 主密钥长度。
const keyLen = 32

// FieldCipher 字段级 AES-GCM 加解密器。并发安全（AEAD 只读，nonce 每次独立生成）。
type FieldCipher struct {
	aead cipher.AEAD
}

// NewFieldCipher 用 32 字节主密钥构造。
func NewFieldCipher(key []byte) (*FieldCipher, error) {
	if len(key) != keyLen {
		return nil, ErrBadKeyLen
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &FieldCipher{aead: aead}, nil
}

// FieldCipherFromVault 从 secret.Vault 取主密钥构造 FieldCipher（首次自动生成并持久化）。
// 主密钥存于平台安全存储（Keystore/DPAPI），本函数只在内存用，不落明文。
func FieldCipherFromVault(v secret.Vault, id string) (*FieldCipher, error) {
	if id == "" {
		return nil, secret.ErrEmptyID
	}
	key, err := v.Get(id)
	if errors.Is(err, secret.ErrNotFound) {
		key = make([]byte, keyLen)
		if _, e := io.ReadFull(rand.Reader, key); e != nil {
			return nil, e
		}
		if e := v.Set(id, key); e != nil {
			return nil, e
		}
	} else if err != nil {
		return nil, err
	}
	return NewFieldCipher(key)
}

// Seal 加密 plaintext，aad 参与完整性绑定。返回 nonce||ciphertext||tag。
func (c *FieldCipher) Seal(plaintext, aad []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return c.aead.Seal(nonce, nonce, plaintext, aad), nil
}

// Open 解密。aad 与加密时不一致（跨行/跨字段搬运）或密文被篡改 → ErrDecrypt。
func (c *FieldCipher) Open(sealed, aad []byte) ([]byte, error) {
	ns := c.aead.NonceSize()
	if len(sealed) < ns {
		return nil, ErrCipherTooShort
	}
	nonce, ct := sealed[:ns], sealed[ns:]
	pt, err := c.aead.Open(nil, nonce, ct, aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return pt, nil
}

// fieldAAD 构造行/字段绑定 AAD（用 0x00 分隔避免拼接歧义）。
func fieldAAD(rowID, field string) []byte {
	return []byte(rowID + "\x00" + field)
}

// SealField 便捷：以「行 id + 字段名」为 AAD 加密。
func (c *FieldCipher) SealField(plaintext []byte, rowID, field string) ([]byte, error) {
	return c.Seal(plaintext, fieldAAD(rowID, field))
}

// OpenField 便捷：以「行 id + 字段名」为 AAD 解密。
func (c *FieldCipher) OpenField(sealed []byte, rowID, field string) ([]byte, error) {
	return c.Open(sealed, fieldAAD(rowID, field))
}

// Redact 便捷封装 secret.Scrub（复用同一密钥字面脱敏），供 store 层落日志前清洗。
func Redact(b []byte, secretStr string) []byte {
	return secret.Scrub(b, secretStr)
}
