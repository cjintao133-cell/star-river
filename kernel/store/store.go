// Package store SQLite 存储基座（T-M2-01）。
//
// 设计要点（docs/architecture.md 批准优化#3）：
//   - 驱动 modernc.org/sqlite（纯 Go，零 cgo，三平台交叉编译；已按依赖清单入账）。
//   - WAL 模式：多读并发 + 单写；busy_timeout 兜底；foreign_keys 开启。
//   - **单写 goroutine 队列**：所有写经内部 channel 交给唯一 writer 串行执行，
//     应用层即保证「至多一个写者」，配合 WAL 杜绝 SQLITE_BUSY 锁错误；读走连接池并发。
//   - 迁移框架：schema_migrations 版本表，按版本号顺序在事务内应用（见 migrate.go）。
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite" // 注册 "sqlite" 驱动
)

// ErrClosed 对已关闭的 Store 执行写/查询时返回。
var ErrClosed = errors.New("store: closed")

// writeOp 一次写请求（经 writer goroutine 串行执行）。
type writeOp struct {
	ctx   context.Context
	query string
	args  []any
	resp  chan writeResult
}

type writeResult struct {
	res sql.Result
	err error
}

// Store SQLite 存储。并发安全；写串行、读并发。
type Store struct {
	db   *sql.DB
	ops  chan writeOp
	quit chan struct{}
	wg   sync.WaitGroup

	closeOnce sync.Once
	closeErr  error
}

// dsnFor 组织 DSN：WAL + busy_timeout + foreign_keys + synchronous(NORMAL)。
// modernc.org/sqlite 支持 _pragma=name(value) 形式，逐连接应用。
func dsnFor(path string) string {
	return "file:" + path +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=synchronous(NORMAL)"
}

// Open 打开（或创建）数据库：设 WAL 等 pragma，执行迁移，启动 writer goroutine。
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("store: empty path")
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("store: mkdir %s: %w", dir, err)
		}
	}
	db, err := sql.Open("sqlite", dsnFor(path))
	if err != nil {
		return nil, err
	}
	// 连接池：单写 goroutine 已串行化写；读并发，池上限适中避免过多连接争锁。
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)

	s := &Store{
		db:   db,
		ops:  make(chan writeOp, 256),
		quit: make(chan struct{}),
	}
	// 迁移在启动 writer 之前同步执行（单线程，无并发写）。
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	s.wg.Add(1)
	go s.writer()
	return s, nil
}

// writer 唯一写者：串行执行 op；收到 quit 后排干并回 ErrClosed，再退出。
func (s *Store) writer() {
	defer s.wg.Done()
	for {
		select {
		case op := <-s.ops:
			res, err := s.db.ExecContext(op.ctx, op.query, op.args...)
			op.resp <- writeResult{res: res, err: err}
		case <-s.quit:
			for {
				select {
				case op := <-s.ops:
					op.resp <- writeResult{err: ErrClosed}
				default:
					return
				}
			}
		}
	}
}

// Exec 经单写 goroutine 串行执行一条写语句。
func (s *Store) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	op := writeOp{ctx: ctx, query: query, args: args, resp: make(chan writeResult, 1)}
	select {
	case s.ops <- op:
	case <-s.quit:
		return nil, ErrClosed
	}
	select {
	case r := <-op.resp:
		return r.res, r.err
	case <-s.quit:
		// writer 可能已 drain 并回写 resp；非阻塞再取一次。
		select {
		case r := <-op.resp:
			return r.res, r.err
		default:
			return nil, ErrClosed
		}
	}
}

// Query 并发读（不经 writer，走连接池）。
func (s *Store) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	select {
	case <-s.quit:
		return nil, ErrClosed
	default:
	}
	return s.db.QueryContext(ctx, query, args...)
}

// DB 暴露底层 *sql.DB（迁移/测试用；生产优先走 Exec/Query）。
func (s *Store) DB() *sql.DB { return s.db }

// Close 幂等关闭：停 writer（排干回 ErrClosed）→ 关底层 DB。
func (s *Store) Close() error {
	s.closeOnce.Do(func() {
		close(s.quit)
		s.wg.Wait()
		s.closeErr = s.db.Close()
	})
	return s.closeErr
}
