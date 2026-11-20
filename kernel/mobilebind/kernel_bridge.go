package mobilebind

// 内核实例门面 + 角色/会话桥（T-新增）。薄门面：打开本地 SQLite，暴露角色与会话 CRUD。
// gomobile 类型约束：函数只用 string/int/bool/[]byte；结构走 JSON string；图片走 []byte。
// 红线：只 import 开源底座（store），不 import 闭源插件。

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"star-river/kernel/core/types"
	"star-river/kernel/store"
)

var (
	kbMu     sync.Mutex
	kbStore  *store.Store
	kbRepo   *store.Repo
	kbChars  *store.CharacterRepo
	kbMsgSeq int64
)

// InitKernel 打开本地数据库并初始化仓储。cipherKeyBase64 为 32 字节字段加密钥（base64）。
// 返回 "OK" 或 "ERR:<信息>"。重复调用会先关闭旧实例。
func InitKernel(dbPath, cipherKeyBase64 string) string {
	key, err := base64.StdEncoding.DecodeString(cipherKeyBase64)
	if err != nil || len(key) != 32 {
		return "ERR: cipher key must be 32 bytes base64"
	}
	kbMu.Lock()
	defer kbMu.Unlock()
	if kbStore != nil {
		_ = kbStore.Close()
		kbStore, kbRepo, kbChars = nil, nil, nil
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return "ERR:" + err.Error()
	}
	cipher, err := store.NewFieldCipher(key)
	if err != nil {
		_ = st.Close()
		return "ERR:" + err.Error()
	}
	repo, err := store.NewRepo(st, cipher)
	if err != nil {
		_ = st.Close()
		return "ERR:" + err.Error()
	}
	chars, err := store.NewCharacterRepo(st)
	if err != nil {
		_ = st.Close()
		return "ERR:" + err.Error()
	}
	kbStore, kbRepo, kbChars = st, repo, chars
	// 重开库后重置派生单例（tooldisc 索引 / MCP 管理器），避免指向已关闭的旧库。
	tdMu.Lock(); tdIdx = nil; tdMu.Unlock()
	mcpMu.Lock(); mcpMgr = nil; mcpMu.Unlock()
	resetAssembly()
	resetProactive()
	return "OK"
}

// CloseKernel 关闭数据库。
func CloseKernel() string {
	kbMu.Lock()
	defer kbMu.Unlock()
	if kbStore != nil {
		_ = kbStore.Close()
		kbStore, kbRepo, kbChars = nil, nil, nil
	}
	return "OK"
}

func kernelReady() bool { return kbStore != nil && kbRepo != nil && kbChars != nil }

// —— 角色桥 ——

type characterDTO struct {
	ID                      string   `json:"id"`
	Name                    string   `json:"name"`
	Description             string   `json:"description,omitempty"`
	Personality             string   `json:"personality,omitempty"`
	Scenario                string   `json:"scenario,omitempty"`
	FirstMes                string   `json:"first_mes,omitempty"`
	AlternateGreetings      []string `json:"alternate_greetings,omitempty"`
	MesExample              string   `json:"mes_example,omitempty"`
	SystemPrompt            string   `json:"system_prompt,omitempty"`
	PostHistoryInstructions string   `json:"post_history_instructions,omitempty"`
	CharacterBook           string   `json:"character_book,omitempty"`
	CreatedAt               int64    `json:"created_at,omitempty"`
	UpdatedAt               int64    `json:"updated_at,omitempty"`
}

func toDTO(c store.Character) characterDTO {
	d := characterDTO{
		ID: c.ID, Name: c.Name, Description: c.Description, Personality: c.Personality, Scenario: c.Scenario,
		FirstMes: c.FirstMes, AlternateGreetings: c.AlternateGreetings, MesExample: c.MesExample,
		SystemPrompt: c.SystemPrompt, PostHistoryInstructions: c.PostHistoryInstructions,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
	if len(c.CharacterBook) > 0 {
		d.CharacterBook = string(c.CharacterBook)
	}
	return d
}

// CreateCharacter 新建角色（入参 JSON characterDTO，id/name 必填）。返回 "OK"/"ERR:..."。
func CreateCharacter(jsonStr string) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	var d characterDTO
	if err := json.Unmarshal([]byte(jsonStr), &d); err != nil {
		return "ERR:" + err.Error()
	}
	c := store.Character{
		ID: d.ID, Name: d.Name, Description: d.Description, Personality: d.Personality, Scenario: d.Scenario,
		FirstMes: d.FirstMes, AlternateGreetings: d.AlternateGreetings, MesExample: d.MesExample,
		SystemPrompt: d.SystemPrompt, PostHistoryInstructions: d.PostHistoryInstructions,
	}
	if d.CharacterBook != "" {
		c.CharacterBook = json.RawMessage(d.CharacterBook)
	}
	if err := kbChars.Create(context.Background(), c); err != nil {
		return "ERR:" + err.Error()
	}
	return "OK"
}

// ListCharacters 列出角色（不含头像/人设图字节）。返回 JSON 数组 / "ERR:..."。
func ListCharacters() string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	list, err := kbChars.List(context.Background())
	if err != nil {
		return "ERR:" + err.Error()
	}
	out := make([]characterDTO, 0, len(list))
	for _, c := range list {
		out = append(out, toDTO(c))
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// GetCharacter 取角色文本字段（不含字节）。返回 JSON / "ERR:..."。
func GetCharacter(id string) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	c, err := kbChars.Get(context.Background(), id)
	if err != nil {
		return "ERR:" + err.Error()
	}
	b, _ := json.Marshal(toDTO(c))
	return string(b)
}

// UpdateCharacter 更新角色文本字段。返回 "OK"/"ERR:..."。
func UpdateCharacter(jsonStr string) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	var d characterDTO
	if err := json.Unmarshal([]byte(jsonStr), &d); err != nil {
		return "ERR:" + err.Error()
	}
	c := store.Character{
		ID: d.ID, Name: d.Name, Description: d.Description, Personality: d.Personality, Scenario: d.Scenario,
		FirstMes: d.FirstMes, AlternateGreetings: d.AlternateGreetings, MesExample: d.MesExample,
		SystemPrompt: d.SystemPrompt, PostHistoryInstructions: d.PostHistoryInstructions,
	}
	if d.CharacterBook != "" {
		c.CharacterBook = json.RawMessage(d.CharacterBook)
	}
	if err := kbChars.Update(context.Background(), c); err != nil {
		return "ERR:" + err.Error()
	}
	return "OK"
}

// SetCharacterAvatar 设置头像字节。返回 "OK"/"ERR:..."。
func SetCharacterAvatar(id string, data []byte) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	if err := kbChars.SetAvatar(context.Background(), id, data); err != nil {
		return "ERR:" + err.Error()
	}
	return "OK"
}

// SetCharacterPersona 设置人设图字节。返回 "OK"/"ERR:..."。
func SetCharacterPersona(id string, data []byte) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	if err := kbChars.SetPersonaImage(context.Background(), id, data); err != nil {
		return "ERR:" + err.Error()
	}
	return "OK"
}

// DeleteCharacter 删除角色。返回 "OK"/"ERR:..."。
func DeleteCharacter(id string) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	if err := kbChars.Delete(context.Background(), id); err != nil {
		return "ERR:" + err.Error()
	}
	return "OK"
}

// —— 会话 / 消息桥 ——

// CreateSession 为某角色新建会话，返回 JSON {"session_id":...} / "ERR:...".
func CreateSession(characterID, title, model string) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	sid := genSessionID()
	now := time.Now().UnixMilli()
	if err := kbRepo.CreateSession(context.Background(), store.Session{ID: sid, Title: title, Model: model, CreatedAt: now, UpdatedAt: now}); err != nil {
		return "ERR:" + err.Error()
	}
	if characterID != "" {
		_, _ = kbStore.Exec(context.Background(), `UPDATE sessions SET character_id=? WHERE id=?`, characterID, sid)
	}
	b, _ := json.Marshal(map[string]string{"session_id": sid})
	return string(b)
}

type sessionDTO struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Model     string `json:"model"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// ListSessions 列出会话（按 updated 降序）。返回 JSON 数组 / "ERR:..."。
func ListSessions() string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	list, err := kbRepo.ListSessions(context.Background(), 200, 0)
	if err != nil {
		return "ERR:" + err.Error()
	}
	out := make([]sessionDTO, 0, len(list))
	for _, s := range list {
		out = append(out, sessionDTO{ID: s.ID, Title: s.Title, Model: s.Model, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt})
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// AppendMessage 追加一条消息（role: user/assistant）。返回 "OK"/"ERR:..."。
func AppendMessage(sessionID, role, text string) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	if !types.Role(role).Valid() {
		return "ERR:invalid role"
	}
	mid := genMessageID()
	msg := types.Message{
		ID: mid, SessionID: sessionID, Role: types.Role(role),
		Blocks:    []types.ContentBlock{{Type: types.BlockText, Text: text}},
		CreatedAt: time.Now().UnixMilli(),
	}
	if _, err := kbRepo.AppendMessage(context.Background(), msg); err != nil {
		return "ERR:" + err.Error()
	}
	return "OK"
}

type messageDTO struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Text      string `json:"text"`
	CreatedAt int64  `json:"created_at"`
}

// ListMessages 列出会话消息（解密后，时间正序）。返回 JSON 数组 / "ERR:..."。
func ListMessages(sessionID string, limit int) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	if limit <= 0 {
		limit = 50
	}
	page, err := kbRepo.ListMessages(context.Background(), sessionID, limit, 0)
	if err != nil {
		return "ERR:" + err.Error()
	}
	out := make([]messageDTO, 0, len(page.Records))
	for _, rec := range page.Records {
		var b string
		for _, blk := range rec.Message.Blocks {
			if blk.Type == types.BlockText {
				b += blk.Text
			}
		}
		out = append(out, messageDTO{ID: rec.Message.ID, Role: string(rec.Message.Role), Text: b, CreatedAt: rec.Message.CreatedAt})
	}
	// ListMessages 返回降序（repo 按 seq desc），翻成时间正序更友好。
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// DeleteSession 删除会话（级联删除其消息）。返回 "OK"/"ERR:..."。
func DeleteSession(sessionID string) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	if err := kbRepo.DeleteSession(context.Background(), sessionID); err != nil {
		return "ERR:" + err.Error()
	}
	return "OK"
}

func genSessionID() string {
	return fmt.Sprintf("s-%d-%d", time.Now().UnixNano(), atomic.AddInt64(&kbMsgSeq, 1))
}

func genMessageID() string {
	return fmt.Sprintf("m-%d-%d", time.Now().UnixNano(), atomic.AddInt64(&kbMsgSeq, 1))
}
