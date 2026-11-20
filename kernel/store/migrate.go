package store

import (
	"database/sql"
	"fmt"
	"time"
)

// migration 一条版本迁移。stmts 在该版本事务内顺序执行。
type migration struct {
	version int
	name    string
	stmts   []string
}

// migrations 迁移注册表，按 version 升序。新增表/列时追加，勿改历史条目。
// v1：app_meta（应用元数据 KV），T-M2-01 基座验证用。
// v2：sessions / messages（T-M2-03），messages.content_cipher 存密文 BLOB。
// v3：model_stats（T-M3-07）每模型请求/成功/失败/延迟/token 本地累计。
var migrations = []migration{
	{
		version: 1,
		name:    "app_meta",
		stmts: []string{
			`CREATE TABLE app_meta (
                key   TEXT PRIMARY KEY,
                value TEXT NOT NULL
            )`,
		},
	},
	{
		version: 2,
		name:    "sessions_messages",
		stmts: []string{
			`CREATE TABLE sessions (
                id         TEXT PRIMARY KEY,
                title      TEXT NOT NULL DEFAULT '',
                model      TEXT NOT NULL DEFAULT '',
                created_at INTEGER NOT NULL,
                updated_at INTEGER NOT NULL
            )`,
			`CREATE TABLE messages (
                seq            INTEGER PRIMARY KEY AUTOINCREMENT,
                id             TEXT NOT NULL UNIQUE,
                session_id     TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
                role           TEXT NOT NULL,
                content_cipher BLOB NOT NULL,
                model          TEXT NOT NULL DEFAULT '',
                created_at     INTEGER NOT NULL
            )`,
			`CREATE INDEX idx_messages_session_seq ON messages(session_id, seq)`,
			`CREATE INDEX idx_sessions_updated ON sessions(updated_at)`,
		},
	},
	{
		version: 3,
		name:    "model_stats",
		stmts: []string{
			`CREATE TABLE model_stats (
                model_id          TEXT PRIMARY KEY,
                requests          INTEGER NOT NULL DEFAULT 0,
                success           INTEGER NOT NULL DEFAULT 0,
                fail              INTEGER NOT NULL DEFAULT 0,
                total_latency_ms  INTEGER NOT NULL DEFAULT 0,
                last_latency_ms   INTEGER NOT NULL DEFAULT 0,
                prompt_tokens     INTEGER NOT NULL DEFAULT 0,
                completion_tokens INTEGER NOT NULL DEFAULT 0,
                updated_at        INTEGER NOT NULL DEFAULT 0
            )`,
		},
	},
	{
		version: 4,
		name:    "characters",
		stmts: []string{
			`CREATE TABLE characters (
                id         TEXT PRIMARY KEY,
                name       TEXT NOT NULL DEFAULT '',
                description TEXT NOT NULL DEFAULT '',
                personality TEXT NOT NULL DEFAULT '',
                scenario   TEXT NOT NULL DEFAULT '',
                first_mes  TEXT NOT NULL DEFAULT '',
                alternate_greetings TEXT NOT NULL DEFAULT '[]',
                mes_example TEXT NOT NULL DEFAULT '',
                system_prompt TEXT NOT NULL DEFAULT '',
                post_history_instructions TEXT NOT NULL DEFAULT '',
                character_book TEXT NOT NULL DEFAULT '{}',
                avatar     BLOB,
                persona_image BLOB,
                created_at INTEGER NOT NULL DEFAULT 0,
                updated_at INTEGER NOT NULL DEFAULT 0
            )`,
			`ALTER TABLE sessions ADD COLUMN character_id TEXT NOT NULL DEFAULT ''`,
			`CREATE INDEX idx_characters_updated ON characters(updated_at)`,
		},
	},
}

// migrate 执行迁移框架：bootstrap 版本表 → 读已应用集合 → 按序在事务内应用未应用者。
func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
        version    INTEGER PRIMARY KEY,
        name       TEXT NOT NULL,
        applied_at INTEGER NOT NULL
    )`); err != nil {
		return fmt.Errorf("store: bootstrap schema_migrations: %w", err)
	}

	applied := map[int]bool{}
	rows, err := s.db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("store: read schema_migrations: %w", err)
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	// 防降级/断档：拒绝「数据库 schema 比本程序新」或「已应用版本有缺口」的库，避免静默跑在未知 schema 上。
	latest := LatestVersion()
	maxApplied := 0
	for v := range applied {
		if v > maxApplied {
			maxApplied = v
		}
	}
	if maxApplied > latest {
		return fmt.Errorf("store: database schema v%d is newer than this build supports (max v%d); refusing to run to avoid corruption", maxApplied, latest)
	}
	for v := 1; v <= maxApplied; v++ {
		if !applied[v] {
			return fmt.Errorf("store: schema gap: applied max v%d but missing v%d; refusing to run", maxApplied, v)
		}
	}

	for _, m := range migrations {
		if applied[m.version] {
			continue
		}
		if err := s.applyOne(m); err != nil {
			return err
		}
	}
	return nil
}

// applyOne 在单事务内应用一条迁移：执行全部 stmts + 记录版本。
func (s *Store) applyOne(m migration) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, stmt := range m.stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("store: migration v%d(%s): %w", m.version, m.name, err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations(version,name,applied_at) VALUES(?,?,?)`,
		m.version, m.name, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("store: record migration v%d: %w", m.version, err)
	}
	return tx.Commit()
}

// SchemaVersion 返回当前已应用的最大迁移版本（无则 0）。
func (s *Store) SchemaVersion() (int, error) {
	var v sql.NullInt64
	if err := s.db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&v); err != nil {
		return 0, err
	}
	if !v.Valid {
		return 0, nil
	}
	return int(v.Int64), nil
}

// LatestVersion 返回注册表里最高版本（迁移目标）。
func LatestVersion() int {
	max := 0
	for _, m := range migrations {
		if m.version > max {
			max = m.version
		}
	}
	return max
}
