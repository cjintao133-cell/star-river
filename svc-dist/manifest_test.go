package svcdist

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"testing"
)

func keypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func sampleManifest(t *testing.T, priv ed25519.PrivateKey) *Manifest {
	t.Helper()
	apk := []byte("FAKE-APK-CONTENT-v1")
	m := &Manifest{
		Version:      "0.1.0",
		Channel:      "stable",
		MinSupported: "0.0.1",
		ReleasedAt:   1720000000000,
		Assets: []Asset{
			{Name: "app.apk", URL: "https://cdn/app.apk", SHA256: SHA256Hex(apk), Size: int64(len(apk))},
		},
	}
	if err := m.Sign(priv); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	return m
}

func TestSignVerify(t *testing.T) {
	pub, priv := keypair(t)
	m := sampleManifest(t, priv)
	if err := m.Verify(pub); err != nil {
		t.Fatalf("verify valid = %v", err)
	}
	// 用错公钥 → 失败。
	otherPub, _ := keypair(t)
	if err := m.Verify(otherPub); err != ErrBadSignature {
		t.Errorf("wrong key verify = %v, want ErrBadSignature", err)
	}
}

// 篡改清单任一字段 → 验签失败。
func TestTamperedManifestRejected(t *testing.T) {
	pub, priv := keypair(t)
	m := sampleManifest(t, priv)
	m.Version = "9.9.9" // 篡改
	if err := m.Verify(pub); err != ErrBadSignature {
		t.Errorf("tampered manifest verify = %v, want ErrBadSignature", err)
	}
}

func TestVerifyAssetIntegrity(t *testing.T) {
	_, priv := keypair(t)
	m := sampleManifest(t, priv)
	good := []byte("FAKE-APK-CONTENT-v1")
	if err := m.VerifyAsset("app.apk", good); err != nil {
		t.Errorf("good asset = %v", err)
	}
	tampered := append([]byte(nil), good...)
	tampered[0] ^= 0xFF
	if err := m.VerifyAsset("app.apk", tampered); err != ErrTampered {
		t.Errorf("tampered asset = %v, want ErrTampered", err)
	}
	if err := m.VerifyAsset("ghost", good); err != ErrAssetMissing {
		t.Errorf("missing asset = %v, want ErrAssetMissing", err)
	}
	// 大小不符也算篡改。
	if err := m.VerifyAsset("app.apk", append(good, 'x')); err != ErrTampered {
		t.Errorf("size mismatch = %v, want ErrTampered", err)
	}
}

func TestManifestValidate(t *testing.T) {
	if (&Manifest{}).Validate() != ErrEmptyVersion {
		t.Error("空 version 应报错")
	}
	if (&Manifest{Version: "1"}).Validate() != ErrNoAssets {
		t.Error("无资产应报错")
	}
	if (&Manifest{Version: "1", Assets: []Asset{{Name: "", SHA256: ""}}}).Validate() == nil {
		t.Error("资产缺字段应报错")
	}
}

func TestSignBadKey(t *testing.T) {
	m := &Manifest{Version: "1", Assets: []Asset{{Name: "a", SHA256: "x"}}}
	if err := m.Sign(ed25519.PrivateKey([]byte("short"))); err != ErrBadKey {
		t.Errorf("bad key Sign = %v", err)
	}
	if err := m.Verify(ed25519.PublicKey([]byte("short"))); err != ErrBadKey {
		t.Errorf("bad key Verify = %v", err)
	}
}

// 内存 Fetcher（零网络）。
type memFetcher struct {
	manifest []byte
	assets   map[string][]byte
}

func (f *memFetcher) FetchManifest(ctx context.Context) ([]byte, error) { return f.manifest, nil }
func (f *memFetcher) FetchAsset(ctx context.Context, url string) ([]byte, error) {
	if d, ok := f.assets[url]; ok {
		return d, nil
	}
	return nil, ErrAssetMissing
}

// 全流程：验签清单 → 下载资产 → 完整性通过；篡改包 → 拒绝。
func TestUpdateChannelEndToEnd(t *testing.T) {
	pub, priv := keypair(t)
	m := sampleManifest(t, priv)
	manBytes := mustJSON(t, m)
	apk := []byte("FAKE-APK-CONTENT-v1")

	f := &memFetcher{manifest: manBytes, assets: map[string][]byte{"https://cdn/app.apk": apk}}
	uc := &UpdateChecker{Fetcher: f, PubKey: pub, Channel: "stable"}

	got, err := uc.Manifest(context.Background())
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	data, err := uc.Download(context.Background(), got, "app.apk")
	if err != nil || string(data) != string(apk) {
		t.Fatalf("Download = %q, %v", data, err)
	}

	// 篡改下载到位的包 → 拒绝。
	f.assets["https://cdn/app.apk"] = []byte("TAMPERED-APK")
	if _, err := uc.Download(context.Background(), got, "app.apk"); err != ErrTampered {
		t.Errorf("tampered download = %v, want ErrTampered", err)
	}
}

// 频道不匹配 → 拒绝。
func TestUpdateChannelMismatch(t *testing.T) {
	pub, priv := keypair(t)
	m := sampleManifest(t, priv) // channel=stable
	uc := &UpdateChecker{Fetcher: &memFetcher{manifest: mustJSON(t, m)}, PubKey: pub, Channel: "beta"}
	if _, err := uc.Manifest(context.Background()); err != ErrBadSignature {
		t.Errorf("channel mismatch = %v, want ErrBadSignature", err)
	}
}

// 伪造清单（无有效签名）→ 拒绝。
func TestUpdateRejectsForged(t *testing.T) {
	pub, _ := keypair(t)
	_, attackerPriv := keypair(t)
	m := sampleManifest(t, attackerPriv) // 攻击者私钥签的
	uc := &UpdateChecker{Fetcher: &memFetcher{manifest: mustJSON(t, m)}, PubKey: pub}
	if _, err := uc.Manifest(context.Background()); err != ErrBadSignature {
		t.Errorf("forged manifest = %v, want ErrBadSignature", err)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestVerifyMalformedSignature(t *testing.T) {
	pub, _ := keypair(t)
	m := &Manifest{Version: "1", Channel: "stable", Assets: []Asset{{Name: "a", SHA256: "00"}}, Signature: "!!!not base64!!!"}
	if err := m.Verify(pub); err != ErrBadSignature {
		t.Errorf("malformed sig = %v, want ErrBadSignature", err)
	}
}

type errFetcher struct{}

func (errFetcher) FetchManifest(ctx context.Context) ([]byte, error) { return nil, ErrAssetMissing }
func (errFetcher) FetchAsset(ctx context.Context, url string) ([]byte, error) {
	return nil, ErrAssetMissing
}

func TestUpdateFetchError(t *testing.T) {
	pub, _ := keypair(t)
	uc := &UpdateChecker{Fetcher: errFetcher{}, PubKey: pub}
	if _, err := uc.Manifest(context.Background()); err == nil {
		t.Error("fetch 错误应上抛")
	}
}

func TestDownloadMissingAsset(t *testing.T) {
	pub, priv := keypair(t)
	m := sampleManifest(t, priv)
	uc := &UpdateChecker{Fetcher: &memFetcher{}, PubKey: pub}
	if _, err := uc.Download(context.Background(), m, "ghost"); err != ErrAssetMissing {
		t.Errorf("download missing = %v, want ErrAssetMissing", err)
	}
}

func TestAssetByName(t *testing.T) {
	_, priv := keypair(t)
	m := sampleManifest(t, priv)
	if a, ok := m.AssetByName("app.apk"); !ok || a.Name != "app.apk" {
		t.Errorf("AssetByName found=%v", ok)
	}
	if _, ok := m.AssetByName("nope"); ok {
		t.Error("不存在应 false")
	}
}
