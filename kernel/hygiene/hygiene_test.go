package hygiene

import (
	"testing"

	"github.com/cjintao133-cell/star-river/kernel/hippocampus"
)

const dayMs = int64(24 * 60 * 60 * 1000)

func TestPruneDropsStaleLowValue(t *testing.T) {
	now := int64(100 * dayMs)
	mems := []hippocampus.Memory{
		// 久未复习(190天前)、低留存、低重要 -> 丢
		{ID: "a", Text: "废信息", Importance: 0.1, Stability: 1, LastReviewAt: now - 190*dayMs},
		// 高重要 -> 留
		{ID: "b", Text: "关键信息", Importance: 0.9, Stability: 1, LastReviewAt: now - 190*dayMs},
		// 近期复习 -> 留存高 -> 留
		{ID: "c", Text: "最近记忆", Importance: 0.1, Stability: 1, LastReviewAt: now},
	}
	rep := Prune(mems, now, Config{})
	if rep.Dropped != 1 || rep.DropIdx[0] != 0 {
		t.Fatalf("应只丢第 0 条: %+v", rep)
	}
	if rep.Kept != 2 {
		t.Fatalf("应留 2: %+v", rep)
	}
}

func TestPruneConfigOverride(t *testing.T) {
	now := int64(100 * dayMs)
	mems := []hippocampus.Memory{{ID: "a", Importance: 0.5, Stability: 1, LastReviewAt: now - 190*dayMs}}
	// 提高重要度门槛到 0.6 -> 0.5 也满足"低重要"，但仍需留存低+久未；190天 St=1 留存极低 -> 丢
	rep := Prune(mems, now, Config{MinImportance: 0.6})
	if rep.Dropped != 1 {
		t.Fatalf("提高门槛后应丢: %+v", rep)
	}
	// 门槛降到 0.4 -> 0.5 不算低重要 -> 不丢
	rep = Prune(mems, now, Config{MinImportance: 0.4})
	if rep.Dropped != 0 {
		t.Fatalf("门槛降低后不应丢: %+v", rep)
	}
}

func TestDuplicatesExactAndFuzzy(t *testing.T) {
	mems := []hippocampus.Memory{
		{ID: "1", Text: "用户喜欢喝咖啡。", Importance: 0.5},
		{ID: "2", Text: "用户喜欢喝咖啡。", Importance: 0.8}, // 完全相同
		{ID: "3", Text: "今天天气不错。", Importance: 0.3},
	}
	groups := Duplicates(mems, Config{})
	if len(groups) != 1 {
		t.Fatalf("应 1 组重复: %+v", groups)
	}
	if groups[0].KeepIdx != 1 { // 重要度高的 "2" 保留
		t.Fatalf("应保留重要度更高的第 1 条: %+v", groups[0])
	}
	if len(groups[0].DropIdxs) != 1 || groups[0].DropIdxs[0] != 0 {
		t.Fatalf("应丢第 0 条: %+v", groups[0])
	}
}

func TestDuplicatesNoFalsePositive(t *testing.T) {
	mems := []hippocampus.Memory{
		{ID: "1", Text: "我喜欢猫", Importance: 0.5},
		{ID: "2", Text: "今天要开会", Importance: 0.5},
	}
	if g := Duplicates(mems, Config{}); len(g) != 0 {
		t.Fatalf("不应误判重复: %+v", g)
	}
}

func TestMergedText(t *testing.T) {
	got := MergedText([]string{"用户喜欢咖啡。用户住在武汉。", "用户喜欢咖啡。"})
	if got != "用户喜欢咖啡；用户住在武汉" {
		t.Fatalf("合并去重不对: %q", got)
	}
}
