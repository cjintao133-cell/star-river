// Package cards 角色卡导入（T-M3-04，伴侣插件能力，底座不 import）。
//
// 支持四载体（A1 已批）：V2/V3 JSON、chara PNG（tEXt/iTXt base64）、charx（zip）。
// 世界书条目转「带触发词的记忆条目」（A2：constant 常驻 / selective 触发词匹配 +
// 递归激活深度上限 + 循环检测），不进系统提示词。alternate_greetings 全部入库（A4）。
// 纯标准库（encoding/json / archive/zip / encoding/base64），零新依赖。
package cards

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

// 解析错误。
var (
	ErrEmpty     = errors.New("cards: empty input")
	ErrUnknown   = errors.New("cards: unrecognized container (not JSON/PNG/charx)")
	ErrNoCard    = errors.New("cards: no character card found")
	ErrBadFormat = errors.New("cards: malformed card")
)

// BookEntry 世界书条目。
type BookEntry struct {
	Keys           []string // 触发词
	Content        string
	Enabled        bool
	Constant       bool // 常驻（constant 高优先级常驻，受预算分配器管）
	InsertionOrder int
}

// Card 解析后的角色卡。
type Card struct {
	Spec                    string // chara_card_v2 / chara_card_v3 / ""(v1)
	SpecVersion             string
	Name                    string
	Description             string
	Personality             string
	Scenario                string
	FirstMessage            string
	MessageExample          string
	SystemPrompt            string
	PostHistoryInstructions string
	AlternateGreetings      []string // A4：全部入库（v1 UI 只用第一条）
	Book                    []BookEntry
}

// —— 线格式（V1 扁平 / V2V3 嵌套 data） ——

type wireEntry struct {
	Keys           []string `json:"keys"`
	Content        string   `json:"content"`
	Enabled        *bool    `json:"enabled"`
	Constant       bool     `json:"constant"`
	InsertionOrder int      `json:"insertion_order"`
}

type wireBook struct {
	Entries []wireEntry `json:"entries"`
}

type wireData struct {
	Name                    string    `json:"name"`
	Description             string    `json:"description"`
	Personality             string    `json:"personality"`
	Scenario                string    `json:"scenario"`
	FirstMes                string    `json:"first_mes"`
	MesExample              string    `json:"mes_example"`
	SystemPrompt            string    `json:"system_prompt"`
	PostHistoryInstructions string    `json:"post_history_instructions"`
	AlternateGreetings      []string  `json:"alternate_greetings"`
	CharacterBook           *wireBook `json:"character_book"`
}

type wireCard struct {
	Spec        string `json:"spec"`
	SpecVersion string `json:"spec_version"`
	// V1 扁平字段：
	Name        string `json:"name"`
	Description string `json:"description"`
	Personality string `json:"personality"`
	Scenario    string `json:"scenario"`
	FirstMes    string `json:"first_mes"`
	MesExample  string `json:"mes_example"`
	// V2/V3 嵌套：
	Data *wireData `json:"data"`
}

// ParseJSON 解析 V2/V3（data 嵌套）或 V1（扁平）JSON。
func ParseJSON(b []byte) (*Card, error) {
	if len(b) == 0 {
		return nil, ErrEmpty
	}
	var w wireCard
	if err := json.Unmarshal(b, &w); err != nil {
		return nil, ErrBadFormat
	}
	c := &Card{Spec: w.Spec, SpecVersion: w.SpecVersion}
	if w.Data != nil {
		// V2/V3。
		c.Name = w.Data.Name
		c.Description = w.Data.Description
		c.Personality = w.Data.Personality
		c.Scenario = w.Data.Scenario
		c.FirstMessage = w.Data.FirstMes
		c.MessageExample = w.Data.MesExample
		c.SystemPrompt = w.Data.SystemPrompt
		c.PostHistoryInstructions = w.Data.PostHistoryInstructions
		c.AlternateGreetings = append([]string(nil), w.Data.AlternateGreetings...)
		if w.Data.CharacterBook != nil {
			c.Book = convertEntries(w.Data.CharacterBook.Entries)
		}
	} else {
		// V1 扁平。character_book 也可能扁平挂在顶层（部分导出），尽力。
		c.Name = w.Name
		c.Description = w.Description
		c.Personality = w.Personality
		c.Scenario = w.Scenario
		c.FirstMessage = w.FirstMes
		c.MessageExample = w.MesExample
	}
	if c.Name == "" && c.Description == "" && c.FirstMessage == "" {
		return nil, ErrNoCard
	}
	return c, nil
}

// convertEntries 世界书条目 → BookEntry（enabled 默认 true）。
func convertEntries(es []wireEntry) []BookEntry {
	out := make([]BookEntry, 0, len(es))
	for _, e := range es {
		enabled := true
		if e.Enabled != nil {
			enabled = *e.Enabled
		}
		out = append(out, BookEntry{
			Keys:           append([]string(nil), e.Keys...),
			Content:        e.Content,
			Enabled:        enabled,
			Constant:       e.Constant,
			InsertionOrder: e.InsertionOrder,
		})
	}
	return out
}

// decodeB64 容错解码（标准/URL、有无 padding）。
func decodeB64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if d, err := base64.StdEncoding.DecodeString(s); err == nil {
		return d, nil
	}
	if d, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return d, nil
	}
	if d, err := base64.URLEncoding.DecodeString(s); err == nil {
		return d, nil
	}
	return base64.RawURLEncoding.DecodeString(s)
}
