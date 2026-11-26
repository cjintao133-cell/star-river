// Package skills 实现 Agent Skills（SKILL.md）开放标准的本地技能包：解析、注册、三级渐进披露。
//
// 一个技能是一个目录，含 SKILL.md：可选 YAML 前置元数据（--- ... ---）+ Markdown 正文，
// 目录下还可带附加文件/脚本。渐进披露三级：
//  1. 元数据（List/Match 只给 name/description/allowed-tools，≈100 词，常驻 context）
//  2. SKILL.md 正文（Get 才下发）
//  3. 附加文件/脚本（GetFile 才按需读取）
//
// 零内部依赖（仅标准库）。
package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Skill 一个技能。
type Skill struct {
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Body         string            `json:"body"` // Markdown 正文（第二级，渐进披露）
	Path         string            `json:"path"`
	Files        []string          `json:"files,omitempty"`         // 附加文件相对路径（第三级，不含 SKILL.md）
	AllowedTools []string          `json:"allowed_tools,omitempty"` // allowed-tools 声明
	Meta         map[string]string `json:"meta,omitempty"`          // license/compatibility/author 等
}

// Meta 仅元数据（渐进披露第一级）。
type Meta struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Path         string   `json:"path"`
	AllowedTools []string `json:"allowed_tools,omitempty"`
	Files        int      `json:"files,omitempty"` // 附加文件数量（不列内容，提示可下钻）
}

var (
	ErrNotFound  = errors.New("skills: not found")
	ErrEmptyName = errors.New("skills: empty skill name")
	ErrNoDir     = errors.New("skills: skill has no directory")
	ErrBadPath   = errors.New("skills: illegal file path")
)

// Registry 技能注册表。
type Registry struct {
	byName map[string]*Skill
	order  []string
}

// NewRegistry 空注册表。
func NewRegistry() *Registry { return &Registry{byName: map[string]*Skill{}} }

// ParseSKILL 解析 SKILL.md 内容（frontmatter + body）。dirName 作为无 name 时的回退名。
func ParseSKILL(content, dirName string) (*Skill, error) {
	s := &Skill{Meta: map[string]string{}}
	body := content
	text := strings.ReplaceAll(content, "\r\n", "\n")
	if strings.HasPrefix(strings.TrimLeft(text, "\n"), "---") {
		trimmed := strings.TrimLeft(text, "\n")
		rest := trimmed[3:]
		if end := strings.Index(rest, "\n---"); end >= 0 {
			fm := rest[:end]
			body = rest[end+4:]
			body = strings.TrimPrefix(body, "\n")
			for _, line := range strings.Split(fm, "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
					continue
				}
				k, v, ok := splitKV(line)
				if !ok {
					continue
				}
				switch k {
				case "name":
					s.Name = v
				case "description":
					s.Description = v
				case "allowed-tools", "allowed_tools", "allowedtools":
					s.AllowedTools = parseList(v)
				default:
					s.Meta[k] = v
				}
			}
		}
	}
	s.Body = strings.TrimSpace(body)
	if s.Name == "" {
		s.Name = dirName
	}
	if strings.TrimSpace(s.Name) == "" {
		return nil, ErrEmptyName
	}
	if s.Description == "" {
		s.Description = firstLine(s.Body)
	}
	return s, nil
}

func splitKV(line string) (string, string, bool) {
	i := strings.Index(line, ":")
	if i <= 0 {
		return "", "", false
	}
	k := strings.TrimSpace(line[:i])
	v := strings.TrimSpace(line[i+1:])
	v = strings.Trim(v, `"'`)
	return strings.ToLower(k), v, true
}

// parseList 解析 allowed-tools 值：支持 "[a, b]" 与 "a, b" 两种写法。
func parseList(v string) []string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "[")
	v = strings.TrimSuffix(v, "]")
	var out []string
	for _, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(strings.Trim(p, `"'`))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "#"))
		if line != "" {
			if len([]rune(line)) > 120 {
				return string([]rune(line)[:120]) + "…"
			}
			return line
		}
	}
	return ""
}

// Add 登记一个技能（重名覆盖，保留首次顺序）。
func (r *Registry) Add(s *Skill) error {
	if s == nil || strings.TrimSpace(s.Name) == "" {
		return ErrEmptyName
	}
	if _, ok := r.byName[s.Name]; !ok {
		r.order = append(r.order, s.Name)
	}
	r.byName[s.Name] = s
	return nil
}

// LoadDir 扫描 dir 下每个子目录的 SKILL.md 并登记，同时收集附加文件（第三级）。返回加载数量。
func (r *Registry) LoadDir(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(dir, e.Name())
		p := filepath.Join(sub, "SKILL.md")
		b, err := os.ReadFile(p)
		if err != nil {
			continue // 无 SKILL.md 的子目录跳过
		}
		s, err := ParseSKILL(string(b), e.Name())
		if err != nil {
			continue
		}
		s.Path = p
		s.Files = collectFiles(sub)
		if err := r.Add(s); err != nil {
			continue
		}
		n++
	}
	return n, nil
}

// collectFiles 收集技能目录下（除 SKILL.md）的全部文件相对路径，排序稳定。
func collectFiles(sub string) []string {
	var files []string
	_ = filepath.WalkDir(sub, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(sub, p)
		if e != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "SKILL.md" {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	sort.Strings(files)
	return files
}

// List 返回全部技能元数据（第一级，按登记顺序）。
func (r *Registry) List() []Meta {
	out := make([]Meta, 0, len(r.order))
	for _, name := range r.order {
		if s, ok := r.byName[name]; ok {
			out = append(out, Meta{Name: s.Name, Description: s.Description, Path: s.Path, AllowedTools: s.AllowedTools, Files: len(s.Files)})
		}
	}
	return out
}

// Get 取技能（含正文与附加文件清单，第二级）。
func (r *Registry) Get(name string) (*Skill, error) {
	s, ok := r.byName[name]
	if !ok {
		return nil, ErrNotFound
	}
	return s, nil
}

// GetFile 读取技能附加文件（第三级，按需）。rel 为相对技能目录的路径，禁止穿越。
func (r *Registry) GetFile(name, rel string) (string, error) {
	s, ok := r.byName[name]
	if !ok {
		return "", ErrNotFound
	}
	if s.Path == "" {
		return "", ErrNoDir
	}
	dir := filepath.Dir(s.Path)
	clean := filepath.Clean(strings.TrimSpace(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
		return "", ErrBadPath
	}
	full := filepath.Join(dir, clean)
	relBack, err := filepath.Rel(dir, full)
	if err != nil || relBack == ".." || strings.HasPrefix(relBack, ".."+string(filepath.Separator)) {
		return "", ErrBadPath
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Match 按查询词对 name/description/body 打分，返回前 k 个（渐进披露：先给元数据）。
func (r *Registry) Match(query string, k int) []Meta {
	terms := tokenize(query)
	type scored struct {
		m Meta
		s int
	}
	var list []scored
	for _, name := range r.order {
		s := r.byName[name]
		hay := strings.ToLower(s.Name + " " + s.Description + " " + s.Body)
		sc := 0
		for _, t := range terms {
			if t == "" {
				continue
			}
			if strings.Contains(strings.ToLower(s.Name), t) {
				sc += 3
			}
			if strings.Contains(strings.ToLower(s.Description), t) {
				sc += 2
			}
			if strings.Contains(hay, t) {
				sc++
			}
		}
		if sc > 0 {
			list = append(list, scored{m: Meta{Name: s.Name, Description: s.Description, Path: s.Path, AllowedTools: s.AllowedTools, Files: len(s.Files)}, s: sc})
		}
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].s > list[j].s })
	if k <= 0 {
		k = 5
	}
	out := make([]Meta, 0, k)
	for i := 0; i < len(list) && i < k; i++ {
		out = append(out, list[i].m)
	}
	return out
}

func tokenize(q string) []string {
	q = strings.ToLower(q)
	f := strings.FieldsFunc(q, func(r rune) bool {
		return r == ' ' || r == ',' || r == '，' || r == '。' || r == '\t' || r == '\n' || r == '/' || r == '、'
	})
	var out []string
	seen := map[string]bool{}
	add := func(t string) {
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	for _, tok := range f {
		add(tok)
		if hasCJK(tok) {
			r := []rune(tok)
			for i := 0; i+1 < len(r); i++ {
				add(string(r[i : i+2])) // CJK 二元组
			}
			if len(r) == 1 {
				add(string(r))
			}
		}
	}
	return out
}

func hasCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

// String 便于日志。
func (s *Skill) String() string { return fmt.Sprintf("%s: %s", s.Name, s.Description) }
