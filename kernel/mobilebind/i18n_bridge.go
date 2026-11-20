package mobilebind

// 字符串表桥（i18n）：让 UI 直接查内核 go:embed 的 zh.json/en.json（单源真源），
// 取代 Kotlin 侧另存一份 assets。回退链：lang→默认(zh)→key。

import (
	"sync"

	"github.com/cjintao133-cell/star-river/kernel/i18n"
)

var (
	i18nMu  sync.Mutex
	i18nB   *i18n.Bundle
	i18nErr bool
)

func i18nBundle() *i18n.Bundle {
	i18nMu.Lock()
	defer i18nMu.Unlock()
	if i18nB == nil && !i18nErr {
		b, err := i18n.New()
		if err != nil {
			i18nErr = true
			return nil
		}
		i18nB = b
	}
	return i18nB
}

// I18nT 取翻译（lang→默认→key）。失败回退 key。
func I18nT(lang, key string) string {
	b := i18nBundle()
	if b == nil {
		return key
	}
	v := b.T(lang, key)
	if v == "" {
		return key
	}
	return v
}

// I18nHas key 是否存在。
func I18nHas(lang, key string) bool {
	b := i18nBundle()
	return b != nil && b.Has(lang, key)
}

// I18nLanguages 支持的语言列表（逗号分隔）。
func I18nLanguages() string {
	b := i18nBundle()
	if b == nil {
		return "zh"
	}
	ls := b.Languages()
	out := ""
	for i, l := range ls {
		if i > 0 {
			out += ","
		}
		out += l
	}
	return out
}
