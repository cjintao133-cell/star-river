package mobilebind

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// P0-4: 多模态消息块落库 + 取回。
func TestMessageMediaBridge(t *testing.T) {
	initTestKernel(t)
	if r := CreateCharacter(`{"id":"c1","name":"n"}`); r != "OK" {
		t.Fatalf("create char: %s", r)
	}
	var cs map[string]string
	if err := json.Unmarshal([]byte(CreateSession("c1", "会话", "m1")), &cs); err != nil {
		t.Fatal(err)
	}
	sid := cs["session_id"]
	if sid == "" {
		t.Fatal("缺 session_id")
	}

	png := []byte{0x89, 'P', 'N', 'G', 1, 2, 3, 4, 5}
	audio := []byte{0, 1, 2, 3, 0xAA, 0xBB}
	blocks := []map[string]any{
		{"type": "text", "text": "看图"},
		{"type": "image", "mime_type": "image/png", "data_b64": base64.StdEncoding.EncodeToString(png)},
		{"type": "audio", "mime_type": "audio/mp4", "data_b64": base64.StdEncoding.EncodeToString(audio)},
	}
	js, _ := json.Marshal(blocks)
	if r := AppendMessageBlocks(sid, "user", string(js)); r != "OK" {
		t.Fatalf("AppendMessageBlocks: %s", r)
	}

	var msgs []struct {
		ID     string      `json:"id"`
		Role   string      `json:"role"`
		Text   string      `json:"text"`
		Blocks []blockMeta `json:"blocks"`
	}
	if err := json.Unmarshal([]byte(ListMessages(sid, 50)), &msgs); err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expect 1 msg, got %d", len(msgs))
	}
	m := msgs[0]
	if m.Text != "看图" {
		t.Errorf("text 聚合不对: %q", m.Text)
	}
	if len(m.Blocks) != 3 {
		t.Fatalf("expect 3 blocks, got %d", len(m.Blocks))
	}
	if m.Blocks[1].Type != "image" || !m.Blocks[1].HasData || m.Blocks[1].Size != len(png) {
		t.Errorf("image 块元信息不对: %+v", m.Blocks[1])
	}
	if m.Blocks[2].Type != "audio" || m.Blocks[2].Size != len(audio) {
		t.Errorf("audio 块元信息不对: %+v", m.Blocks[2])
	}
	if m.Blocks[0].HasData {
		t.Errorf("text 块不应有二进制")
	}

	// 取回图片/音频字节与写入一致
	if got := GetMessageMedia(sid, m.ID, 1); got != base64.StdEncoding.EncodeToString(png) {
		t.Errorf("取回图片不一致: %q", got)
	}
	if got := GetMessageMedia(sid, m.ID, 2); got != base64.StdEncoding.EncodeToString(audio) {
		t.Errorf("取回音频不一致: %q", got)
	}
	// 文本块无二进制
	if r := GetMessageMedia(sid, m.ID, 0); !strings.HasPrefix(r, "ERR:") {
		t.Errorf("文本块取媒体应 ERR, got %q", r)
	}
	// 越界
	if r := GetMessageMedia(sid, m.ID, 9); !strings.HasPrefix(r, "ERR:") {
		t.Errorf("越界应 ERR, got %q", r)
	}
	// 不存在的消息
	if r := GetMessageMedia(sid, "nope", 0); !strings.HasPrefix(r, "ERR:") {
		t.Errorf("不存在消息应 ERR, got %q", r)
	}
}

func TestAppendMessageBlocksValidation(t *testing.T) {
	initTestKernel(t)
	_ = CreateCharacter(`{"id":"c1","name":"n"}`)
	var cs map[string]string
	_ = json.Unmarshal([]byte(CreateSession("c1", "s", "m1")), &cs)
	sid := cs["session_id"]

	cases := []struct {
		name, role, js string
	}{
		{"非法角色", "bogus", `[{"type":"text","text":"x"}]`},
		{"坏 json", "user", `{not json`},
		{"空块", "user", `[]`},
		{"非法类型", "user", `[{"type":"bogus","text":"x"}]`},
		{"坏 base64", "user", `[{"type":"image","data_b64":"!!!"}]`},
	}
	for _, c := range cases {
		if r := AppendMessageBlocks(sid, c.role, c.js); !strings.HasPrefix(r, "ERR:") {
			t.Errorf("%s 应 ERR, got %q", c.name, r)
		}
	}
	// 超过块数上限
	many := make([]map[string]any, 0, maxBlocksPerMessage+1)
	for i := 0; i < maxBlocksPerMessage+1; i++ {
		many = append(many, map[string]any{"type": "text", "text": "x"})
	}
	mj, _ := json.Marshal(many)
	if r := AppendMessageBlocks(sid, "user", string(mj)); !strings.HasPrefix(r, "ERR:") {
		t.Errorf("超块数应 ERR, got %q", r)
	}
}
