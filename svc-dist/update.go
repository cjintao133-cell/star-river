package svcdist

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
)

// Fetcher 网络取件抽象（生产用 HTTP，测试用内存/静态桩；便于零网络单测）。
type Fetcher interface {
	FetchManifest(ctx context.Context) ([]byte, error)
	FetchAsset(ctx context.Context, url string) ([]byte, error)
}

// UpdateChecker 更新通道：拉清单→验签→（可选频道过滤）；下载资产→完整性校验。
type UpdateChecker struct {
	Fetcher Fetcher
	PubKey  ed25519.PublicKey
	Channel string // 非空则只接受该频道清单
}

// Manifest 拉取并校验清单（JSON 解析→Validate→Verify 签名→频道匹配）。
// 任一环节失败返回错误（篡改/伪造清单一律拒绝）。
func (u *UpdateChecker) Manifest(ctx context.Context) (*Manifest, error) {
	raw, err := u.Fetcher.FetchManifest(ctx)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, ErrBadSignature // 清单不可解析按签名/格式失败处理
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	if err := m.Verify(u.PubKey); err != nil {
		return nil, err
	}
	if u.Channel != "" && m.Channel != u.Channel {
		return nil, ErrBadSignature
	}
	return &m, nil
}

// Download 下载指定资产并校验完整性（SHA256+大小）。清单需先经 Manifest() 验签。
// 返回校验通过的数据。
func (u *UpdateChecker) Download(ctx context.Context, m *Manifest, name string) ([]byte, error) {
	a, ok := m.AssetByName(name)
	if !ok {
		return nil, ErrAssetMissing
	}
	data, err := u.Fetcher.FetchAsset(ctx, a.URL)
	if err != nil {
		return nil, err
	}
	if err := m.VerifyAsset(name, data); err != nil {
		return nil, err
	}
	return data, nil
}
