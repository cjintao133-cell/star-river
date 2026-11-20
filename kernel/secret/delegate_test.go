package secret

import (
	"errors"
	"testing"
)

// fakeDelegate 记录委托调用，模拟平台层（如 Kotlin Keystore）。
type fakeDelegate struct {
	m       map[string][]byte
	setCall int
	delCall int
}

func newFakeDelegate() *fakeDelegate { return &fakeDelegate{m: map[string][]byte{}} }

func (f *fakeDelegate) Set(id string, s []byte) error {
	f.setCall++
	f.m[id] = append([]byte(nil), s...)
	return nil
}
func (f *fakeDelegate) Get(id string) ([]byte, error) {
	s, ok := f.m[id]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), s...), nil
}
func (f *fakeDelegate) Delete(id string) error {
	f.delCall++
	delete(f.m, id)
	return nil
}

func TestDelegateVaultDelegates(t *testing.T) {
	fd := newFakeDelegate()
	v := NewDelegateVault(fd)
	if err := v.Set("k", []byte("secret")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if fd.setCall != 1 {
		t.Errorf("delegate Set not called")
	}
	got, err := v.Get("k")
	if err != nil || string(got) != "secret" {
		t.Errorf("Get = %q, %v", got, err)
	}
	if err := v.Delete("k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if fd.delCall != 1 {
		t.Errorf("delegate Delete not called")
	}
}

// 校验在委托前拦截，不把非法参数透传给平台层。
func TestDelegateVaultValidation(t *testing.T) {
	fd := newFakeDelegate()
	v := NewDelegateVault(fd)
	if err := v.Set("", []byte("x")); err != ErrEmptyID {
		t.Errorf("empty id err = %v", err)
	}
	if err := v.Set("k", nil); err != ErrEmptySecret {
		t.Errorf("empty secret err = %v", err)
	}
	if fd.setCall != 0 {
		t.Errorf("非法参数不应透传 delegate, setCall=%d", fd.setCall)
	}
}

func TestDelegateVaultNil(t *testing.T) {
	v := NewDelegateVault(nil)
	if err := v.Set("k", []byte("x")); !errors.Is(err, errNoDelegate) {
		t.Errorf("nil delegate Set err = %v", err)
	}
	if _, err := v.Get("k"); !errors.Is(err, errNoDelegate) {
		t.Errorf("nil delegate Get err = %v", err)
	}
	if err := v.Delete("k"); !errors.Is(err, errNoDelegate) {
		t.Errorf("nil delegate Delete err = %v", err)
	}
}

func TestRedactScrub(t *testing.T) {
	secret := "sk-SECRET-XYZ"
	in := "url?key=" + secret + "&other=1"
	out := ScrubString(in, secret)
	if contains(out, secret) {
		t.Errorf("ScrubString 未抹除: %q", out)
	}
	if !contains(out, "[REDACTED]") {
		t.Errorf("ScrubString 未替换: %q", out)
	}
	if string(Scrub([]byte(in), secret)) == in {
		t.Errorf("Scrub 未抹除")
	}
	// 空 secret 不动。
	if ScrubString("abc", "") != "abc" {
		t.Error("空 secret 不应改动")
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestDelegateVaultEmptyIDGetDelete(t *testing.T) {
	fd := newFakeDelegate()
	v := NewDelegateVault(fd)
	if _, err := v.Get(""); err != ErrEmptyID {
		t.Errorf("Get empty id err = %v", err)
	}
	if err := v.Delete(""); err != ErrEmptyID {
		t.Errorf("Delete empty id err = %v", err)
	}
}

func TestScrubEmptyBytes(t *testing.T) {
	if got := Scrub(nil, "secret"); len(got) != 0 {
		t.Errorf("Scrub(nil) = %v, want empty", got)
	}
	if got := ScrubString("", "secret"); got != "" {
		t.Errorf("ScrubString empty = %q", got)
	}
}
