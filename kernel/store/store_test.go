package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// WAL 模式生效（并发读写基座）。
func TestOpenWALMode(t *testing.T) {
	s := openTemp(t)
	rows, err := s.Query(context.Background(), `PRAGMA journal_mode`)
	if err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("no row")
	}
	var mode string
	if err := rows.Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
}

func TestMigrateVersionAndAppMeta(t *testing.T) {
	s := openTemp(t)
	v, err := s.SchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if v != LatestVersion() || v < 1 {
		t.Errorf("SchemaVersion = %d, want %d", v, LatestVersion())
	}
	// app_meta 可用。
	if _, err := s.Exec(context.Background(), `INSERT INTO app_meta(key,value) VALUES(?,?)`, "theme", "dark"); err != nil {
		t.Fatalf("insert app_meta: %v", err)
	}
	rows, err := s.Query(context.Background(), `SELECT value FROM app_meta WHERE key=?`, "theme")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("no row")
	}
	var val string
	rows.Scan(&val)
	if val != "dark" {
		t.Errorf("value = %q", val)
	}
}

// 重开同一库：迁移不重复应用（版本稳定、数据保留）。
func TestMigrateIdempotentOnReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reopen.db")
	s1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s1.Exec(context.Background(), `INSERT INTO app_meta(key,value) VALUES('k','v')`); err != nil {
		t.Fatal(err)
	}
	s1.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	v, _ := s2.SchemaVersion()
	if v != LatestVersion() {
		t.Errorf("reopen SchemaVersion = %d", v)
	}
	rows, err := s2.Query(context.Background(), `SELECT COUNT(*) FROM app_meta`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	rows.Next()
	var n int
	rows.Scan(&n)
	if n != 1 {
		t.Errorf("app_meta rows after reopen = %d, want 1", n)
	}
}

// 并发读写压测：多写者经单写 goroutine，多读者并发；断言零锁错误、最终计数正确。
func TestConcurrentReadWriteNoLock(t *testing.T) {
	s := openTemp(t)
	const writers = 6
	const perWriter = 40
	const readers = 6
	const perReader = 80

	var wg sync.WaitGroup
	var errCount int64

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for j := 0; j < perWriter; j++ {
				key := fmt.Sprintf("w%d-k%d", w, j)
				if _, err := s.Exec(context.Background(),
					`INSERT INTO app_meta(key,value) VALUES(?,?)`, key, "x"); err != nil {
					atomic.AddInt64(&errCount, 1)
					t.Errorf("writer %d insert: %v", w, err)
					return
				}
			}
		}(w)
	}
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			for j := 0; j < perReader; j++ {
				rows, err := s.Query(context.Background(), `SELECT COUNT(*) FROM app_meta`)
				if err != nil {
					atomic.AddInt64(&errCount, 1)
					t.Errorf("reader %d query: %v", r, err)
					return
				}
				rows.Next()
				var n int
				rows.Scan(&n)
				rows.Close()
			}
		}(r)
	}
	wg.Wait()

	if errCount != 0 {
		t.Fatalf("并发出现 %d 个错误（应为 0，WAL+单写应无锁错误）", errCount)
	}
	rows, err := s.Query(context.Background(), `SELECT COUNT(*) FROM app_meta`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	rows.Next()
	var total int
	rows.Scan(&total)
	if want := writers * perWriter; total != want {
		t.Errorf("final count = %d, want %d", total, want)
	}
}

// 关闭后写入返回 ErrClosed（不 panic、不写坏通道）。
func TestExecAfterClose(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := s.Exec(context.Background(), `INSERT INTO app_meta(key,value) VALUES('a','b')`); !errors.Is(err, ErrClosed) {
		t.Errorf("Exec after close err = %v, want ErrClosed", err)
	}
	if _, err := s.Query(context.Background(), `SELECT 1`); !errors.Is(err, ErrClosed) {
		t.Errorf("Query after close err = %v, want ErrClosed", err)
	}
	// Close 幂等。
	if err := s.Close(); err != nil {
		t.Errorf("second Close err = %v", err)
	}
}

// 直接构造底层连接也应是 WAL（验证 DSN pragma 对每个连接生效）。
func TestDSNPragmaApplied(t *testing.T) {
	s := openTemp(t)
	var mode string
	err := s.DB().QueryRow(`PRAGMA journal_mode`).Scan(&mode)
	if err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Errorf("DB journal_mode = %q", mode)
	}
	var fk int
	s.DB().QueryRow(`PRAGMA foreign_keys`).Scan(&fk)
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
}

var _ = sql.ErrNoRows // keep database/sql imported for clarity in helpers
