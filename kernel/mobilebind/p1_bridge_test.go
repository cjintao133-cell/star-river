package mobilebind

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCriticBridgeHeuristic(t *testing.T) {
	initTestKernel(t)
	var v map[string]any
	// 无 base/model -> 仅启发式
	_ = json.Unmarshal([]byte(CriticReview("完整的正常回答内容。", "问题", "", "", "")), &v)
	if v["pass"] != true {
		t.Fatalf("正常回答应通过: %+v", v)
	}
	_ = json.Unmarshal([]byte(CriticReview("  ", "", "", "", "")), &v)
	if v["pass"] != false {
		t.Fatalf("空答应不通过: %+v", v)
	}
}

func TestSkillsBridge(t *testing.T) {
	initTestKernel(t)
	dir := t.TempDir()
	d := filepath.Join(dir, "translate")
	_ = os.MkdirAll(d, 0755)
	_ = os.WriteFile(filepath.Join(d, "SKILL.md"), []byte("---\nname: translate\ndescription: 中英互译\n---\n把文本翻译成英文"), 0644)

	out := SkillsLoadDir(dir)
	if !strings.Contains(out, `"loaded":1`) {
		t.Fatalf("加载应是 1: %s", out)
	}
	if r := SkillsAdd("summarize", "总结长文", "提炼要点"); r != "OK" {
		t.Fatalf("add: %s", r)
	}
	var list []map[string]any
	_ = json.Unmarshal([]byte(SkillsList()), &list)
	if len(list) != 2 {
		t.Fatalf("应有 2 技能: %+v", list)
	}
	var hits []map[string]any
	_ = json.Unmarshal([]byte(SkillsMatch("帮我翻译英文", 3)), &hits)
	if len(hits) == 0 || hits[0]["name"] != "translate" {
		t.Fatalf("应命中 translate: %+v", hits)
	}
	var full map[string]any
	_ = json.Unmarshal([]byte(SkillsGet("translate")), &full)
	if !strings.Contains(full["body"].(string), "翻译成英文") {
		t.Fatalf("应取到正文: %+v", full)
	}
	if r := SkillsGet("nope"); !strings.HasPrefix(r, "ERR") {
		t.Fatalf("不存在应 ERR: %s", r)
	}
}

func TestMcpServerBridge(t *testing.T) {
	initTestKernel(t)
	_ = CreateCharacter(`{"id":"c1","name":"n"}`)
	// manifest
	var man map[string]any
	_ = json.Unmarshal([]byte(McpServerManifest()), &man)
	if man["name"] != "star-river" {
		t.Fatalf("manifest 名字不对: %+v", man)
	}
	tools := man["tools"].([]any)
	if len(tools) < 5 {
		t.Fatalf("工具太少: %d", len(tools))
	}
	// initialize
	resp := McpServerHandle(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if !strings.Contains(resp, "protocolVersion") {
		t.Fatalf("initialize 响应不对: %s", resp)
	}
	// tools/list
	resp = McpServerHandle(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if !strings.Contains(resp, "memory_search") {
		t.Fatalf("tools/list 应含 memory_search: %s", resp)
	}
	// tools/call characters_list
	resp = McpServerHandle(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"characters_list","arguments":{}}}`)
	if !strings.Contains(resp, "result") || !strings.Contains(resp, "isError") {
		t.Fatalf("tools/call 响应不对: %s", resp)
	}
	// 通知无响应
	if McpServerHandle(`{"jsonrpc":"2.0","method":"notifications/initialized"}`) != "" {
		t.Fatalf("通知应无响应")
	}
	// 未知工具 -> isError true（在 content 结果里）
	resp = McpServerHandle(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"bogus","arguments":{}}}`)
	if !strings.Contains(resp, "unknown tool") {
		t.Fatalf("未知工具应报错内容: %s", resp)
	}
}
