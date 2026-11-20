package hippocampus

import (
	"testing"
	"time"
)

// 召回打分公式 = sim × (α+βR) × (γ+imp+emo)。
func TestRecallScoreFormula(t *testing.T) {
	got := recallScore(0.9, 0.5, 0.4, 0.2, 0.3, 0.7, 0.1)
	// 0.9 * (0.3+0.7*0.5) * (0.1+0.4+0.2) = 0.9 * 0.65 * 0.7 = 0.4095
	want := 0.9 * (0.3 + 0.7*0.5) * (0.1 + 0.4 + 0.2)
	if !approx(got, want, 1e-9) {
		t.Errorf("recallScore = %v, want %v", got, want)
	}
}

// 低于阈值不进上下文；高于阈值进；按分降序。
func TestRecallThresholdAndSort(t *testing.T) {
	clk := baseClock()
	e := New(Options{Clock: clk, Threshold: 0.15})
	e.Encode("hot", "", 0.8, 0.8)  // 高分
	e.Encode("cold", "", 0.0, 0.0) // 低分
	cands := []Candidate{
		{Memory: mustMem(e, "hot"), Similarity: 0.9},
		{Memory: mustMem(e, "cold"), Similarity: 0.05}, // sim 低 → 分低被滤
	}
	res := e.Recall(cands)
	if len(res) != 1 || res[0].Memory.ID != "hot" {
		t.Fatalf("recall = %+v, want 只 hot", res)
	}
	if res[0].Score < 0.15 {
		t.Errorf("保留项应 >= 阈值")
	}
}

// 强线索"突然想起"：高 importance/emotion + 强语义线索，即使分数低于严格阈值也强制纳入；
// 普通记忆同 sim 但低 imp/emo 不 surprise、被阈值过滤。
func TestSurpriseRecall(t *testing.T) {
	clk := baseClock()
	e := New(Options{Clock: clk, Threshold: 0.9}) // 极严格阈值
	e.Encode("trauma", "", 0.9, 0.9)              // 高情绪事件，S 大
	e.Encode("trivial", "", 0.0, 0.0)             // 平淡

	// 让高情绪记忆也衰减不少，但仍靠强线索召回。
	clk.Advance(168 * time.Hour)

	cands := []Candidate{
		{Memory: mustMem(e, "trauma"), Similarity: 0.9},  // 强线索
		{Memory: mustMem(e, "trivial"), Similarity: 0.9}, // 同 sim 但平淡
	}
	res := e.Recall(cands)
	if len(res) != 1 || res[0].Memory.ID != "trauma" {
		t.Fatalf("只应强线索 trauma 纳入, got %+v", res)
	}
	if !res[0].Surprise {
		t.Error("trauma 应标记 Surprise=true")
	}
	if res[0].Score >= e.threshold {
		t.Errorf("示范前提：trauma 分数应低于严格阈值, got %v", res[0].Score)
	}
}

// 强线索相似度不足则不 surprise（有 emotion 但语义不相关）。
func TestSurpriseRequiresSim(t *testing.T) {
	clk := baseClock()
	e := New(Options{Clock: clk, Threshold: 0.9})
	e.Encode("trauma", "", 0.9, 0.9)
	clk.Advance(168 * time.Hour)
	res := e.Recall([]Candidate{{Memory: mustMem(e, "trauma"), Similarity: 0.2}}) // sim < strongSim
	if len(res) != 0 {
		t.Errorf("语义不相关不应 surprise 纳入, got %+v", res)
	}
}

// RecallFromStore 便捷接口：全量以相似度函数召回。
func TestRecallFromStore(t *testing.T) {
	clk := baseClock()
	e := New(Options{Clock: clk, Threshold: 0.01})
	e.Encode("a", "苹果", 0.5, 0.5)
	e.Encode("b", "香蕉", 0.5, 0.5)
	res := e.RecallFromStore(func(m Memory) float64 {
		if m.ID == "a" {
			return 0.9
		}
		return 0.1
	})
	if len(res) == 0 || res[0].Memory.ID != "a" {
		t.Errorf("RecallFromStore top = %+v, want a", res)
	}
}

func mustMem(e *Engine, id string) Memory {
	m, _ := e.Get(id)
	return m
}
