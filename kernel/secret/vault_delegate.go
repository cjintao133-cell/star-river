package secret

// Delegate 由平台层实现具体加解密与安全持久化。
// Android Keystore 场景：Kotlin 侧用 android.security.keystore 完成加解密，
// 经 gomobile 门面（kernel/mobilebind）把 Set/Get/Delete 注入到 Go 侧实现本接口。
// 这样 Go 内核不直接碰 JNI/Keystore，平台代码留在各平台层，接口统一。
type Delegate interface {
	Set(id string, secret []byte) error
	Get(id string) ([]byte, error)
	Delete(id string) error
}

// DelegateVault 把平台 Delegate 适配为 Vault，并统一做入参校验与错误归一。
type DelegateVault struct {
	d Delegate
}

// NewDelegateVault 包装平台 Delegate；d 为 nil 时返回的 Vault 所有方法返回错误。
func NewDelegateVault(d Delegate) *DelegateVault { return &DelegateVault{d: d} }

// Set 校验后委托平台保存。
func (v *DelegateVault) Set(id string, secret []byte) error {
	if v.d == nil {
		return errNoDelegate
	}
	if id == "" {
		return ErrEmptyID
	}
	if len(secret) == 0 {
		return ErrEmptySecret
	}
	return v.d.Set(id, secret)
}

// Get 校验后委托平台取回。
func (v *DelegateVault) Get(id string) ([]byte, error) {
	if v.d == nil {
		return nil, errNoDelegate
	}
	if id == "" {
		return nil, ErrEmptyID
	}
	return v.d.Get(id)
}

// Delete 校验后委托平台删除。
func (v *DelegateVault) Delete(id string) error {
	if v.d == nil {
		return errNoDelegate
	}
	if id == "" {
		return ErrEmptyID
	}
	return v.d.Delete(id)
}
