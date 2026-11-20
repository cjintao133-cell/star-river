package mobilebind

// 装配测试：情绪/记忆引擎真实接入——记录一轮后情绪变化、记忆可召回、三面板非 demo。

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssemblyMoodMemory(t *testing.T) {
	if r := InitKernel(filepath.Join(t.TempDir(), "asm.db"), base64.StdEncoding.EncodeToString(make([]byte, 32))); r != "OK" {
		t.Fatalf("init: %s", r)
	}
	before := map[string]float64{}
	_ = json.Unmarshal([]byte(MoodStateJSON()), &before)
	intimacy0 := before["intimacy"]

	observeTurn("我答应你，以后一定早点睡觉，别让我熬夜", "好，我记住了，用户")

	after := map[string]float64{}
	_ = json.Unmarshal([]byte(MoodStateJSON()), &after)
	if after["intimacy"] <= intimacy0 {
		t.Fatalf("亲密应增加: before=%v after=%v", intimacy0, after["intimacy"])
	}

	hits := MemoryHitsJSON(10)
	if !strings.Contains(hits, "早点睡觉") {
		t.Fatalf("记忆应可召回: %s", hits)
	}

	// 三面板应非 demo（有真实数据）
	var dp map[string]any
	_ = json.Unmarshal([]byte(DevPanels()), &dp)
	if dp["demo"] == true {
		t.Fatalf("有真实数据时 demo 应为 false")
	}

	// system 注入含语气 + 相关记忆
	sys := assembleSystem("你是示例角色", "睡觉")
	if !strings.Contains(sys, "语气") {
		t.Fatalf("system 应含情绪语气指令: %s", sys)
	}
	if !strings.Contains(sys, "早点睡觉") {
		t.Fatalf("system 应注入相关记忆: %s", sys)
	}
}

func TestAssemblyPersist(t *testing.T) {
	dir := t.TempDir()
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	db := filepath.Join(dir, "p.db")
	InitKernel(db, key)
	observeTurn("关键承诺：生日是五月一日", "记住了")
	m := MemoryHitsJSON(10)
	if !strings.Contains(m, "生日") {
		t.Fatalf("初次应召回: %s", m)
	}
	// 重开库（模拟重启）→ 记忆应恢复
	InitKernel(db, key)
	m2 := MemoryHitsJSON(10)
	if !strings.Contains(m2, "生日") {
		t.Fatalf("重启后记忆应持久化恢复: %s", m2)
	}
}
