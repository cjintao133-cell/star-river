package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFrontmatterExtended(t *testing.T) {
	content := `---
name: pdf-tools
description: 处理 PDF 文件
license: MIT
compatibility: 需要 python3
author: yao
allowed-tools: [read_file, write_file]
---
正文：抽文本、填表、合并。`
	s, err := ParseSKILL(content, "pdf")
	if err != nil {
		t.Fatal(err)
	}
	if s.Meta["license"] != "MIT" || s.Meta["compatibility"] != "需要 python3" || s.Meta["author"] != "yao" {
		t.Fatalf("可选字段解析不对: %+v", s.Meta)
	}
	if len(s.AllowedTools) != 2 || s.AllowedTools[0] != "read_file" || s.AllowedTools[1] != "write_file" {
		t.Fatalf("allowed-tools 数组解析不对: %+v", s.AllowedTools)
	}
}

func TestParseAllowedToolsCommaForm(t *testing.T) {
	s, err := ParseSKILL("---\nname: x\ndescription: d\nallowed-tools: a, b, c\n---\nbody", "x")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.AllowedTools) != 3 || s.AllowedTools[2] != "c" {
		t.Fatalf("逗号形式 allowed-tools 解析不对: %+v", s.AllowedTools)
	}
}

func makeSkillDir(t *testing.T, root, name, skillMD string, extra map[string]string) {
	t.Helper()
	d := filepath.Join(root, name)
	if err := os.MkdirAll(d, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "SKILL.md"), []byte(skillMD), 0644); err != nil {
		t.Fatal(err)
	}
	for rel, content := range extra {
		full := filepath.Join(d, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadDirCollectsFilesAndGetFile(t *testing.T) {
	root := t.TempDir()
	makeSkillDir(t, root, "translate",
		"---\nname: translate\ndescription: 翻译\nallowed-tools: [read_file]\n---\n翻译正文",
		map[string]string{
			"reference.md":   "# 参考\n术语表...",
			"scripts/run.py": "print('hi')",
		})
	r := NewRegistry()
	n, err := r.LoadDir(root)
	if err != nil || n != 1 {
		t.Fatalf("加载失败: n=%d err=%v", n, err)
	}
	// 第一级：List 带 files 数量但不含内容
	list := r.List()
	if list[0].Files != 2 {
		t.Fatalf("应发现 2 个附加文件: %+v", list[0])
	}
	if len(list[0].AllowedTools) != 1 || list[0].AllowedTools[0] != "read_file" {
		t.Fatalf("元数据应带 allowed-tools: %+v", list[0])
	}
	// 第二级：Get 带 files 清单
	sk, _ := r.Get("translate")
	found := map[string]bool{}
	for _, f := range sk.Files {
		found[f] = true
	}
	if !found["reference.md"] || !found["scripts/run.py"] {
		t.Fatalf("附加文件清单不对: %+v", sk.Files)
	}
	// 第三级：GetFile 读内容
	if c, err := r.GetFile("translate", "reference.md"); err != nil || !strings.Contains(c, "术语表") {
		t.Fatalf("读参考文件失败: %q %v", c, err)
	}
	if c, err := r.GetFile("translate", "scripts/run.py"); err != nil || !strings.Contains(c, "print") {
		t.Fatalf("读脚本失败: %q %v", c, err)
	}
}

func TestGetFilePathSafety(t *testing.T) {
	root := t.TempDir()
	makeSkillDir(t, root, "s", "---\nname: s\ndescription: d\n---\nbody", map[string]string{"ok.txt": "hello"})
	r := NewRegistry()
	_, _ = r.LoadDir(root)
	// 目录穿越拒绝
	for _, bad := range []string{"../secret", "../../etc/passwd", "/etc/passwd", "..", "a/../../x"} {
		if _, err := r.GetFile("s", bad); err == nil {
			t.Errorf("穿越路径 %q 应被拒", bad)
		}
	}
	// 不存在文件 -> 报错
	if _, err := r.GetFile("s", "nope.txt"); err == nil {
		t.Errorf("不存在文件应报错")
	}
	// 不存在技能
	if _, err := r.GetFile("nope", "ok.txt"); err == nil {
		t.Errorf("不存在技能应报错")
	}
}

func TestGetFileNoDir(t *testing.T) {
	r := NewRegistry()
	_ = r.Add(&Skill{Name: "mem", Description: "内存技能", Body: "x"})
	if _, err := r.GetFile("mem", "a.txt"); err == nil {
		t.Fatalf("无目录技能取文件应报错")
	}
}
