// Package i18n 是界面文案字符串表（T-M2-07）：界面文案与模型语种解耦，
// 按钮/菜单/提示一律走独立字符串表，禁止在 UI 代码里硬编码（约定见 docs/architecture.md「工程纪律」）。
//
// 中文为默认，英文表齐全（key 集合一致，有 check-i18n-keys.sh 守卫与单测断言）。
// 资源以 go:embed 的 JSON 形式入仓（zh.json / en.json），设置页切换语言后全界面按 key 取值即可生效
// （切换 UI 与真机走查属界面层，押后；内核提供取值与回退能力）。
//
// 依赖方向：纯标准库 + embed，无插件依赖。
package i18n

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// DefaultLang 默认语言（中文）。
const DefaultLang = "zh"

//go:embed zh.json
var zhData []byte

//go:embed en.json
var enData []byte

// Bundle 多语言字符串表。并发安全。
type Bundle struct {
	mu    sync.RWMutex
	langs map[string]map[string]string
	def   string
}

// New 用内嵌资源构建默认 Bundle（zh 默认 + en）。
func New() (*Bundle, error) {
	return NewFrom(map[string][]byte{"zh": zhData, "en": enData}, DefaultLang)
}

// NewFrom 用给定语言数据构建（lang -> JSON 字节）。def 必须在数据中存在。
func NewFrom(data map[string][]byte, def string) (*Bundle, error) {
	langs := make(map[string]map[string]string, len(data))
	for lang, raw := range data {
		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("i18n: parse %s: %w", lang, err)
		}
		langs[lang] = m
	}
	if _, ok := langs[def]; !ok {
		return nil, fmt.Errorf("i18n: default language %q missing", def)
	}
	return &Bundle{langs: langs, def: def}, nil
}

// T 取 lang 下 key 的文案。回退链：lang → 默认语言 → 返回 key 本身（可见缺失，不静默空白）。
func (b *Bundle) T(lang, key string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if m, ok := b.langs[lang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	if m, ok := b.langs[b.def]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return key
}

// Tf 取文案并做 {name} 占位替换（缺失占位保留原样）。
func (b *Bundle) Tf(lang, key string, args map[string]string) string {
	s := b.T(lang, key)
	for k, v := range args {
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}

// Has 报告 lang 下是否有 key（不触发回退）。
func (b *Bundle) Has(lang, key string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	m, ok := b.langs[lang]
	if !ok {
		return false
	}
	_, ok = m[key]
	return ok
}

// Languages 已加载语言（排序，默认语言排前）。
func (b *Bundle) Languages() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]string, 0, len(b.langs))
	for l := range b.langs {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i] == b.def {
			return true
		}
		if out[j] == b.def {
			return false
		}
		return out[i] < out[j]
	})
	return out
}

// Keys 返回 lang 的全部 key（排序）。
func (b *Bundle) Keys(lang string) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	m := b.langs[lang]
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// DefaultLangOf 返回默认语言。
func (b *Bundle) DefaultLangOf() string { return b.def }

// Missing 返回 reference 语言有、而 lang 缺失的 key（用于完整性校验）。
func (b *Bundle) Missing(lang, reference string) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	ref := b.langs[reference]
	tgt := b.langs[lang]
	var miss []string
	for k := range ref {
		if _, ok := tgt[k]; !ok {
			miss = append(miss, k)
		}
	}
	sort.Strings(miss)
	return miss
}
