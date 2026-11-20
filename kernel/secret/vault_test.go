package secret

import (
	"bytes"
	"testing"
)

func TestMemVaultRoundTrip(t *testing.T) {
	v := NewMemVault()
	sec := []byte("sk-SECRET-XYZ")
	if err := v.Set("openai.key", sec); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := v.Get("openai.key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, sec) {
		t.Errorf("Get = %q, want %q", got, sec)
	}
}

func TestMemVaultNotFound(t *testing.T) {
	v := NewMemVault()
	if _, err := v.Get("nope"); err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestMemVaultValidation(t *testing.T) {
	v := NewMemVault()
	if err := v.Set("", []byte("x")); err != ErrEmptyID {
		t.Errorf("empty id Set err = %v", err)
	}
	if err := v.Set("k", nil); err != ErrEmptySecret {
		t.Errorf("empty secret Set err = %v", err)
	}
	if _, err := v.Get(""); err != ErrEmptyID {
		t.Errorf("empty id Get err = %v", err)
	}
	if err := v.Delete(""); err != ErrEmptyID {
		t.Errorf("empty id Delete err = %v", err)
	}
}

func TestMemVaultOverwriteAndDelete(t *testing.T) {
	v := NewMemVault()
	v.Set("k", []byte("v1"))
	v.Set("k", []byte("v2"))
	got, _ := v.Get("k")
	if string(got) != "v2" {
		t.Errorf("overwrite Get = %q", got)
	}
	if err := v.Delete("k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := v.Get("k"); err != ErrNotFound {
		t.Errorf("after delete err = %v", err)
	}
	// 幂等：再删不报错。
	if err := v.Delete("k"); err != nil {
		t.Errorf("delete idempotent err = %v", err)
	}
}

// 存入/取出都应是副本，调用方改动不互相影响（防密钥在内存里被意外抹改）。
func TestMemVaultCopySemantics(t *testing.T) {
	v := NewMemVault()
	src := []byte("secret")
	v.Set("k", src)
	src[0] = 'X' // 改源
	got, _ := v.Get("k")
	if string(got) != "secret" {
		t.Errorf("Set 未存副本: %q", got)
	}
	got[0] = 'Y' // 改取出
	got2, _ := v.Get("k")
	if string(got2) != "secret" {
		t.Errorf("Get 未返副本: %q", got2)
	}
}
