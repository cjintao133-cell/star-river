package mobilebind

// 角色卡解析桥（cards）：用 kernel/cards 统一解析 V1/V2/V3 JSON、PNG(tEXt)、charx(zip)。
// 取代 Kotlin 侧只认 JSON V2 的简易解析（去重），并支持 PNG/charx 更全格式。

import (
	"encoding/json"

	"github.com/cjintao133-cell/star-river/kernel/cards"
)

// ParseCharacterCard 解析角色卡字节，返回 CharacterDTO 形状的 JSON（含世界书）。失败 "ERR:..."。
func ParseCharacterCard(data []byte) string {
	if len(data) == 0 {
		return "ERR:empty card"
	}
	c, err := cards.Parse(data)
	if err != nil {
		return "ERR:" + err.Error()
	}
	d := characterDTO{
		Name:                    c.Name,
		Description:             c.Description,
		Personality:             c.Personality,
		Scenario:                c.Scenario,
		FirstMes:                c.FirstMessage,
		AlternateGreetings:      c.AlternateGreetings,
		MesExample:              c.MessageExample,
		SystemPrompt:            c.SystemPrompt,
		PostHistoryInstructions: c.PostHistoryInstructions,
	}
	if len(c.Book) > 0 {
		if b, err := json.Marshal(c.Book); err == nil {
			d.CharacterBook = string(b)
		}
	}
	b, err := json.Marshal(d)
	if err != nil {
		return "ERR:" + err.Error()
	}
	return string(b)
}
