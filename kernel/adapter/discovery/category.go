// Package discovery 模型发现与三层分类（T-M1-06）。
//
// 职责边界：本包只做与协议无关的纯逻辑——把任意端点 /models 拉到的一批模型 id
// 归类（快速/旗舰/推理/未分类）、编排懒校验、维护能力台账；不碰任何 HTTP/传输细节。
// 各协议适配器（openai/anthropic/gemini）在集成层用 Lister/Prober 接口接入，本包不 import 它们。
//
// 持久化：能力台账经 CapabilityStore 接口落库；M1 提供内存实现供测试，M2 起由
// kernel/store（SQLite）实现同一接口（T-M2-01/T-M3-07）。本包不依赖 store 包。
package discovery

import (
	"sort"
	"strings"
)

// Category 三层分类（快速/旗舰/推理）+ 未分类兜底。
type Category string

const (
	CategoryFast      Category = "fast"      // 快速/廉价档
	CategoryFlagship  Category = "flagship"  // 旗舰/强档
	CategoryReasoning Category = "reasoning" // 推理档
	CategoryUnknown   Category = "unknown"   // 未分类兜底
)

// 判定来源，便于排障与 UI 标注（可追溯哪条规则命中）。
const (
	SourceMetadata  = "metadata"  // 内置元数据精确匹配
	SourceHeuristic = "heuristic" // 名字启发式
	SourceFallback  = "fallback"  // 未分类兜底
)

// Classification 单个模型的分类结果。ModelID 为原始输入（供展示），
// 判定基于归一化后的 id（去空白/小写/去 models/ 前缀）。
type Classification struct {
	ModelID  string
	Category Category
	Source   string
}

// metadataTable 内置元数据精确匹配表（键为归一化小写基名）。
// 只放确定性高的常用模型；未列者走启发式再兜底。新增条目无需改代码逻辑。
// 匹配用"最长基名前缀"：id 等于键，或以 键+"-" 开头（吸收日期/版本后缀）。
var metadataTable = map[string]Category{
	// OpenAI
	"gpt-4o":       CategoryFlagship,
	"gpt-4o-mini":  CategoryFast,
	"gpt-4-turbo":  CategoryFlagship,
	"gpt-4.1":      CategoryFlagship,
	"gpt-4.1-mini": CategoryFast,
	"gpt-4.1-nano": CategoryFast,
	"o1":           CategoryReasoning,
	"o1-mini":      CategoryReasoning,
	"o1-preview":   CategoryReasoning,
	"o3":           CategoryReasoning,
	"o3-mini":      CategoryReasoning,
	"o4-mini":      CategoryReasoning,
	// Anthropic
	"claude-3-opus":     CategoryFlagship,
	"claude-3-sonnet":   CategoryFlagship,
	"claude-3-haiku":    CategoryFast,
	"claude-3-5-sonnet": CategoryFlagship,
	"claude-3-5-haiku":  CategoryFast,
	"claude-3-7-sonnet": CategoryFlagship,
	// Google
	"gemini-1.5-pro":      CategoryFlagship,
	"gemini-1.5-flash":    CategoryFast,
	"gemini-1.5-flash-8b": CategoryFast,
	"gemini-2.0-flash":    CategoryFast,
	// DeepSeek / 其他
	"deepseek-chat":     CategoryFlagship,
	"deepseek-reasoner": CategoryReasoning,
	"deepseek-r1":       CategoryReasoning,
}

// heuristicRules 名字启发式规则，按顺序匹配（推理 > 快速 > 旗舰），命中即返回。
// 仅用于元数据未覆盖的模型，属尽力而为的 UI 提示，不追求零误判。
var heuristicRules = []struct {
	subs []string
	cat  Category
}{
	{[]string{"reason", "thinking", "qwq", "o1", "o3", "o4", "r1"}, CategoryReasoning},
	{[]string{"mini", "flash", "haiku", "nano", "small", "lite", "turbo", "instant", "8b"}, CategoryFast},
	{[]string{"opus", "sonnet", "pro", "max", "ultra", "large", "70b", "405b", "gpt-4", "gpt-5"}, CategoryFlagship},
}

// normalizeID 归一化模型 id：去空白、小写、去 models/ 前缀。
func normalizeID(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimPrefix(s, "models/")
	return s
}

// matchMetadata 最长基名前缀匹配内置元数据表。返回命中分类与是否命中。
func matchMetadata(norm string) (Category, bool) {
	bestLen := -1
	var bestCat Category
	for k, cat := range metadataTable {
		if norm == k || strings.HasPrefix(norm, k+"-") {
			if len(k) > bestLen {
				bestLen = len(k)
				bestCat = cat
			}
		}
	}
	if bestLen >= 0 {
		return bestCat, true
	}
	return "", false
}

// matchHeuristic 名字启发式匹配。返回命中分类与是否命中。
func matchHeuristic(norm string) (Category, bool) {
	for _, r := range heuristicRules {
		for _, s := range r.subs {
			if strings.Contains(norm, s) {
				return r.cat, true
			}
		}
	}
	return "", false
}

// Classify 三层分类：内置元数据精确匹配 → 名字启发式 → 未分类兜底。
// ModelID 保留原始输入；判定基于归一化 id。
func Classify(modelID string) Classification {
	norm := normalizeID(modelID)
	if cat, ok := matchMetadata(norm); ok {
		return Classification{ModelID: modelID, Category: cat, Source: SourceMetadata}
	}
	if cat, ok := matchHeuristic(norm); ok {
		return Classification{ModelID: modelID, Category: cat, Source: SourceHeuristic}
	}
	return Classification{ModelID: modelID, Category: CategoryUnknown, Source: SourceFallback}
}

// allDigits 报告 s 是否为非空纯数字串。
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// normalizeBase 归一化去版本/日期后缀（反复剥离尾部纯数字段），用于掉包比对。
func normalizeBase(s string) string {
	s = normalizeID(s)
	for {
		i := strings.LastIndex(s, "-")
		if i < 0 {
			break
		}
		if !allDigits(s[i+1:]) {
			break
		}
		s = s[:i]
	}
	return s
}

// ModelMismatch 报告端点实际应答模型与请求模型是否不一致（中转掉包信号，UI 标记用，不阻断）。
// 按归一化基名比较（去日期/版本后缀）；returned 为空视为无信号，返回 false。
func ModelMismatch(requested, returned string) bool {
	if strings.TrimSpace(returned) == "" {
		return false
	}
	return normalizeBase(requested) != normalizeBase(returned)
}

// BuiltinCandidates 返回内置元数据表里的全部模型基名（升序），用于 /models 不可用时
// 降级为"手动输入模型 id + 候选列表"的候选来源。
func BuiltinCandidates() []string {
	out := make([]string, 0, len(metadataTable))
	for k := range metadataTable {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
