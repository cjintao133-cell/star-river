package discovery

import (
	"strings"

	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

// 能力位来源/置信（哪些是实测、哪些是元数据、哪些是推测）。
const (
	ConfProbed    = "probed"    // 实测探测确证
	ConfMetadata  = "metadata"  // 端点元数据声明
	ConfHeuristic = "heuristic" // 名字/惯例推测
	ConfUnknown   = "unknown"   // 未探测未声明
)

// 能力位 key（用于 CapsSource 标注）。
const (
	CapVision    = "vision"
	CapTools     = "tools"
	CapReasoning = "reasoning"
	CapEmbedding = "embedding"
	CapStreaming = "streaming"
)

// ModelMeta 端点为某模型返回的富元数据（各协议映射到此中性结构，缺失即零值）。
type ModelMeta struct {
	ID               string
	Provider         string
	ContextLength    int      // 上下文窗口（0=未声明）
	InputModalities  []string // 如 ["text","image","audio"]
	OutputModalities []string
	DeclaredCaps     map[string]bool // 端点直接声明的能力（键同 Cap* 常量）
}

// CapabilityCard 一个模型的「能力底裤」：分类 + 能力位 + 每位来源 + 上下文/模态。
type CapabilityCard struct {
	ModelID       string
	Provider      string
	Category      Category
	ClassSource   string // 分类判定来源（metadata/heuristic/fallback）
	Caps          types.ModelCapabilities
	CapsSource    map[string]string // 能力位 -> Conf*（缺省视为 unknown）
	ContextLength int
	Modalities    []string
}

// sourceOf 取能力位来源，缺省 unknown。
func (c CapabilityCard) sourceOf(key string) string {
	if v, ok := c.CapsSource[key]; ok {
		return v
	}
	return ConfUnknown
}

// setCap 仅在来源优先级更高时覆盖（probed > metadata > heuristic > unknown）。
func (c *CapabilityCard) setCap(key string, val bool, src string) {
	if !val {
		return
	}
	if rank(src) >= rank(c.sourceOf(key)) {
		c.CapsSource[key] = src
		switch key {
		case CapVision:
			c.Caps.Vision = true
		case CapTools:
			c.Caps.Tools = true
		case CapReasoning:
			c.Caps.Reasoning = true
		case CapEmbedding:
			c.Caps.Embedding = true
		case CapStreaming:
			c.Caps.Streaming = true
		}
	}
}

// rank 来源优先级。
func rank(src string) int {
	switch src {
	case ConfProbed:
		return 3
	case ConfMetadata:
		return 2
	case ConfHeuristic:
		return 1
	default:
		return 0
	}
}

// BuildCard 合并三方证据生成能力卡：
//  1. probe 非 nil：以实测为准（ConfProbed），压过元数据/启发式；
//  2. 端点元数据（DeclaredCaps / InputModalities）（ConfMetadata）；
//  3. 名字/惯例启发式（ConfHeuristic）兜底；
//
// 分类由 Classify 给出。probe 为 nil 表示未实测（不烧钱，懒探测时再补）。
func BuildCard(meta ModelMeta, probe *types.ModelCapabilities) CapabilityCard {
	cls := Classify(meta.ID)
	card := CapabilityCard{
		ModelID:       meta.ID,
		Provider:      meta.Provider,
		Category:      cls.Category,
		ClassSource:   cls.Source,
		CapsSource:    map[string]string{},
		ContextLength: meta.ContextLength,
		Modalities:    mergeModalities(meta.InputModalities, meta.OutputModalities),
	}
	card.Caps.ModelID = meta.ID

	// 1) 实测优先。
	if probe != nil {
		card.setCap(CapVision, probe.Vision, ConfProbed)
		card.setCap(CapTools, probe.Tools, ConfProbed)
		card.setCap(CapReasoning, probe.Reasoning, ConfProbed)
		card.setCap(CapEmbedding, probe.Embedding, ConfProbed)
		card.setCap(CapStreaming, probe.Streaming, ConfProbed)
	}

	// 2) 元数据声明。
	for k, v := range meta.DeclaredCaps {
		card.setCap(k, v, ConfMetadata)
	}
	if hasModality(meta.InputModalities, "image") || hasModality(meta.OutputModalities, "image") {
		card.setCap(CapVision, true, ConfMetadata)
	}

	// 3) 启发式兜底。
	hint := HeuristicCaps(meta.ID)
	for k, v := range hint {
		card.setCap(k, v, ConfHeuristic)
	}
	// 流式：绝大多数 chat 模型支持，作为惯例推测（实测会覆盖）。
	card.setCap(CapStreaming, true, ConfHeuristic)

	return card
}

// BuildCards 批量建卡。probeFn 可为 nil（全部不实测）。已知探测结果的模型走实测。
func BuildCards(metas []ModelMeta, probeFn func(id string) (*types.ModelCapabilities, bool)) []CapabilityCard {
	out := make([]CapabilityCard, 0, len(metas))
	for _, m := range metas {
		var probe *types.ModelCapabilities
		if probeFn != nil {
			if p, ok := probeFn(m.ID); ok {
				probe = p
			}
		}
		out = append(out, BuildCard(m, probe))
	}
	return out
}

// HeuristicCaps 由模型 id 名字推断能力位（尽力而为，UI 提示级）。
// 返回 Map[能力位Key]bool；未涉及的键不出现。
func HeuristicCaps(id string) map[string]bool {
	n := normalizeID(id)
	out := map[string]bool{}
	// Embedding 专属模型。
	if containsAny(n, "embed", "bge-", "text-embedding", "e5-", "gte-", "m3e") {
		out[CapEmbedding] = true
		return out // embedding 模型通常不是 chat 模型，不再判其他
	}
	// 视觉。
	if containsAny(n, "-vl", "vl-", "vision", "llava", "gpt-4o", "gpt-4.1", "gemini", "claude-3", "claude-4", "claude-sonnet", "claude-opus", "qwen2-vl", "qwen2.5-vl", "internvl", "pixtral") {
		out[CapVision] = true
	}
	// 推理。
	if containsAny(n, "reason", "thinking", "qwq", "o1", "o3", "o4", "-r1", "r1-", "deepseek-r1", "qvq") {
		out[CapReasoning] = true
	}
	// 工具调用：主流系列惯例支持（推测级）。
	if containsAny(n, "gpt-4", "gpt-4o", "gpt-4.1", "gpt-5", "claude", "gemini", "qwen", "glm-4", "deepseek", "mistral", "llama-3", "command-r") {
		out[CapTools] = true
	}
	return out
}

// containsAny 报告 n 是否含任一子串。
func containsAny(n string, subs ...string) bool {
	for _, s := range subs {
		if strings.Contains(n, s) {
			return true
		}
	}
	return false
}

// hasModality 报告模态列表是否含某模态（大小写不敏感）。
func hasModality(mods []string, want string) bool {
	for _, m := range mods {
		if strings.EqualFold(m, want) {
			return true
		}
	}
	return false
}

// mergeModalities 合并去重输入/输出模态。
func mergeModalities(in, out []string) []string {
	seen := map[string]bool{}
	var res []string
	for _, m := range append(append([]string{}, in...), out...) {
		k := strings.ToLower(strings.TrimSpace(m))
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		res = append(res, k)
	}
	return res
}
