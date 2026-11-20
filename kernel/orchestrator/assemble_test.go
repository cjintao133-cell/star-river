package orchestrator

import (
	"strings"
	"testing"
	"unicode/utf8"

	"star-river/kernel/core/types"
)

func txtMsg(role types.Role, s string) types.Message {
	return types.Message{Role: role, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: s}}}
}

// fullLayers 构造七层都有内容的输入。
func fullLayers(recentN int) Layers {
	var recent []types.Message
	for i := 0; i < recentN; i++ {
		recent = append(recent, txtMsg(types.RoleUser, "msg"+itoa(i)))
	}
	return Layers{
		System:  "系统指令",
		Persona: "你是示例角色",
		World:   []string{"世界观A", "世界观B"},
		Memory:  []string{"重要记忆1", "重要记忆2"},
		Summary: "此前聊了这些",
		Recent:  recent,
		Current: txtMsg(types.RoleUser, "现在这句"),
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// 不限预算：全保留，七层齐全。
func TestAssembleUnlimited(t *testing.T) {
	a := NewAssembler(Config{TotalChars: 0}) // 0 = 不限
	rep := a.Assemble(fullLayers(6), nil)
	if rep.Truncated {
		t.Error("不限预算不应截断")
	}
	var sysFound, curFound bool
	for _, m := range rep.Messages {
		if m.Role == types.RoleSystem && strings.Contains(m.Blocks[0].Text, "人设") {
			sysFound = true
		}
		if m.Role == types.RoleUser && m.Blocks[0].Text == "现在这句" {
			curFound = true
		}
	}
	if !sysFound || !curFound {
		t.Errorf("七层应齐全: sys=%v cur=%v", sysFound, curFound)
	}
	// 摘要、世界、记忆都进 system。
	sysText := rep.Messages[0].Blocks[0].Text
	for _, want := range []string{"系统指令", "对话摘要", "世界观A", "重要记忆1", "重要记忆2"} {
		if !strings.Contains(sysText, want) {
			t.Errorf("system 缺 %q", want)
		}
	}
}

// 人设与本次输入绝不裁（即使预算极小到装不下）。
func TestAssemblePersonaAndCurrentNeverDropped(t *testing.T) {
	a := NewAssembler(Config{TotalChars: 20, MinRecentMessages: 1})
	rep := a.Assemble(fullLayers(3), nil)
	var sysHasPersona, curFound bool
	for _, m := range rep.Messages {
		if m.Role == types.RoleSystem && strings.Contains(m.Blocks[0].Text, "你是示例角色") {
			sysHasPersona = true
		}
		if m.Role == types.RoleUser && m.Blocks[0].Text == "现在这句" {
			curFound = true
		}
	}
	if !sysHasPersona {
		t.Error("人设被裁了（不可压下限失效）")
	}
	if !curFound {
		t.Error("本次输入被裁了")
	}
	if !rep.Truncated {
		t.Error("极小预算应标记 Truncated")
	}
}

// 近期消息保留至少 MinRecentMessages 条（即使预算紧）。
func TestAssembleRecentFloor(t *testing.T) {
	a := NewAssembler(Config{TotalChars: 30, MinRecentMessages: 3})
	rep := a.Assemble(fullLayers(10), nil)
	// 数一下近期消息（role user 且文本形如 msgN，排除 current "现在这句"）。
	n := 0
	for _, m := range rep.Messages {
		if m.Role == types.RoleUser && strings.HasPrefix(m.Blocks[0].Text, "msg") {
			n++
		}
	}
	if n < 3 {
		t.Errorf("近期消息保留 %d 条, 应 >= 下限 3", n)
	}
	// 保留的应是最近的若干条（floor 是最后 3 条 msg7,8,9）+ 可能的一些更旧。
	last := rep.Messages[len(rep.Messages)-2] // current 前一条应是 msg9
	if !strings.HasPrefix(last.Blocks[0].Text, "msg9") {
		t.Errorf("最新消息应是 msg9, got %q", last.Blocks[0].Text)
	}
}

// 压缩从低优先级：预算只够保留高优先层时，摘要(20)先被丢，记忆(60)优先于世界书(40)保留。
func TestAssembleCompressLowPriorityFirst(t *testing.T) {
	// 不可压：system+persona+current+recent(floor=1最新) 已占不少，剩余给可压层。
	// 构造：让摘要一定被丢、记忆一定保留、世界书可能被丢。
	in := Layers{
		System:  "", // 去掉系统，减少不可压基线差异
		Persona: "P",
		World:   []string{"W1", "W2"},
		Memory:  []string{"MEM1"},
		Summary: "SUMMARY-TEXT-LONG",
		Recent:  []types.Message{txtMsg(types.RoleUser, "r1")},
		Current: txtMsg(types.RoleUser, "c"),
	}
	// 估算：persona(1)+current(1)+recent floor1(2)=4。留给可压。
	// 记忆 MEM1(4) 优先级 60；世界 W1(2),W2(2) 优先级 40；摘要 18 优先级 20。
	// 给预算 = 4 + 恰够记忆(4) + 少量（不够摘要）。
	a := NewAssembler(Config{TotalChars: 9, MinRecentMessages: 1})
	rep := a.Assemble(in, nil)
	sysText := ""
	for _, m := range rep.Messages {
		if m.Role == types.RoleSystem {
			sysText = m.Blocks[0].Text
		}
	}
	if strings.Contains(sysText, "SUMMARY-TEXT") {
		t.Error("低优先级摘要应被压掉")
	}
	if !strings.Contains(sysText, "MEM1") {
		t.Errorf("高优先级记忆应保留, sys=%q", sysText)
	}
	if !rep.Truncated {
		t.Error("应标记 Truncated")
	}
}

// 截断前强制记忆沉淀：被裁的近期消息交给 sink。
func TestAssembleDroppedRecentSunk(t *testing.T) {
	var sunk []types.Message
	sink := MemorySinkFunc(func(msgs []types.Message) { sunk = append(sunk, msgs...) })
	// 10 条近期，预算只够 floor(2) 且没余量留更旧的。
	a := NewAssembler(Config{TotalChars: 20, MinRecentMessages: 2})
	rep := a.Assemble(fullLayers(10), sink)
	if len(rep.DroppedRecent) == 0 {
		t.Fatal("应有被裁的近期消息")
	}
	if len(sunk) != len(rep.DroppedRecent) {
		t.Errorf("sink 收到 %d 条, 应等于丢弃 %d 条", len(sunk), len(rep.DroppedRecent))
	}
	// 被裁的应是较旧的（msg0..），不是最新的。
	for _, m := range sunk {
		if m.Blocks[0].Text == "msg9" {
			t.Error("最新消息不应被裁")
		}
	}
}

// 摘要按字符截断不切坏 UTF-8。
func TestAssembleSummaryUTF8Truncate(t *testing.T) {
	// 摘要含中文（每字 3 字节）。给紧预算让它被字符截断。
	in := Layers{
		Persona: "P",
		Summary: "摘要中文内容测试",
		Current: txtMsg(types.RoleUser, "c"),
	}
	a := NewAssembler(Config{TotalChars: 8, MinRecentMessages: 1}) // persona1+cur1=2, 余 6
	rep := a.Assemble(in, nil)
	var sysText string
	for _, m := range rep.Messages {
		if m.Role == types.RoleSystem {
			sysText = m.Blocks[0].Text
		}
	}
	if !utf8.ValidString(sysText) {
		t.Errorf("system 文本不是合法 UTF-8（截断切坏多字节）: %q", sysText)
	}
}

// UsedChars 各层统计之和 == TotalUsed。
func TestAssembleUsedCharsSum(t *testing.T) {
	a := NewAssembler(Config{TotalChars: 0})
	rep := a.Assemble(fullLayers(4), nil)
	sum := 0
	for i := 0; i < int(NumLayers); i++ {
		sum += rep.UsedChars[i]
	}
	if sum != rep.TotalUsed {
		t.Errorf("各层和 %d != TotalUsed %d", sum, rep.TotalUsed)
	}
}

// 上下文溢出无缝接力集成测试：多轮对话增长，每轮以固定预算组装；
// 任何一时刻"待在上下文里的 + 已沉淀到记忆的"必须覆盖全部历史消息，无一静默丢失。
func TestOverflowRelayNoSilentLoss(t *testing.T) {
	a := NewAssembler(Config{TotalChars: 60, MinRecentMessages: 3})
	var all []types.Message
	sunk := map[string]bool{}
	var sunkCount int
	sink := MemorySinkFunc(func(msgs []types.Message) {
		for _, m := range msgs {
			if !sunk[m.Blocks[0].Text] {
				sunk[m.Blocks[0].Text] = true
				sunkCount++
			}
		}
	})
	const turns = 30
	for i := 0; i < turns; i++ {
		all = append(all, txtMsg(types.RoleUser, "turn-"+itoa(i)))
		in := Layers{
			Persona: "P",
			Recent:  all,
			Current: txtMsg(types.RoleUser, "now"),
		}
		a.Assemble(in, sink)
	}
	// 统计"待在上下文里的历史消息"（最后一轮的分配结果）。
	in := Layers{Persona: "P", Recent: all, Current: txtMsg(types.RoleUser, "now")}
	rep := a.Assemble(in, sink)
	inCtx := map[string]bool{}
	for _, m := range rep.Messages {
		if m.Role == types.RoleUser && strings.HasPrefix(m.Blocks[0].Text, "turn-") {
			inCtx[m.Blocks[0].Text] = true
		}
	}
	// 每条历史要么在最终上下文，要么在沉淀集合。
	for _, m := range all {
		txt := m.Blocks[0].Text
		if !inCtx[txt] && !sunk[txt] {
			t.Errorf("消息 %q 既不在上下文也未沉淀（静默丢失）", txt)
		}
	}
	if sunkCount == 0 {
		t.Error("溢出应发生过沉淀")
	}
	if len(inCtx) < 3 {
		t.Errorf("近期下限应保留 >=3 条, got %d", len(inCtx))
	}
}
