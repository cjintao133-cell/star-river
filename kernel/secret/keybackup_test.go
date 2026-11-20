package secret

import (
	"bytes"
	"errors"
	"testing"
)

func TestKeyBackupRoundtrip(t *testing.T) {
	plaintext := []byte("sk-这是-密钥材料-0123456789")
	password := []byte("correct horse battery staple")
	blob, err := EncryptBackup(plaintext, password, kbMinIter)
	if err != nil {
		t.Fatal(err)
	}
	// 格式头
	if string(blob[:4]) != keyBackupMagic {
		t.Errorf("magic 不对: %q", blob[:4])
	}
	got, err := DecryptBackup(blob, password)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("往返不一致: %q", got)
	}
}

func TestKeyBackupWrongPassword(t *testing.T) {
	blob, err := EncryptBackup([]byte("secret"), []byte("right"), kbMinIter)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptBackup(blob, []byte("wrong")); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("错误密码应报 ErrWrongPassword, got %v", err)
	}
}

func TestKeyBackupTampered(t *testing.T) {
	blob, _ := EncryptBackup([]byte("secret"), []byte("pw"), kbMinIter)
	// 翻最后一字节（密文）→ GCM 校验失败
	blob[len(blob)-1] ^= 0xFF
	if _, err := DecryptBackup(blob, []byte("pw")); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("篡改应报 ErrWrongPassword, got %v", err)
	}
}

func TestKeyBackupBadMagic(t *testing.T) {
	blob, _ := EncryptBackup([]byte("s"), []byte("p"), kbMinIter)
	blob[0] = 'X'
	if _, err := DecryptBackup(blob, []byte("p")); !errors.Is(err, ErrKeyBackupFormat) {
		t.Fatalf("坏 magic 应报 ErrKeyBackupFormat, got %v", err)
	}
}

func TestKeyBackupTooShort(t *testing.T) {
	if _, err := DecryptBackup([]byte("SRKB"), []byte("p")); !errors.Is(err, ErrKeyBackupFormat) {
		t.Fatalf("过短应报 ErrKeyBackupFormat, got %v", err)
	}
}

func TestKeyBackupLowIterRejected(t *testing.T) {
	if _, err := EncryptBackup([]byte("s"), []byte("p"), 100); !errors.Is(err, ErrKeyBackupFormat) {
		t.Fatalf("低迭代应报 ErrKeyBackupFormat, got %v", err)
	}
	// 篡改头部把 iters 改低 → 解密报 ErrKeyBackupFormat
	blob, _ := EncryptBackup([]byte("s"), []byte("p"), kbMinIter)
	blob[5], blob[6], blob[7], blob[8] = 0, 0, 0, 1 // iters=1
	if _, err := DecryptBackup(blob, []byte("p")); !errors.Is(err, ErrKeyBackupFormat) {
		t.Fatalf("低迭代头应报 ErrKeyBackupFormat, got %v", err)
	}
}

func TestKeyBackupEmptyPassword(t *testing.T) {
	if _, err := EncryptBackup([]byte("s"), nil, kbMinIter); err == nil {
		t.Error("空密码加密应报错")
	}
	blob, _ := EncryptBackup([]byte("s"), []byte("p"), kbMinIter)
	if _, err := DecryptBackup(blob, nil); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("空密码解密应报 ErrWrongPassword, got %v", err)
	}
}

func TestKeyBackupRandomized(t *testing.T) {
	// 同明文同密码两次加密应产生不同 blob（salt/nonce 随机）。
	b1, _ := EncryptBackup([]byte("s"), []byte("p"), kbMinIter)
	b2, _ := EncryptBackup([]byte("s"), []byte("p"), kbMinIter)
	if bytes.Equal(b1, b2) {
		t.Error("两次加密应因随机 salt/nonce 不同")
	}
}

func TestKeyBackupDefaultIter(t *testing.T) {
	// iters<=0 用默认（较慢，仅一次）。
	blob, err := EncryptBackup([]byte("s"), []byte("p"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptBackup(blob, []byte("p")); err != nil {
		t.Fatalf("默认迭代往返应成功: %v", err)
	}
}
