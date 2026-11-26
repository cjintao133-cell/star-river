package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSKILLFrontmatter(t *testing.T) {
	content := "---\nname: translate\ndescription: 中英互译\nauthor: yao\n---\n# 用法\n把文本翻译成目标语言。"
	s, err := ParseSKILL(content, "dirfallback")
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "translate" || s.Description != "中英互译" {
		t.Fatalf("frontmatter 解析不对: %+v", s)
	}
	if s.Meta["author"] != "yao" {
		t.Errorf("自定义元数据丢失: %+v", s.Meta)
	}
	if !containsStr(s.Body, "把文本翻译") {
		t.Errorf("正文不对: %q", s.Body)
	}
}

func TestParseSKILLNoFrontmatter(t *testing.T) {
	s, err := ParseSKILL("# 摘要技能\n这是摘要正文内容。", "summarize")
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "summarize" {
		t.Errorf("无 name 应用目录名: %q", s.Name)
	}
	if s.Description == "" {
		t.Errorf("description 应回退首行")
	}
}

func TestParseSKILLEmptyName(t *testing.T) {
	if _, err := ParseSKILL("   ", ""); err == nil {
		t.Fatal("空名应报错")
	}
}

func TestRegistryLoadDirAndList(t *testing.T) {
	dir := t.TempDir()
	mk := func(sub, content string) {
		d := filepath.Join(dir, sub)
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "SKILL.md"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	mk("translate", "---\nname: translate\ndescription: 翻译文本\n---\n正文A")
	mk("summarize", "---\nname: summarize\ndescription: 总结长文\n---\n正文B")
	mk("empty", "no front")                          // 仍应加载（name 用目录名）
	os.MkdirAll(filepath.Join(dir, "nofront"), 0755) // 无 SKILL.md 应跳过

	r := NewRegistry()
	n, err := r.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("应加载 3 个技能, got %d", n)
	}
	list := r.List()
	if len(list) != 3 {
		t.Fatalf("List 应 3 条: %d", len(list))
	}
	// 元数据不应包含正文
	for _, m := range list {
		if containsStr(m.Description, "正文A") {
			t.Errorf("元数据不应含正文")
		}
	}
}

func TestRegistryMatchAndGet(t *testing.T) {
	r := NewRegistry()
	_ = r.Add(&Skill{Name: "translate", Description: "中英互译", Body: "把文本翻译成英文"})
	_ = r.Add(&Skill{Name: "summarize", Description: "总结长文", Body: "提炼要点"})
	_ = r.Add(&Skill{Name: "recipe", Description: "做菜步骤", Body: "番茄炒蛋"})

	hits := r.Match("帮我翻译这段英文", 2)
	if len(hits) == 0 || hits[0].Name != "translate" {
		t.Fatalf("应命中 translate: %+v", hits)
	}
	if len(hits) > 2 {
		t.Fatalf("k 限制失效: %d", len(hits))
	}
	if _, err := r.Get("translate"); err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if _, err := r.Get("nope"); err == nil {
		t.Fatal("不存在应报错")
	}
}

func TestRegistryOverwriteKeepsOrder(t *testing.T) {
	r := NewRegistry()
	_ = r.Add(&Skill{Name: "a", Description: "1"})
	_ = r.Add(&Skill{Name: "b", Description: "2"})
	_ = r.Add(&Skill{Name: "a", Description: "更新"}) // 覆盖
	list := r.List()
	if len(list) != 2 || list[0].Name != "a" || list[0].Description != "更新" {
		t.Fatalf("覆盖且保留顺序不对: %+v", list)
	}
}

func containsStr(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
