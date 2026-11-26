// Package hygiene 是"记忆卫生"作业：定期对被检索到的记忆做修剪(prune)、去重合并(merge)、
// 摘要(summarize)决策。设计为纯函数（不改动入参、不做 IO），便于离线确定性测试；
// 真正的落地（删/改/写记忆）由入口层按本包给出的建议执行。
//
// 动机：对齐 always-on agent 的"记忆卫生在独立定时作业里做，而非任务中途写长期记忆"。
package hygiene

import (
	"sort"
	"strings"

	"github.com/cjintao133-cell/star-river/kernel/hippocampus"
)

// Config 卫生策略（零值走默认）。
type Config struct {
	MinRetention   float64 // 留存度低于此值才可能被修剪（默认 0.15）
	MinImportance  float64 // 重要度低于此值才可能被修剪（默认 0.30）
	MaxIdleDays    float64 // 距上次复习超过此天数才可能被修剪（默认 60）
	MergeThreshold float64 // 文本相似度（bigram Jaccard）≥ 此值视为重复（默认 0.85）
}

func (c Config) withDefaults() Config {
	if c.MinRetention <= 0 {
		c.MinRetention = 0.15
	}
	if c.MinImportance <= 0 {
		c.MinImportance = 0.30
	}
	if c.MaxIdleDays <= 0 {
		c.MaxIdleDays = 60
	}
	if c.MergeThreshold <= 0 || c.MergeThreshold > 1 {
		c.MergeThreshold = 0.85
	}
	return c
}

// PruneReport 修剪报告。
type PruneReport struct {
	DropIdx []int `json:"drop_idx"`
	Kept    int   `json:"kept"`
	Dropped int   `json:"dropped"`
}

// Prune 返回建议丢弃的索引：留存低 且 重要度低 且 久未复习。三者同时满足才丢（保守）。
func Prune(mems []hippocampus.Memory, now int64, cfg Config) PruneReport {
	cfg = cfg.withDefaults()
	rep := PruneReport{}
	const dayMillis = int64(24 * 60 * 60 * 1000)
	for i, m := range mems {
		r := hippocampus.Retention(m, now)
		idleDays := float64(now-m.LastReviewAt) / float64(dayMillis)
		if r < cfg.MinRetention && m.Importance < cfg.MinImportance && idleDays > cfg.MaxIdleDays {
			rep.DropIdx = append(rep.DropIdx, i)
		}
	}
	rep.Dropped = len(rep.DropIdx)
	rep.Kept = len(mems) - rep.Dropped
	return rep
}

// MergeGroup 一组重复记忆（第一个为保留主条，其余为可合并项）。
type MergeGroup struct {
	KeepIdx  int   `json:"keep_idx"`
	DropIdxs []int `json:"drop_idxs"`
}

// Duplicates 找出近重复记忆分组（按相同文本或 bigram Jaccard ≥ 阈值）。
// 返回的组内 KeepIdx 为组内首条（重要度更高者优先保留）。
func Duplicates(mems []hippocampus.Memory, cfg Config) []MergeGroup {
	cfg = cfg.withDefaults()
	n := len(mems)
	used := make([]bool, n)
	// 按重要度高到低排序索引，优先保留重要的。
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return mems[order[a]].Importance > mems[order[b]].Importance })

	var groups []MergeGroup
	for _, i := range order {
		if used[i] {
			continue
		}
		g := MergeGroup{KeepIdx: i}
		for _, j := range order {
			if j == i || used[j] {
				continue
			}
			if similar(mems[i].Text, mems[j].Text, cfg.MergeThreshold) {
				g.DropIdxs = append(g.DropIdxs, j)
				used[j] = true
			}
		}
		if len(g.DropIdxs) > 0 {
			used[i] = true
			groups = append(groups, g)
		}
	}
	return groups
}

// MergedText 把一组记忆文本合并为去重后的句子串（供写回一条合并记忆）。
func MergedText(texts []string) string {
	seen := map[string]bool{}
	var parts []string
	for _, t := range texts {
		for _, s := range splitSentences(t) {
			k := normalize(s)
			if k == "" || seen[k] {
				continue
			}
			seen[k] = true
			parts = append(parts, strings.TrimSpace(s))
		}
	}
	return strings.Join(parts, "；")
}

func splitSentences(s string) []string {
	f := strings.FieldsFunc(s, func(r rune) bool {
		return r == '。' || r == '！' || r == '？' || r == '.' || r == '!' || r == '?' || r == '\n'
	})
	return f
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\u3000' || r == '，' || r == ',' || r == '。' || r == '.' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// similar 文本相似：归一化相等，或字符 bigram Jaccard ≥ threshold。
func similar(a, b string, threshold float64) bool {
	na, nb := normalize(a), normalize(b)
	if na == "" || nb == "" {
		return false
	}
	if na == nb {
		return true
	}
	return jaccard(bigrams(na), bigrams(nb)) >= threshold
}

func bigrams(s string) map[string]bool {
	r := []rune(s)
	m := map[string]bool{}
	if len(r) < 2 {
		if len(r) == 1 {
			m[string(r)] = true
		}
		return m
	}
	for i := 0; i+1 < len(r); i++ {
		m[string(r[i:i+2])] = true
	}
	return m
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	inter := 0
	for k := range a {
		if b[k] {
			inter++
		}
	}
	uni := len(a) + len(b) - inter
	if uni == 0 {
		return 0
	}
	return float64(inter) / float64(uni)
}
