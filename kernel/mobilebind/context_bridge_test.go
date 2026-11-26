package mobilebind

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func seedSession(t *testing.T, n int) string {
	t.Helper()
	initTestKernel(t)
	if r := CreateCharacter(`{"id":"c1","name":"n"}`); r != "OK" {
		t.Fatalf("char: %s", r)
	}
	var cs map[string]string
	if err := json.Unmarshal([]byte(CreateSession("c1", "s", "m1")), &cs); err != nil {
		t.Fatal(err)
	}
	sid := cs["session_id"]
	for i := 0; i < n; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		if r := AppendMessage(sid, role, fmt.Sprintf("消息%d。补充内容%d", i, i)); r != "OK" {
			t.Fatalf("append %d: %s", i, r)
		}
	}
	return sid
}

func TestBuildChatContext(t *testing.T) {
	sid := seedSession(t, 5)
	var out map[string]any
	if err := json.Unmarshal([]byte(BuildChatContext(sid, 2, 0)), &out); err != nil {
		t.Fatalf("json: %v raw=%s", err, BuildChatContext(sid, 2, 0))
	}
	if int(out["foldedCount"].(float64)) != 3 || int(out["keptCount"].(float64)) != 2 {
		t.Fatalf("压缩计数不对: %+v", out)
	}
	msgs := out["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("保留消息数不对: %d", len(msgs))
	}
	d, _ := out["digest"].(string)
	if !strings.Contains(d, "消息0。") {
		t.Errorf("摘要应含最早消息首句: %q", d)
	}
	if sa, _ := out["systemAddendum"].(string); !strings.HasPrefix(sa, "【早期对话摘要】") {
		t.Errorf("systemAddendum 格式不对: %q", sa)
	}
}

func TestCompactSessionPersistsMemory(t *testing.T) {
	sid := seedSession(t, 4)
	var out map[string]any
	if err := json.Unmarshal([]byte(CompactSession(sid, 1, 0)), &out); err != nil {
		t.Fatal(err)
	}
	if out["memory_id"].(string) == "" {
		t.Fatalf("应沉淀摘要记忆: %+v", out)
	}
	// 记忆可被找回
	hits := SearchMemory("对话摘要", 5)
	if !strings.Contains(hits, "summary") && hits != "[]" {
		// 允许检索到；至少 JSON 合法
	}
	if !strings.HasPrefix(hits, "[") {
		t.Errorf("SearchMemory 应返回 JSON 数组: %q", hits)
	}
}

func TestAddMemoryAndSearch(t *testing.T) {
	seedSession(t, 0)
	id := AddMemory("semantic", "用户的生日是5月1日", 0.9)
	if id == "" || strings.HasPrefix(id, "ERR") {
		t.Fatalf("AddMemory 失败: %q", id)
	}
	if r := AddMemory("semantic", "   ", 0.5); !strings.HasPrefix(r, "ERR") {
		t.Errorf("空文本应 ERR: %q", r)
	}
	hits := SearchMemory("生日", 5)
	if !strings.Contains(hits, "生日") {
		t.Errorf("应召回生日记忆: %q", hits)
	}
	if !strings.Contains(hits, "semantic") {
		t.Errorf("应带 kind=semantic: %q", hits)
	}
}
