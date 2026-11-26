package mobilebind

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// 真跑：1200 条记忆不丢（旧版 500 上限会丢 700 条），且中文混合检索命中。
func TestMemoryScaleNoCapAndHybrid(t *testing.T) {
	initTestKernel(t)
	for i := 0; i < 1200; i++ {
		kind := "episodic"
		txt := fmt.Sprintf("第%d条记忆 普通内容", i)
		if i == 600 {
			txt = "用户的生日是5月1日，喜欢喝咖啡"
			kind = "semantic"
		}
		if r := AddMemory(kind, txt, 0.5); strings.HasPrefix(r, "ERR") {
			t.Fatalf("AddMemory %d: %s", i, r)
		}
	}
	if n := MemoryCount(); n < 1200 {
		t.Fatalf("记忆数应 >=1200，实际 %d（说明仍有截断）", n)
	}
	// 中文关键词（3+ 字）应能命中
	hits := SearchMemory("生日 咖啡", 5)
	if !strings.Contains(hits, "生日") {
		t.Fatalf("混合检索未命中生日记忆: %s", hits)
	}
	// 英文/数字关键词
	_ = SearchMemory("第777条", 5)
	t.Logf("memory count=%d", MemoryCount())
}

func TestKbIngestAndSearch(t *testing.T) {
	initTestKernel(t)
	doc := strings.Repeat("人工智能代理需要长期记忆与知识库支持。检索增强生成通过向量和关键词混合召回。", 40)
	var res map[string]any
	json.Unmarshal([]byte(KbIngest("doc1", "测试文档", "unit", doc)), &res)
	if int(res["chunks"].(float64)) < 2 {
		t.Fatalf("应分成多块: %+v", res)
	}
	out := KbSearch("知识库", 3)
	if !strings.Contains(out, "doc1") {
		t.Fatalf("知识库检索未命中: %s", out)
	}
	var st map[string]int
	json.Unmarshal([]byte(KbStats()), &st)
	if st["docs"] < 1 {
		t.Fatalf("docs 统计不对: %v", st)
	}
}
