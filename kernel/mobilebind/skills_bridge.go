package mobilebind

// P1 技能包桥：加载/列出/检索/取用 SKILL.md 技能（渐进披露）。

import (
	"encoding/json"
	"sync"

	"github.com/cjintao133-cell/star-river/kernel/skills"
)

var (
	skillsMu  sync.Mutex
	skillsReg *skills.Registry
)

func skillsRegistry() *skills.Registry {
	skillsMu.Lock()
	defer skillsMu.Unlock()
	if skillsReg == nil {
		skillsReg = skills.NewRegistry()
	}
	return skillsReg
}

// resetSkills 重开库/重置时清空注册表。
func resetSkills() {
	skillsMu.Lock()
	skillsReg = nil
	skillsMu.Unlock()
}

// SkillsLoadDir 从目录扫描加载技能，返回加载数量字符串。
func SkillsLoadDir(dir string) string {
	if dir == "" {
		return "ERR:empty dir"
	}
	n, err := skillsRegistry().LoadDir(dir)
	if err != nil {
		return "ERR:" + err.Error()
	}
	b, _ := json.Marshal(map[string]int{"loaded": n})
	return string(b)
}

// SkillsAdd 直接登记一个技能（name/description/body）。
func SkillsAdd(name, description, body string) string {
	if name == "" {
		return "ERR:empty name"
	}
	if err := skillsRegistry().Add(&skills.Skill{Name: name, Description: description, Body: body}); err != nil {
		return "ERR:" + err.Error()
	}
	return "OK"
}

// SkillsList 列出全部技能元数据 JSON 数组。
func SkillsList() string {
	b, _ := json.Marshal(skillsRegistry().List())
	return string(b)
}

// SkillsMatch 按查询检索技能元数据（前 k 条）。
func SkillsMatch(query string, k int) string {
	b, _ := json.Marshal(skillsRegistry().Match(query, k))
	return string(b)
}

// SkillsGet 取技能全文（含 body）。不存在返回 "ERR:..."。
func SkillsGet(name string) string {
	s, err := skillsRegistry().Get(name)
	if err != nil {
		return "ERR:" + err.Error()
	}
	b, _ := json.Marshal(s)
	return string(b)
}

// SkillsGetFile 读取技能附加文件（第三级，按需）。rel 为相对技能目录的路径。
func SkillsGetFile(name, rel string) string {
	content, err := skillsRegistry().GetFile(name, rel)
	if err != nil {
		return "ERR:" + err.Error()
	}
	return content
}
