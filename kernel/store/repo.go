package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"star-river/kernel/core/types"
)

// 会话/消息仓储错误。
var (
	ErrNoCipher        = errors.New("store: repo requires a field cipher")
	ErrInvalidSession  = errors.New("store: invalid session")
	ErrInvalidMessage  = errors.New("store: invalid message")
	ErrSessionNotFound = errors.New("store: session not found")
	ErrMessageNotFound = errors.New("store: message not found")
)

// Session 会话元数据。时间戳 UnixMilli。
type Session struct {
	ID        string
	Title     string
	Model     string
	CreatedAt int64
	UpdatedAt int64
}

// MessageRecord 一条消息 + 其在库中的稳定序号 seq（分页游标用）。
type MessageRecord struct {
	Seq     int64
	Message types.Message
}

// MessagePage 消息分页结果。NextBeforeSeq 作为下一页入参；HasMore 为 false 表示到底。
type MessagePage struct {
	Records       []MessageRecord
	NextBeforeSeq int64
	HasMore       bool
}

// Repo 会话/消息仓储。content（blocks）经 FieldCipher 加密后落 content_cipher 列。
type Repo struct {
	st     *Store
	cipher *FieldCipher
}

// NewRepo 构造仓储。cipher 必须非空（消息内容强制加密，红线 4）。
func NewRepo(st *Store, cipher *FieldCipher) (*Repo, error) {
	if st == nil {
		return nil, errors.New("store: nil store")
	}
	if cipher == nil {
		return nil, ErrNoCipher
	}
	return &Repo{st: st, cipher: cipher}, nil
}

// nowMilli 时间戳（测试可经 Store 注入？v1 用真实时钟；假时钟贯穿留 T-M3-06）。
func nowMilli() int64 { return time.Now().UnixMilli() }

// CreateSession 新建会话（id 冲突报错）。
func (r *Repo) CreateSession(ctx context.Context, s Session) error {
	if s.ID == "" {
		return fmt.Errorf("%w: id empty", ErrInvalidSession)
	}
	now := nowMilli()
	if s.CreatedAt == 0 {
		s.CreatedAt = now
	}
	if s.UpdatedAt == 0 {
		s.UpdatedAt = s.CreatedAt
	}
	_, err := r.st.Exec(ctx,
		`INSERT INTO sessions(id,title,model,created_at,updated_at) VALUES(?,?,?,?,?)`,
		s.ID, s.Title, s.Model, s.CreatedAt, s.UpdatedAt)
	return err
}

// GetSession 取会话；不存在返回 ErrSessionNotFound。
func (r *Repo) GetSession(ctx context.Context, id string) (Session, error) {
	var s Session
	rows, err := r.st.Query(ctx,
		`SELECT id,title,model,created_at,updated_at FROM sessions WHERE id=?`, id)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	if !rows.Next() {
		if e := rows.Err(); e != nil {
			return s, e
		}
		return s, ErrSessionNotFound
	}
	if err := rows.Scan(&s.ID, &s.Title, &s.Model, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return s, err
	}
	return s, nil
}

// UpdateSessionTitle 更新标题并刷新 updated_at。
func (r *Repo) UpdateSessionTitle(ctx context.Context, id, title string) error {
	res, err := r.st.Exec(ctx, `UPDATE sessions SET title=?, updated_at=? WHERE id=?`,
		title, nowMilli(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// ListSessions 分页列会话（按 updated_at 倒序）。offset/limit 分页。
func (r *Repo) ListSessions(ctx context.Context, limit, offset int) ([]Session, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.st.Query(ctx,
		`SELECT id,title,model,created_at,updated_at FROM sessions ORDER BY updated_at DESC LIMIT ? OFFSET ?`,
		limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.Title, &s.Model, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// DeleteSession 删会话（外键 ON DELETE CASCADE 连带删 messages）。
func (r *Repo) DeleteSession(ctx context.Context, id string) error {
	res, err := r.st.Exec(ctx, `DELETE FROM sessions WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// AppendMessage 追加消息：blocks 序列化后经 FieldCipher 加密（AAD=行 id+字段）落库，
// 并刷新所属会话 updated_at。会话不存在则外键失败。
func (r *Repo) AppendMessage(ctx context.Context, m types.Message) (int64, error) {
	if m.ID == "" || m.SessionID == "" {
		return 0, fmt.Errorf("%w: id/session_id empty", ErrInvalidMessage)
	}
	if !m.Role.Valid() {
		return 0, fmt.Errorf("%w: invalid role %q", ErrInvalidMessage, m.Role)
	}
	blocksJSON, err := json.Marshal(m.Blocks)
	if err != nil {
		return 0, err
	}
	ct, err := r.cipher.SealField(blocksJSON, m.ID, "blocks")
	if err != nil {
		return 0, err
	}
	created := m.CreatedAt
	if created == 0 {
		created = nowMilli()
	}
	// 原子事务：INSERT 消息 + 刷新会话 updated_at 一起提交；updated_at 用 MAX(旧,新) 防倒调。
	seq, err := r.st.execTx(ctx, []stmt{
		{query: `INSERT INTO messages(id,session_id,role,content_cipher,model,created_at) VALUES(?,?,?,?,?,?)`,
			args: []any{m.ID, m.SessionID, string(m.Role), ct, m.Model, created}},
		{query: `UPDATE sessions SET updated_at=MAX(updated_at,?) WHERE id=?`,
			args: []any{created, m.SessionID}},
	})
	if err != nil {
		return 0, err
	}
	return seq, nil
}

// GetMessage 取单条消息（解密 blocks）。
func (r *Repo) GetMessage(ctx context.Context, id string) (MessageRecord, error) {
	var rec MessageRecord
	rows, err := r.st.Query(ctx,
		`SELECT seq,id,session_id,role,content_cipher,model,created_at FROM messages WHERE id=?`, id)
	if err != nil {
		return rec, err
	}
	defer rows.Close()
	if !rows.Next() {
		if e := rows.Err(); e != nil {
			return rec, e
		}
		return rec, ErrMessageNotFound
	}
	if err := r.scanMessage(rows, &rec); err != nil {
		return rec, err
	}
	return rec, nil
}

// ListMessages 分页拉某会话消息（按 seq 倒序，最新在前）。
// beforeSeq=0 从最新开始；返回 NextBeforeSeq 作为下一页入参，HasMore 指示是否还有更早的。
func (r *Repo) ListMessages(ctx context.Context, sessionID string, limit int, beforeSeq int64) (MessagePage, error) {
	if limit <= 0 {
		limit = 50
	}
	// 多取一条判断 hasMore。
	q := `SELECT seq,id,session_id,role,content_cipher,model,created_at FROM messages WHERE session_id=?`
	args := []any{sessionID}
	if beforeSeq > 0 {
		q += ` AND seq < ?`
		args = append(args, beforeSeq)
	}
	q += ` ORDER BY seq DESC LIMIT ?`
	args = append(args, limit+1)

	rows, err := r.st.Query(ctx, q, args...)
	if err != nil {
		return MessagePage{}, err
	}
	defer rows.Close()
	var recs []MessageRecord
	for rows.Next() {
		var rec MessageRecord
		if err := r.scanMessage(rows, &rec); err != nil {
			return MessagePage{}, err
		}
		recs = append(recs, rec)
	}
	if err := rows.Err(); err != nil {
		return MessagePage{}, err
	}
	page := MessagePage{}
	if len(recs) > limit {
		page.HasMore = true
		recs = recs[:limit]
	}
	page.Records = recs
	if page.HasMore && len(recs) > 0 {
		page.NextBeforeSeq = recs[len(recs)-1].Seq
	}
	return page, nil
}

// CountMessages 会话消息数（测试/UI 用）。
func (r *Repo) CountMessages(ctx context.Context, sessionID string) (int, error) {
	var n int
	err := r.st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE session_id=?`, sessionID).Scan(&n)
	return n, err
}

// scanMessage 从一行扫描并解密为 MessageRecord。
func (r *Repo) scanMessage(rows *sql.Rows, rec *MessageRecord) error {
	var (
		id, sessionID, role, model string
		ct                         []byte
		createdAt                  int64
	)
	if err := rows.Scan(&rec.Seq, &id, &sessionID, &role, &ct, &model, &createdAt); err != nil {
		return err
	}
	plain, err := r.cipher.OpenField(ct, id, "blocks")
	if err != nil {
		return fmt.Errorf("store: decrypt message %s: %w", id, err)
	}
	var blocks []types.ContentBlock
	if err := json.Unmarshal(plain, &blocks); err != nil {
		return fmt.Errorf("store: unmarshal blocks %s: %w", id, err)
	}
	rec.Message = types.Message{
		ID:        id,
		SessionID: sessionID,
		Role:      types.Role(role),
		Blocks:    blocks,
		Model:     model,
		CreatedAt: createdAt,
	}
	return nil
}
