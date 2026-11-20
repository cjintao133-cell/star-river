package i18n

import (
	"strings"
	"testing"
)

func TestNewLoadsDefault(t *testing.T) {
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	langs := b.Languages()
	if len(langs) != 2 || langs[0] != "zh" {
		t.Fatalf("应 2 语言且 zh 在前, got %v", langs)
	}
	if b.DefaultLangOf() != "zh" {
		t.Errorf("默认应 zh, got %s", b.DefaultLangOf())
	}
	// 中文默认文案
	if got := b.T("zh", "common.ok"); got != "确定" {
		t.Errorf("zh common.ok = %q", got)
	}
	if got := b.T("en", "common.ok"); got != "OK" {
		t.Errorf("en common.ok = %q", got)
	}
}

func TestKeysParity(t *testing.T) {
	// 英文表必须齐全（与中文 key 集合一致），这是 PRD 硬要求。
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if miss := b.Missing("en", "zh"); len(miss) != 0 {
		t.Errorf("en 缺 key: %v", miss)
	}
	if miss := b.Missing("zh", "en"); len(miss) != 0 {
		t.Errorf("zh 缺 key: %v", miss)
	}
	// 每个 zh key 在 en 都有非空文案
	for _, k := range b.Keys("zh") {
		if !b.Has("en", k) {
			t.Errorf("en 缺 key %q", k)
			continue
		}
		if strings.TrimSpace(b.T("en", k)) == "" {
			t.Errorf("en[%q] 文案为空", k)
		}
	}
}

func TestFallback(t *testing.T) {
	b, _ := New()
	// 未知语言 → 回退默认 zh
	if got := b.T("fr", "common.ok"); got != "确定" {
		t.Errorf("未知语言应回退 zh, got %q", got)
	}
	// 未知 key → 返回 key 本身（可见缺失）
	if got := b.T("zh", "no.such.key"); got != "no.such.key" {
		t.Errorf("未知 key 应返回 key 本身, got %q", got)
	}
	// 某语言缺 key → 回退 zh 的该 key
	b2, err := NewFrom(map[string][]byte{
		"zh": []byte(`{"a":"甲","b":"乙"}`),
		"en": []byte(`{"a":"A"}`),
	}, "zh")
	if err != nil {
		t.Fatal(err)
	}
	if got := b2.T("en", "b"); got != "乙" {
		t.Errorf("en 缺 b 应回退 zh 乙, got %q", got)
	}
}

func TestInterpolation(t *testing.T) {
	b, _ := NewFrom(map[string][]byte{
		"zh": []byte(`{"greet":"你好，{name}！"}`),
	}, "zh")
	if got := b.Tf("zh", "greet", map[string]string{"name": "用户"}); got != "你好，用户！" {
		t.Errorf("插值不对: %q", got)
	}
	// 缺失占位保留原样
	if got := b.Tf("zh", "greet", nil); got != "你好，{name}！" {
		t.Errorf("缺占位应保留: %q", got)
	}
}

func TestHasAndKeys(t *testing.T) {
	b, _ := New()
	if !b.Has("zh", "settings.title") || b.Has("zh", "nope") {
		t.Error("Has 判断不对")
	}
	keys := b.Keys("zh")
	if len(keys) == 0 {
		t.Fatal("keys 不应为空")
	}
	// 排序
	for i := 1; i < len(keys); i++ {
		if keys[i-1] > keys[i] {
			t.Errorf("keys 未排序: %v", keys)
			break
		}
	}
}

func TestNewFromErrors(t *testing.T) {
	// 默认语言缺失
	if _, err := NewFrom(map[string][]byte{"en": []byte(`{}`)}, "zh"); err == nil {
		t.Error("默认语言缺失应报错")
	}
	// 坏 JSON
	if _, err := NewFrom(map[string][]byte{"zh": []byte(`{bad`)}, "zh"); err == nil {
		t.Error("坏 JSON 应报错")
	}
}
