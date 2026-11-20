//go:build windows

package secret

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// 说明：Windows DPAPI（当前用户范围）实现。
// 仅 Windows 构建可见；Linux 构建机无法运行验证（runtime DEFERRED），
// 密码学实现属红线 7 的"加密实现"类 diff，标 REVIEW-NEEDED，外部评审前不视为终稿。

var (
	modCrypt32  = syscall.NewLazyDLL("crypt32.dll")
	modKernel32 = syscall.NewLazyDLL("kernel32.dll")

	procCryptProtectData   = modCrypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = modCrypt32.NewProc("CryptUnprotectData")
	procLocalFree          = modKernel32.NewProc("LocalFree")
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

func newBlob(b []byte) (dataBlob, bool) {
	if len(b) == 0 {
		return dataBlob{}, false
	}
	return dataBlob{cbData: uint32(len(b)), pbData: &b[0]}, true
}

// sanitizeID 只保留 [a-zA-Z0-9_-]，其余替换为下划线，杜绝路径穿越。
func sanitizeID(id string) string {
	b := make([]byte, 0, len(id))
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '-':
			b = append(b, c)
		default:
			b = append(b, '_')
		}
	}
	if len(b) == 0 {
		return "_"
	}
	return string(b)
}

// DPAPIVault 用 DPAPI 加密 secret，密文落 dir 目录下一个文件（id 映射文件名）。
// DPAPI 负责加密，文件系统负责持久化；两者组合构成当前用户范围的密钥库。
type DPAPIVault struct {
	dir string
}

// NewDPAPIVault 指定密文存储目录（建议 %APPDATA%/star-river/secrets）。
func NewDPAPIVault(dir string) *DPAPIVault { return &DPAPIVault{dir: dir} }

func (v *DPAPIVault) file(id string) string {
	return filepath.Join(v.dir, sanitizeID(id)+".bin")
}

// Set 加密并落盘。
func (v *DPAPIVault) Set(id string, secret []byte) error {
	if id == "" {
		return ErrEmptyID
	}
	if len(secret) == 0 {
		return ErrEmptySecret
	}
	if err := os.MkdirAll(v.dir, 0o700); err != nil {
		return err
	}
	ct, err := dpapiProtect(secret)
	if err != nil {
		return err
	}
	return os.WriteFile(v.file(id), ct, 0o600)
}

// Get 读文件并解密。
func (v *DPAPIVault) Get(id string) ([]byte, error) {
	if id == "" {
		return nil, ErrEmptyID
	}
	ct, err := os.ReadFile(v.file(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return dpapiUnprotect(ct)
}

// Delete 删密文文件，幂等。
func (v *DPAPIVault) Delete(id string) error {
	if id == "" {
		return ErrEmptyID
	}
	if err := os.Remove(v.file(id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func dpapiProtect(plain []byte) ([]byte, error) {
	in, ok := newBlob(plain)
	if !ok {
		return nil, ErrEmptySecret
	}
	var out dataBlob
	// BOOL CryptProtectData(DATA_BLOB*, LPCWSTR, DATA_BLOB*, PVOID, PROMPT*, DWORD, DATA_BLOB*)
	r1, _, e := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&in)), // pDataIn
		0,                            // szDataDescr
		0,                            // pOptionalEntropy
		0,                            // pvReserved
		0,                            // pPromptStruct
		0,                            // dwFlags（0=当前用户范围）
		uintptr(unsafe.Pointer(&out)),
	)
	if r1 == 0 {
		return nil, e
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	return append([]byte(nil), unsafe.Slice(out.pbData, int(out.cbData))...), nil
}

func dpapiUnprotect(ct []byte) ([]byte, error) {
	in, ok := newBlob(ct)
	if !ok {
		return nil, ErrNotFound
	}
	var out dataBlob
	r1, _, e := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&out)),
	)
	if r1 == 0 {
		return nil, e
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	return append([]byte(nil), unsafe.Slice(out.pbData, int(out.cbData))...), nil
}

var _ Vault = (*DPAPIVault)(nil)
