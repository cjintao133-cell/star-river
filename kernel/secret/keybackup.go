package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// key 备份加密（T-M7-07）：用户密码 → PBKDF2-SHA256 派生密钥 → AES-256-GCM 加密 key 明文。
//
// **REVIEW-NEEDED**（红线 7）：这是密钥材料的加密实现，发布前须外部评审。
// 纯标准库（crypto/pbkdf2、crypto/aes、crypto/cipher），零第三方依赖（红线 1）。
//
// 封装格式（大端）：
//   magic "SRKB" | version(1) | iters(uint32) | salt(16) | nonce(12) | ciphertext+tag
// 密码永不落盘；PBKDF2 迭代次数写入头部以便将来升级后兼容解密。

const (
	keyBackupMagic   = "SRKB"
	keyBackupVersion = byte(1)
	kbSaltLen        = 16
	kbNonceLen       = 12 // GCM 标准 nonce
	kbKeyLen         = 32 // AES-256
	// DefaultKBIter 默认 PBKDF2 迭代次数（≥ OWASP 2023 建议 pbkdf2-sha256 的 600k，此处取 600k）。
	DefaultKBIter = 600_000
	kbMinIter     = 10_000 // 解密时允许的最小迭代（防降级/DoS）
)

// 备份相关错误。
var (
	ErrKeyBackupFormat = errors.New("secret: malformed key backup")
	ErrWrongPassword   = errors.New("secret: wrong password or corrupted key backup")
)

// EncryptBackup 用 password 派生密钥加密 plaintext（通常是串接的 key 材料）返回自描述 blob。
// iters<=0 用 DefaultKBIter。每次调用使用新随机 salt/nonce。
func EncryptBackup(plaintext, password []byte, iters int) ([]byte, error) {
	if len(password) == 0 {
		return nil, errors.New("secret: empty password")
	}
	if iters <= 0 {
		iters = DefaultKBIter
	}
	if iters < kbMinIter {
		return nil, fmt.Errorf("%w: iterations too low", ErrKeyBackupFormat)
	}
	salt := make([]byte, kbSaltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	key, err := pbkdf2.Key(sha256.New, string(password), salt, iters, kbKeyLen)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	// AAD 绑定头部（magic/version/iters/salt），防篡改头部绕过迭代控制。
	header := buildHeader(iters, salt)
	ct := gcm.Seal(nil, nonce, plaintext, header)

	out := make([]byte, 0, len(header)+len(nonce)+len(ct))
	out = append(out, header...)
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

// DecryptBackup 用 password 解密 EncryptBackup 产物。密码错误或数据被篡改返回 ErrWrongPassword。
func DecryptBackup(blob, password []byte) ([]byte, error) {
	if len(password) == 0 {
		return nil, ErrWrongPassword
	}
	// 最小长度：magic(4)+ver(1)+iters(4)+salt(16)+nonce(12)+tag(16)。
	if len(blob) < 4+1+4+kbSaltLen+kbNonceLen+16 {
		return nil, ErrKeyBackupFormat
	}
	if string(blob[:4]) != keyBackupMagic {
		return nil, fmt.Errorf("%w: bad magic", ErrKeyBackupFormat)
	}
	if blob[4] != keyBackupVersion {
		return nil, fmt.Errorf("%w: unsupported version %d", ErrKeyBackupFormat, blob[4])
	}
	iters := int(binary.BigEndian.Uint32(blob[5:9]))
	if iters < kbMinIter {
		return nil, fmt.Errorf("%w: iterations too low", ErrKeyBackupFormat)
	}
	headerLen := 9
	salt := blob[headerLen : headerLen+kbSaltLen]
	headerLen += kbSaltLen
	nonce := blob[headerLen : headerLen+kbNonceLen]
	headerLen += kbNonceLen
	ct := blob[headerLen:]
	header := buildHeader(iters, salt)

	key, err := pbkdf2.Key(sha256.New, string(password), salt, iters, kbKeyLen)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	pt, err := gcm.Open(nil, nonce, ct, header)
	if err != nil {
		return nil, ErrWrongPassword
	}
	return pt, nil
}

// buildHeader magic|version|iters(4BE)|salt。
func buildHeader(iters int, salt []byte) []byte {
	h := make([]byte, 0, 4+1+4+len(salt))
	h = append(h, []byte(keyBackupMagic)...)
	h = append(h, keyBackupVersion)
	var it [4]byte
	binary.BigEndian.PutUint32(it[:], uint32(iters))
	h = append(h, it[:]...)
	h = append(h, salt...)
	return h
}

// 保证 subtle 被使用（常量时间比较工具保留给未来校验场景）。
var _ = subtle.ConstantTimeCompare
