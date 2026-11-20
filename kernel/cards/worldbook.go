package cards

import "strings"

// DefaultMaxActivationDepth selective 触发词递归激活的默认深度上限（A2：防无限循环）。
const DefaultMaxActivationDepth = 3

// ActivatedEntry 一次激活结果（selective 触发）。
type ActivatedEntry struct {
	Index int
	Entry BookEntry
	Depth int // 激活深度（0=直接被初始文本触发）
}

// ConstantEntries 返回 enabled 且 constant 的常驻条目（高优先级常驻，受预算分配器管）。
func (c *Card) ConstantEntries() []BookEntry {
	var out []BookEntry
	for _, e := range c.Book {
		if e.Enabled && e.Constant {
			out = append(out, e)
		}
	}
	return out
}

// Activate 依据初始文本激活 selective 世界书条目：
//   - 关键词匹配（大小写不敏感的子串）；
//   - 被激活条目的内容追加进匹配文本，可级联触发更多条目（递归），深度上限 maxDepth；
//   - 每个条目至多激活一次（循环检测，防 A→B→A 循环）；
//   - constant 条目不计入激活（常驻由 ConstantEntries 处理），enabled=false 跳过。
//
// maxDepth<=0 用 DefaultMaxActivationDepth。
func (c *Card) Activate(text string, maxDepth int) []ActivatedEntry {
	if maxDepth <= 0 {
		maxDepth = DefaultMaxActivationDepth
	}
	activated := make(map[int]bool, len(c.Book))
	var out []ActivatedEntry
	acc := strings.ToLower(text)

	// 逐层 BFS：每一轮在当前累积文本上找新命中，命中后把内容并入累积文本进入下一层。
	depth := 0
	for depth <= maxDepth {
		var newly []int
		for i, e := range c.Book {
			if !e.Enabled || e.Constant || activated[i] {
				continue
			}
			if entryMatches(e, acc) {
				newly = append(newly, i)
			}
		}
		if len(newly) == 0 {
			break
		}
		for _, i := range newly {
			activated[i] = true
			out = append(out, ActivatedEntry{Index: i, Entry: c.Book[i], Depth: depth})
			acc += " " + strings.ToLower(c.Book[i].Content)
		}
		depth++
	}
	return out
}

// entryMatches 任一词命中即算匹配。
func entryMatches(e BookEntry, lowerAcc string) bool {
	for _, k := range e.Keys {
		if k == "" {
			continue
		}
		if strings.Contains(lowerAcc, strings.ToLower(k)) {
			return true
		}
	}
	return false
}
