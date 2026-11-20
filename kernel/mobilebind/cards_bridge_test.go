package mobilebind

// 角色卡解析桥：V2 JSON 解析出字段 + 世界书；坏卡报错。

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseCharacterCardV2(t *testing.T) {
	card := `{"spec":"chara_card_v2","data":{"name":"示例角色","description":"示例角色","personality":"温和","first_mes":"哼",
	  "alternate_greetings":["你好","喂"],"mes_example":"<START>","system_prompt":"你是示例角色",
	  "character_book":{"entries":[{"keys":["生日"],"content":"五月一日","enabled":true}]}}}`
	got := ParseCharacterCard([]byte(card))
	if strings.HasPrefix(got, "ERR:") {
		t.Fatalf("解析失败: %s", got)
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(got), &m)
	if m["name"] != "示例角色" || m["personality"] != "温和" {
		t.Fatalf("字段不符: %v", m)
	}
	if cb, ok := m["character_book"].(string); !ok || !strings.Contains(cb, "生日") {
		t.Fatalf("世界书应解析: %v", m["character_book"])
	}
}

func TestParseCharacterCardBad(t *testing.T) {
	if r := ParseCharacterCard(nil); !strings.HasPrefix(r, "ERR:") {
		t.Fatalf("空应报错")
	}
	if r := ParseCharacterCard([]byte("not a card")); !strings.HasPrefix(r, "ERR:") {
		t.Fatalf("坏卡应报错")
	}
}
