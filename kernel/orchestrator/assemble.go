package orchestrator

// T-M3-03：七层上下文 + 预算分配器。
//
// 七层（优先级从高到低默认见 DefaultConfig）：系统指令 / 人设 / 世界书 / 长期记忆 /
// 滚动摘要 / 近期消息 / 本次用户输入。
//
// 组装规则：
//   - 不可压下限：系统、人设、本次用户输入整段保留；近期消息至少保留 MinRecentMessages 条。
//   - 可压层（世界书/记忆/摘要 + 近期消息超额部分）按优先级配额；总预算超了，
//     从优先级最低的层开始压缩/裁剪（整条目级拟合，摘要按字符截断）。
//   - 截断前强制记忆沉淀：被裁掉的近期消息经 MemorySink 交给 hippocampus 落库，
//     不静默丢失（滚动摘要 + 强制沉淀的接线点）。

import "star-river/kernel/core/types"

// Layer 上下文层。
type Layer int

const (
	LaySystem  Layer = iota // 系统指令
	LayPersona              // 人设
	LayWorld                // 世界书/设定
	LayMemory               // 长期记忆召回
	LaySummary              // 滚动摘要
	LayRecent               // 近期消息
	LayCurrent              // 本次用户输入
	NumLayers
)

// Config 预算配置。TotalChars<=0 表示不限预算（全量保留）。
type Config struct {
	TotalChars        int
	MinRecentMessages int            // 近期消息不可压下限条数（默认 4）
	Priority          [NumLayers]int // 各层优先级（越大越先保；压缩从最小起）
}

// DefaultConfig 给定总预算的默认配置。优先级：人设/系统/本次最高，近期次之，
// 记忆 > 世界书 > 摘要（摘要最易压）。
func DefaultConfig(totalChars int) Config {
	var p [NumLayers]int
	p[LaySystem] = 100
	p[LayPersona] = 100
	p[LayCurrent] = 100
	p[LayRecent] = 80
	p[LayMemory] = 60
	p[LayWorld] = 40
	p[LaySummary] = 20
	return Config{TotalChars: totalChars, MinRecentMessages: 4, Priority: p}
}

// Layers 七层原始输入。
type Layers struct {
	System  string
	Persona string
	World   []string        // 世界书条目文本
	Memory  []string        // 召回记忆条目文本
	Summary string          // 滚动摘要
	Recent  []types.Message // 近期消息（时间正序，最后一条最新）
	Current types.Message   // 本次用户输入
}

// MemorySink 截断前强制记忆沉淀钩子：把被裁的近期消息交给记忆引擎落库。
type MemorySink interface {
	Sink(msgs []types.Message)
}

// MemorySinkFunc 函数适配。
type MemorySinkFunc func(msgs []types.Message)

// Sink 实现 MemorySink。
func (f MemorySinkFunc) Sink(msgs []types.Message) { f(msgs) }

// Report 组装结果。
type Report struct {
	Messages      []types.Message
	UsedChars     [NumLayers]int
	TotalUsed     int
	DroppedRecent []types.Message // 被裁掉的近期消息（已交 sink 沉淀）
	Truncated     bool            // 是否发生过压缩/裁剪
}

// Assembler 预算分配器。
type Assembler struct {
	cfg Config
}

// NewAssembler 建分配器；优先级/下限为 0 时套默认。
func NewAssembler(cfg Config) *Assembler {
	d := DefaultConfig(cfg.TotalChars)
	if cfg.MinRecentMessages <= 0 {
		cfg.MinRecentMessages = d.MinRecentMessages
	}
	for i := 0; i < int(NumLayers); i++ {
		if cfg.Priority[i] == 0 {
			cfg.Priority[i] = d.Priority[i]
		}
	}
	return &Assembler{cfg: cfg}
}

func textLen(s string) int { return len(s) }

// Assemble 组装上下文并返回报告。sink 可为 nil（不沉淀）。
func (a *Assembler) Assemble(in Layers, sink MemorySink) Report {
	var rep Report

	// 不可压层：系统、人设、本次用户输入（整段保留）。
	sysText := in.System
	personaText := in.Persona
	currentChars := msgChars(in.Current)
	mandatory := textLen(sysText) + textLen(personaText) + currentChars

	// 近期消息不可压下限：保留最近 MinRecentMessages 条。
	recent := in.Recent
	floorN := a.cfg.MinRecentMessages
	if floorN > len(recent) {
		floorN = len(recent)
	}
	// 下限消息的字符数。
	floorRecent := recent[len(recent)-floorN:]
	floorRecentChars := 0
	for _, m := range floorRecent {
		floorRecentChars += msgChars(m)
	}
	mandatory += floorRecentChars

	// 超出预算的部分，按优先级分配给可压单元。
	budget := a.cfg.TotalChars
	unlimited := budget <= 0
	remaining := 0
	if !unlimited {
		remaining = budget - mandatory
		if remaining < 0 {
			// 连不可压都超预算：仍保不可压（人设/本次绝不丢），剩余按 0。
			remaining = 0
			rep.Truncated = true
		}
	}

	// 可压单元：世界书条目、记忆条目、摘要、近期消息超额部分（floorN 之前更旧的）。
	// 每单元 = 独立层，按优先级从高到低分配；同级内按输入顺序。
	var units []unit
	if tc := textLen(in.Summary); tc > 0 {
		units = append(units, unit{layer: LaySummary, kind: 0, chars: tc})
	}
	for i, w := range in.World {
		units = append(units, unit{layer: LayWorld, kind: 1, chars: textLen(w), index: i})
	}
	for i, m := range in.Memory {
		units = append(units, unit{layer: LayMemory, kind: 2, chars: textLen(m), index: i})
	}
	olderRecent := recent[:len(recent)-floorN]
	for i, m := range olderRecent {
		units = append(units, unit{layer: LayRecent, kind: 3, chars: msgChars(m), index: i})
	}

	// 按层优先级降序（稳定排序，保留同层输入顺序）。
	// 但近期超额消息应从最新往前保留：先反转其顺序再排序，分配后再复原。
	// 简化：对 kind==3 单元用「越新越优先」——把 index 越大视为越高优先（同层内）。
	sortUnitsByPriority(units, a.cfg.Priority)

	// 分配结果标记。
	includeSummary := false
	summaryLen := 0
	includeWorld := make([]bool, len(in.World))
	includeMemory := make([]bool, len(in.Memory))
	includeOlderRecent := make([]bool, len(olderRecent))

	for _, u := range units {
		if unlimited {
			// 全保留。
			switch u.kind {
			case 0:
				includeSummary, summaryLen = true, u.chars
			case 1:
				includeWorld[u.index] = true
			case 2:
				includeMemory[u.index] = true
			case 3:
				includeOlderRecent[u.index] = true
			}
			continue
		}
		if u.chars <= remaining {
			remaining -= u.chars
			switch u.kind {
			case 0:
				includeSummary, summaryLen = true, u.chars
			case 1:
				includeWorld[u.index] = true
			case 2:
				includeMemory[u.index] = true
			case 3:
				includeOlderRecent[u.index] = true
			}
		} else {
			rep.Truncated = true
			// 摘要可按字符部分截断；条目/消息整条拟合，放不下则丢弃。
			if u.kind == 0 && remaining > 0 {
				summaryLen = remaining
				includeSummary = true
				remaining = 0
			}
			// 其余丢弃（低优先级）
		}
	}

	// —— 组装消息（按层顺序：system, persona(合并进system), world/记忆/摘要(合并进system 或独立 system 消息), recent, current）。
	// 为兼容适配器（system 单条 + user/assistant 交替），把 system/persona/world/memory/summary
	// 合并为一条 leading system 消息（分段）。
	var sysParts []string
	if sysText != "" {
		sysParts = append(sysParts, sysText)
	}
	if personaText != "" {
		sysParts = append(sysParts, "【人设】\n"+personaText)
	}
	if includeSummary && summaryLen > 0 {
		s := in.Summary
		if summaryLen < len(s) {
			s = truncateUTF8(s, summaryLen)
		}
		sysParts = append(sysParts, "【对话摘要】\n"+s)
	}
	for i, w := range in.World {
		if includeWorld[i] {
			sysParts = append(sysParts, "【设定】"+w)
		}
	}
	for i, m := range in.Memory {
		if includeMemory[i] {
			sysParts = append(sysParts, "【记忆】"+m)
		}
	}

	if len(sysParts) > 0 {
		joined := joinSections(sysParts)
		rep.Messages = append(rep.Messages, types.Message{
			Role:   types.RoleSystem,
			Blocks: []types.ContentBlock{{Type: types.BlockText, Text: joined}},
		})
		rep.UsedChars[LaySystem] = textLen(sysText)
		rep.UsedChars[LayPersona] = textLen(personaText)
		if includeSummary {
			rep.UsedChars[LaySummary] = summaryLen
		}
		for i := range in.World {
			if includeWorld[i] {
				rep.UsedChars[LayWorld] += textLen(in.World[i])
			}
		}
		for i := range in.Memory {
			if includeMemory[i] {
				rep.UsedChars[LayMemory] += textLen(in.Memory[i])
			}
		}
	}

	// 近期消息：先放超额部分中保留的（较旧的），再放下限（最新 floorN 条）。
	var keptRecent []types.Message
	for i := range olderRecent {
		if includeOlderRecent[i] {
			keptRecent = append(keptRecent, olderRecent[i])
			rep.UsedChars[LayRecent] += msgChars(olderRecent[i])
		} else {
			rep.DroppedRecent = append(rep.DroppedRecent, olderRecent[i])
		}
	}
	for _, m := range floorRecent {
		keptRecent = append(keptRecent, m)
		rep.UsedChars[LayRecent] += msgChars(m)
	}
	rep.Messages = append(rep.Messages, keptRecent...)
	rep.Messages = append(rep.Messages, in.Current)
	rep.UsedChars[LayCurrent] = currentChars

	for i := 0; i < int(NumLayers); i++ {
		rep.TotalUsed += rep.UsedChars[i]
	}

	// 截断前强制记忆沉淀：被裁的近期消息交 sink（不静默丢失）。
	if len(rep.DroppedRecent) > 0 && sink != nil {
		sink.Sink(rep.DroppedRecent)
	}
	return rep
}

// truncateUTF8 截断到不超过 max 字节且不切坏 UTF-8。
func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && (s[cut]&0xC0) == 0x80 { // 回退到符文边界
		cut--
	}
	return s[:cut]
}

// joinSections 用空行连接分段。
func joinSections(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "\n\n"
		}
		out += p
	}
	return out
}

// unit 一个可压分配单元。
type unit struct {
	layer Layer
	kind  int // 0=摘要文本, 1=world 条目, 2=memory 条目, 3=recent 超额消息
	chars int
	index int
}

// sortUnitsByPriority 按层优先级降序；同层内 recent(kind3) 按 index 降序（越新越优先），
// 其余按 index 升序（保持输入顺序）。插入排序（单元数不大）。
func sortUnitsByPriority(units []unit, prio [NumLayers]int) {
	for i := 1; i < len(units); i++ {
		for j := i; j > 0; j-- {
			if lessUnit(units[j], units[j-1], prio) {
				units[j], units[j-1] = units[j-1], units[j]
			} else {
				break
			}
		}
	}
}

func lessUnit(a, b unit, prio [NumLayers]int) bool {
	if prio[a.layer] != prio[b.layer] {
		return prio[a.layer] > prio[b.layer] // 高优先在前
	}
	if a.kind == 3 && b.kind == 3 {
		return a.index > b.index // 越新越先分配
	}
	return a.index < b.index
}
