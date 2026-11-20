package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"star-river/kernel/core/types"
)

// AppendMessage 的会话 updated_at 必须取 MAX(旧,新)，不允许倒调（astra 评审：两次独立提交+倒调）。
func TestAppendMessageUpdatedAtNoBackwards(t *testing.T) {
	r, st, _ := newRepo(t)
	ctx := context.Background()
	if err := r.CreateSession(ctx, Session{ID: "s1", Title: "t"}); err != nil {
		t.Fatal(err)
	}
	// 人为把 updated_at 抬到很大，断言随后用小 created 追加不会把它拉低。
	if _, err := st.Exec(ctx, `UPDATE sessions SET updated_at=? WHERE id=?`, int64(10_000), "s1"); err != nil {
		t.Fatal(err)
	}
	seq, err := r.AppendMessage(ctx, mkMsg("m1", "s1", types.RoleUser, "hi", 1000))
	if err != nil {
		t.Fatal(err)
	}
	if seq <= 0 {
		t.Fatalf("seq=%d", seq)
	}
	var u int64
	if err := st.DB().QueryRow(`SELECT updated_at FROM sessions WHERE id=?`, "s1").Scan(&u); err != nil {
		t.Fatal(err)
	}
	if u != 10_000 {
		t.Fatalf("updated_at 被倒调: got %d want 10000", u)
	}
	// 正常前进：created 更大时应抬升。
	if _, err := r.AppendMessage(ctx, mkMsg("m2", "s1", types.RoleAssistant, "yo", 20_000)); err != nil {
		t.Fatal(err)
	}
	st.DB().QueryRow(`SELECT updated_at FROM sessions WHERE id=?`, "s1").Scan(&u)
	if u != 20_000 {
		t.Fatalf("updated_at 未前进: got %d want 20000", u)
	}
	// 消息确实入库。
	page, err := r.ListMessages(ctx, "s1", 10, 0)
	if err != nil || len(page.Records) != 2 {
		t.Fatalf("messages=%d err=%v", len(page.Records), err)
	}
}

// migrate 必须拒绝「数据库 schema 比本程序新」的库（防止降级跑坏数据）。
func TestMigrateRejectsFutureSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	db, err := sql.Open("sqlite", dsnFor(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations(version,name,applied_at) VALUES(99,'future',0)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := Open(path); err == nil {
		t.Fatal("应拒绝未来 schema")
	} else if !strings.Contains(err.Error(), "newer") {
		t.Fatalf("错误信息不含 newer: %v", err)
	}
}

// migrate 必须拒绝「已应用版本有缺口」的库。
func TestMigrateRejectsGap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gap.db")
	st, err := Open(path) // 应用全部迁移（假设 ≥3 条）
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	if LatestVersion() < 3 {
		t.Skip("迁移条数 <3，跳过断档测试")
	}
	db, err := sql.Open("sqlite", dsnFor(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version=2`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := Open(path); err == nil {
		t.Fatal("应拒绝断档 schema")
	} else if !strings.Contains(err.Error(), "gap") {
		t.Fatalf("错误信息不含 gap: %v", err)
	}
}
