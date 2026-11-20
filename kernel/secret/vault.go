// Package secret 密钥/敏感字段安全存储的统一接口（T-M1-09）。
//
// 红线 3/4：key 与敏感字段只经本包 Vault 读写，绝不落 SQLite 明文、不进日志/错误。
// 持久化层（store）只存 secret 的 id 引用（如 "openai.key"），绝不存 secret 本体；
// 运行时由 Vault 从平台安全存储取回后再交给适配器（适配器只在内存用，用后即弃）。
//
// 平台后端（统一接口，后端可插拔）：
//   - Android：Keystore。Keystore 的密钥在安全硬件里，加解密经 JNI/Kotlin 侧完成，
//     通过 Delegate 注入本包（见 vault_delegate.go，接线属 kernel/mobilebind 门面侧）。
//   - Windows：DPAPI（vault_windows.go，当前用户范围）。
//   - 测试/开发：MemVault（内存，非持久，仅测试与本地开发）。
package secret

import (
	"errors"
	"sync"
)

// 哨兵错误。
var (
	ErrNotFound    = errors.New("secret: not found")
	ErrEmptyID     = errors.New("secret: empty id")
	ErrEmptySecret = errors.New("secret: empty secret")
)

// Vault 密钥/敏感字段安全存储统一接口。各平台后端实现同一接口，
// 上层（设置页/适配器装配）只依赖此接口，不感知平台差异。
type Vault interface {
	// Set 安全保存 id 对应的 secret（覆盖语义）。
	Set(id string, secret []byte) error
	// Get 取回 secret；不存在返回 ErrNotFound。
	Get(id string) ([]byte, error)
	// Delete 删除；不存在不报错（幂等）。
	Delete(id string) error
}

// MemVault 内存实现（并发安全，非持久）。仅用于测试与本地开发；
// 进程退出即丢失，不得用于生产密钥持久化。
type MemVault struct {
	mu sync.RWMutex
	m  map[string][]byte
}

// NewMemVault 建空内存 Vault。
func NewMemVault() *MemVault { return &MemVault{m: make(map[string][]byte)} }

// Set 保存（存储副本，避免调用方后续修改影响内部状态）。
func (v *MemVault) Set(id string, secret []byte) error {
	if id == "" {
		return ErrEmptyID
	}
	if len(secret) == 0 {
		return ErrEmptySecret
	}
	cp := append([]byte(nil), secret...)
	v.mu.Lock()
	v.m[id] = cp
	v.mu.Unlock()
	return nil
}

// Get 取回（返回副本，避免调用方改动内部状态）。
func (v *MemVault) Get(id string) ([]byte, error) {
	if id == "" {
		return nil, ErrEmptyID
	}
	v.mu.RLock()
	s, ok := v.m[id]
	v.mu.RUnlock()
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), s...), nil
}

// Delete 删除，幂等。
func (v *MemVault) Delete(id string) error {
	if id == "" {
		return ErrEmptyID
	}
	v.mu.Lock()
	delete(v.m, id)
	v.mu.Unlock()
	return nil
}
