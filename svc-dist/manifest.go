// Package svcdist 分发服务（T-M3-05）：清单签名 + APK/资源更新通道 + 完整性校验。
//
// 安全模型：
//   - 清单 Manifest 由发行方用 ed25519 私钥签名（Sign）；客户端内置公钥校验（Verify）。
//   - 每个 Asset 带 SHA256（hex）；下载后用 VerifyAsset 校验完整性。
//   - 篡改清单 → 签名校验失败（ErrBadSignature）；篡改包 → 完整性失败（ErrTampered）。
//
// ed25519 为标准库（crypto/ed25519），零新依赖。签名/更新链属红线 7 → REVIEW-NEEDED。
package svcdist

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
)

// 错误。
var (
	ErrNoAssets     = errors.New("svcdist: manifest has no assets")
	ErrEmptyVersion = errors.New("svcdist: empty version")
	ErrBadSignature = errors.New("svcdist: signature verification failed")
	ErrTampered     = errors.New("svcdist: integrity check failed (tampered or corrupt)")
	ErrAssetMissing = errors.New("svcdist: asset not found in manifest")
	ErrBadKey       = errors.New("svcdist: invalid ed25519 key")
)

// Asset 一个分发产物（APK / 人设包 / 主题包等）。
type Asset struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"` // 十六进制
	Size   int64  `json:"size"`
}

// Manifest 版本清单。Signature 覆盖 Version/Channel/MinSupported/ReleasedAt/Assets。
type Manifest struct {
	Version      string  `json:"version"`
	Channel      string  `json:"channel"`
	MinSupported string  `json:"min_supported"`
	ReleasedAt   int64   `json:"released_at"` // UnixMilli
	Assets       []Asset `json:"assets"`
	Signature    string  `json:"signature"` // base64(ed25519 签名)
}

// payload 用于签名的规范化载荷（不含 Signature，字段固定顺序，json.Marshal 确定性）。
type payload struct {
	Version      string  `json:"version"`
	Channel      string  `json:"channel"`
	MinSupported string  `json:"min_supported"`
	ReleasedAt   int64   `json:"released_at"`
	Assets       []Asset `json:"assets"`
}

func (m *Manifest) payloadBytes() []byte {
	b, _ := json.Marshal(payload{
		Version:      m.Version,
		Channel:      m.Channel,
		MinSupported: m.MinSupported,
		ReleasedAt:   m.ReleasedAt,
		Assets:       m.Assets,
	})
	return b
}

// Validate 基本结构校验（签名之外）。
func (m *Manifest) Validate() error {
	if m.Version == "" {
		return ErrEmptyVersion
	}
	if len(m.Assets) == 0 {
		return ErrNoAssets
	}
	for _, a := range m.Assets {
		if a.Name == "" || a.SHA256 == "" {
			return errors.New("svcdist: asset missing name/sha256")
		}
	}
	return nil
}

// Sign 用发行方私钥签名清单（写入 m.Signature）。先 Validate。
func (m *Manifest) Sign(priv ed25519.PrivateKey) error {
	if len(priv) != ed25519.PrivateKeySize {
		return ErrBadKey
	}
	if err := m.Validate(); err != nil {
		return err
	}
	sig := ed25519.Sign(priv, m.payloadBytes())
	m.Signature = base64.StdEncoding.EncodeToString(sig)
	return nil
}

// Verify 用发行方公钥校验清单签名。
func (m *Manifest) Verify(pub ed25519.PublicKey) error {
	if len(pub) != ed25519.PublicKeySize {
		return ErrBadKey
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return ErrBadSignature
	}
	if !ed25519.Verify(pub, m.payloadBytes(), sig) {
		return ErrBadSignature
	}
	return nil
}

// SHA256Hex 计算数据 SHA256 十六进制（供打包时填 Asset.SHA256）。
func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// VerifyAsset 依清单中同名 Asset 校验数据完整性（SHA256）。未找到返回 ErrAssetMissing。
func (m *Manifest) VerifyAsset(name string, data []byte) error {
	for _, a := range m.Assets {
		if a.Name == name {
			got := SHA256Hex(data)
			if !equalFoldHex(got, a.SHA256) {
				return ErrTampered
			}
			if a.Size > 0 && int64(len(data)) != a.Size {
				return ErrTampered
			}
			return nil
		}
	}
	return ErrAssetMissing
}

// AssetByName 查资产。
func (m *Manifest) AssetByName(name string) (Asset, bool) {
	for _, a := range m.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// equalFoldHex 无分配的小写十六进制比较。
func equalFoldHex(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'F' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'F' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
