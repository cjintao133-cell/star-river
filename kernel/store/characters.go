package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// 角色仓储错误。
var (
	ErrCharacterNotFound = errors.New("store: character not found")
	ErrInvalidCharacter  = errors.New("store: invalid character (id/name required)")
)

// Character 一个角色（对齐角色卡字段，见 docs/ui-spec.md §3）。
// AlternateGreetings 序列化为 JSON 数组；CharacterBook 原样存 JSON 文本。
type Character struct {
	ID                      string
	Name                    string
	Description             string
	Personality             string
	Scenario                string
	FirstMes                string
	AlternateGreetings      []string
	MesExample              string
	SystemPrompt            string
	PostHistoryInstructions string
	CharacterBook           json.RawMessage // 世界书原始 JSON（可空）
	Avatar                  []byte          // 头像图片字节（可空，List 不返回）
	PersonaImage            []byte          // 人设图立绘字节（可空，List 不返回）
	CreatedAt               int64
	UpdatedAt               int64
}

// CharacterRepo 角色仓储。并发安全由底层 store 保证（写串行/读并发）。
type CharacterRepo struct {
	st *Store
}

// NewCharacterRepo 建仓储。
func NewCharacterRepo(st *Store) (*CharacterRepo, error) {
	if st == nil {
		return nil, errors.New("store: nil store")
	}
	return &CharacterRepo{st: st}, nil
}

// Create 新建角色（id/name 必填；created/updated 由本方法填当前时刻）。
func (r *CharacterRepo) Create(ctx context.Context, c Character) error {
	if c.ID == "" || c.Name == "" {
		return ErrInvalidCharacter
	}
	alt := mustAltJSON(c.AlternateGreetings)
	book := bookOrDefault(c.CharacterBook)
	now := nowMilli()
	_, err := r.st.Exec(ctx,
		`INSERT INTO characters(id,name,description,personality,scenario,first_mes,alternate_greetings,
            mes_example,system_prompt,post_history_instructions,character_book,avatar,persona_image,created_at,updated_at)
         VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.Name, c.Description, c.Personality, c.Scenario, c.FirstMes, alt,
		c.MesExample, c.SystemPrompt, c.PostHistoryInstructions, string(book),
		c.Avatar, c.PersonaImage, now, now)
	return err
}

// Get 取角色（含头像/人设图字节）。
func (r *CharacterRepo) Get(ctx context.Context, id string) (Character, error) {
	row := r.st.DB().QueryRowContext(ctx,
		`SELECT id,name,description,personality,scenario,first_mes,alternate_greetings,
            mes_example,system_prompt,post_history_instructions,character_book,avatar,persona_image,created_at,updated_at
         FROM characters WHERE id=?`, id)
	c, err := scanCharacter(row.Scan, true)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Character{}, ErrCharacterNotFound
		}
		return Character{}, err
	}
	return c, nil
}

// List 列出全部角色（按 updated_at 降序；**不含** avatar/persona_image 大字段）。
func (r *CharacterRepo) List(ctx context.Context) ([]Character, error) {
	rows, err := r.st.DB().QueryContext(ctx,
		`SELECT id,name,description,personality,scenario,first_mes,alternate_greetings,
            mes_example,system_prompt,post_history_instructions,character_book,created_at,updated_at
         FROM characters ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Character{}
	for rows.Next() {
		var c Character
		var alt, book string
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.Personality, &c.Scenario, &c.FirstMes,
			&alt, &c.MesExample, &c.SystemPrompt, &c.PostHistoryInstructions, &book, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.AlternateGreetings = parseAlt(alt)
		c.CharacterBook = rawBook(book)
		out = append(out, c)
	}
	return out, rows.Err()
}

// Update 更新角色文本字段（不动 avatar/persona_image，用 SetAvatar/SetPersonaImage 单独设）。
// 更新 updated_at。id 必须已存在。
func (r *CharacterRepo) Update(ctx context.Context, c Character) error {
	if c.ID == "" || c.Name == "" {
		return ErrInvalidCharacter
	}
	alt := mustAltJSON(c.AlternateGreetings)
	book := bookOrDefault(c.CharacterBook)
	res, err := r.st.Exec(ctx,
		`UPDATE characters SET name=?,description=?,personality=?,scenario=?,first_mes=?,alternate_greetings=?,
            mes_example=?,system_prompt=?,post_history_instructions=?,character_book=?,updated_at=?
         WHERE id=?`,
		c.Name, c.Description, c.Personality, c.Scenario, c.FirstMes, alt,
		c.MesExample, c.SystemPrompt, c.PostHistoryInstructions, string(book), nowMilli(), c.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCharacterNotFound
	}
	return nil
}

// SetAvatar 设置头像图片字节。
func (r *CharacterRepo) SetAvatar(ctx context.Context, id string, data []byte) error {
	return r.updateBlob(ctx, id, "avatar", data)
}

// SetPersonaImage 设置人设图立绘字节。
func (r *CharacterRepo) SetPersonaImage(ctx context.Context, id string, data []byte) error {
	return r.updateBlob(ctx, id, "persona_image", data)
}

// updateBlob 更新某 BLOB 列（列名来自受控常量，无注入面）。
func (r *CharacterRepo) updateBlob(ctx context.Context, id, col string, data []byte) error {
	res, err := r.st.Exec(ctx, `UPDATE characters SET `+col+`=?, updated_at=? WHERE id=?`, data, nowMilli(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCharacterNotFound
	}
	return nil
}

// Delete 删除角色（其会话不级联；会话-角色关联由上层决定）。
func (r *CharacterRepo) Delete(ctx context.Context, id string) error {
	res, err := r.st.Exec(ctx, `DELETE FROM characters WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCharacterNotFound
	}
	return nil
}

// —— 内部 ——

func scanCharacter(scan func(...any) error, withBlob bool) (Character, error) {
	var c Character
	var alt, book string
	if withBlob {
		if err := scan(&c.ID, &c.Name, &c.Description, &c.Personality, &c.Scenario, &c.FirstMes, &alt,
			&c.MesExample, &c.SystemPrompt, &c.PostHistoryInstructions, &book, &c.Avatar, &c.PersonaImage,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			return Character{}, err
		}
	}
	c.AlternateGreetings = parseAlt(alt)
	c.CharacterBook = rawBook(book)
	return c, nil
}

func mustAltJSON(alt []string) string {
	if len(alt) == 0 {
		return "[]"
	}
	b, err := json.Marshal(alt)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func parseAlt(s string) []string {
	var out []string
	if s == "" {
		return nil
	}
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

func bookOrDefault(b json.RawMessage) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("{}")
	}
	return b
}

func rawBook(s string) json.RawMessage {
	if s == "" {
		return nil
	}
	return json.RawMessage(s)
}
