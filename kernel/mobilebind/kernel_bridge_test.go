package mobilebind

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func initTestKernel(t *testing.T) string {
	t.Helper()
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	dbPath := filepath.Join(t.TempDir(), "kernel.db")
	if r := InitKernel(dbPath, key); r != "OK" {
		t.Fatalf("InitKernel: %s", r)
	}
	t.Cleanup(func() { CloseKernel() })
	return key
}

func TestInitKernelErrors(t *testing.T) {
	if !strings.HasPrefix(InitKernel("/tmp/x.db", "short"), "ERR:") {
		t.Error("短 key 应 ERR")
	}
	if !strings.HasPrefix(InitKernel("/tmp/x.db", "!!!notb64"), "ERR:") {
		t.Error("坏 base64 应 ERR")
	}
}

func TestKernelNotInit(t *testing.T) {
	CloseKernel()
	if !strings.HasPrefix(ListCharacters(), "ERR:") {
		t.Error("未初始化 ListCharacters 应 ERR")
	}
	if !strings.HasPrefix(CreateSession("c", "t", "m"), "ERR:") {
		t.Error("未初始化 CreateSession 应 ERR")
	}
}

func TestCharacterBridge(t *testing.T) {
	initTestKernel(t)
	// create
	cj := `{"id":"c1","name":"示例角色","personality":"温和","alternate_greetings":["a","b"]}`
	if r := CreateCharacter(cj); r != "OK" {
		t.Fatalf("create: %s", r)
	}
	// list
	var list []map[string]any
	if err := json.Unmarshal([]byte(ListCharacters()), &list); err != nil || len(list) != 1 {
		t.Fatalf("list: %v %s", err, ListCharacters())
	}
	if list[0]["name"] != "示例角色" {
		t.Errorf("name 不对: %v", list[0])
	}
	// get
	var got map[string]any
	_ = json.Unmarshal([]byte(GetCharacter("c1")), &got)
	if got["personality"] != "温和" {
		t.Errorf("get 不对: %v", got)
	}
	// avatar / persona
	if r := SetCharacterAvatar("c1", []byte("AV")); r != "OK" {
		t.Errorf("avatar: %s", r)
	}
	if r := SetCharacterPersona("c1", []byte("PE")); r != "OK" {
		t.Errorf("persona: %s", r)
	}
	// update
	if r := UpdateCharacter(`{"id":"c1","name":"示例角色改"}`); r != "OK" {
		t.Errorf("update: %s", r)
	}
	_ = json.Unmarshal([]byte(GetCharacter("c1")), &got)
	if got["name"] != "示例角色改" {
		t.Errorf("update 未生效: %v", got)
	}
	// delete
	if r := DeleteCharacter("c1"); r != "OK" {
		t.Errorf("delete: %s", r)
	}
	if !strings.HasPrefix(GetCharacter("c1"), "ERR:") {
		t.Error("删除后 get 应 ERR")
	}
}

func TestSessionMessageBridge(t *testing.T) {
	initTestKernel(t)
	_ = CreateCharacter(`{"id":"c1","name":"n"}`)
	// create session
	var cs map[string]string
	if err := json.Unmarshal([]byte(CreateSession("c1", "会话", "m1")), &cs); err != nil {
		t.Fatal(err)
	}
	sid := cs["session_id"]
	if sid == "" {
		t.Fatal("缺 session_id")
	}
	// append messages
	if r := AppendMessage(sid, "user", "你好"); r != "OK" {
		t.Fatalf("append user: %s", r)
	}
	if r := AppendMessage(sid, "assistant", "在的"); r != "OK" {
		t.Fatalf("append assistant: %s", r)
	}
	// list messages（解密后正序）
	var msgs []map[string]any
	if err := json.Unmarshal([]byte(ListMessages(sid, 50)), &msgs); err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[0]["text"] != "你好" || msgs[1]["text"] != "在的" {
		t.Fatalf("消息往返/解密不对: %+v", msgs)
	}
	// list sessions
	var ss []map[string]any
	_ = json.Unmarshal([]byte(ListSessions()), &ss)
	if len(ss) != 1 || ss[0]["title"] != "会话" {
		t.Errorf("会话列表不对: %v", ss)
	}
	// append 非法角色
	if !strings.HasPrefix(AppendMessage(sid, "bogus", "x"), "ERR:") {
		t.Error("非法角色应 ERR")
	}
	// delete session（级联删消息）
	if r := DeleteSession(sid); r != "OK" {
		t.Fatalf("delete session: %s", r)
	}
	var msgs2 []map[string]any
	_ = json.Unmarshal([]byte(ListMessages(sid, 50)), &msgs2)
	if len(msgs2) != 0 {
		t.Errorf("删会话后消息应清空: %v", msgs2)
	}
}
