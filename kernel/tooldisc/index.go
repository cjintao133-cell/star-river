// Package tooldisc 是工具「两段式按需发现」（T-M6-01 追加，千级规模不惧）。
//
// 背景：远程 MCP 可挂几百上千个工具，如果把全部工具的完整 schema 常驻进模型上下文，
// token 会被撑爆、启动时连接全部 server 也会拖垮。本包实现：
//
//  1. **工具注册表落 SQLite**：server 命名空间 + 工具名 + 短描述 + 分类 + 完整 schema（分离存）。
//     启动不拉全部 schema；工具列表懒拉取 + 缓存；热插拔（Replace/Remove）走注册表刷新。
//  2. **固定小 footprint 注入**：上下文只常驻一个 tool_search 元工具（+可选分类目录），
//     完整 schema 仅在命中后按名拉取，不进常驻上下文。footprint token 与工具总数无关（常量）。
//  3. **懒连接**：MCP server 仅在被检索命中/调用时建连，空闲超时断开；不启动时连全部。
//  4. 检索 v1：工具名/短描述/分类关键词 + 分类过滤（SQLite 内建 LIKE，零新依赖）；语义检索后续可选增强。
//
// 依赖方向（红线 2）：本包只 import 标准库、kernel/store（SQLite 底座）、kernel/tool（Tool 类型 + 内置工具接口）。
// **不 import 任何伴侣伴侣插件包**（hippocampus/endocrine/proactive）。
package tooldisc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"star-river/kernel/store"
	"star-river/kernel/tool"
)

// IndexedTool 一个待入索引的工具（完整定义 + 分类）。
type IndexedTool struct {
	Tool     tool.Tool
	Category string
}

// FromTools 用工具定义构造索引条目（分类空）。
func FromTools(tools []tool.Tool) []IndexedTool {
	out := make([]IndexedTool, len(tools))
	for i, t := range tools {
		out[i] = IndexedTool{Tool: t}
	}
	return out
}

// ToolMeta 检索命中的工具元数据（**不含完整 schema**，保证 footprint 小）。
type ToolMeta struct {
	Server    string `json:"server"`
	Name      string `json:"name"`
	Title     string `json:"title,omitempty"`
	ShortDesc string `json:"short_desc,omitempty"`
	Category  string `json:"category,omitempty"`
}

// shortDesc 把描述截断为短描述（去换行，限 80 字符）。
func shortDesc(desc string) string {
	d := strings.ReplaceAll(desc, "\n", " ")
	d = strings.TrimSpace(d)
	r := []rune(d)
	if len(r) > 80 {
		return string(r[:80]) + "…"
	}
	return d
}

// Index SQLite 工具注册表。并发安全由底层 store 保证（写串行、读并发）。
type Index struct {
	st *store.Store
}

// NewIndex 建注册表（幂等建表）。
func NewIndex(st *store.Store) (*Index, error) {
	if st == nil {
		return nil, fmt.Errorf("tooldisc: nil store")
	}
	if _, err := st.DB().Exec(`CREATE TABLE IF NOT EXISTS tool_index (
        server     TEXT NOT NULL,
        name       TEXT NOT NULL,
        title      TEXT NOT NULL DEFAULT '',
        short_desc TEXT NOT NULL DEFAULT '',
        category   TEXT NOT NULL DEFAULT '',
        schema     TEXT NOT NULL DEFAULT '',
        PRIMARY KEY (server, name)
    )`); err != nil {
		return nil, fmt.Errorf("tooldisc: create table: %w", err)
	}
	if _, err := st.DB().Exec(`CREATE INDEX IF NOT EXISTS idx_tool_index_category ON tool_index(category)`); err != nil {
		return nil, fmt.Errorf("tooldisc: create index: %w", err)
	}
	return &Index{st: st}, nil
}

// ReplaceServer 整体替换某 server 的工具集（热插拔：先删后插，事务内原子）。
func (ix *Index) ReplaceServer(ctx context.Context, server string, items []IndexedTool) error {
	if server == "" {
		return fmt.Errorf("tooldisc: empty server")
	}
	tx, err := ix.st.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM tool_index WHERE server=?`, server); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO tool_index(server,name,title,short_desc,category,schema) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, it := range items {
		if it.Tool.Name == "" {
			continue
		}
		schema := string(it.Tool.InputSchema)
		if schema == "" {
			schema = "{}"
		}
		if _, err := stmt.ExecContext(ctx, server, it.Tool.Name, it.Tool.Title, shortDesc(it.Tool.Description), it.Category, schema); err != nil {
			return fmt.Errorf("tooldisc: insert %s/%s: %w", server, it.Tool.Name, err)
		}
	}
	return tx.Commit()
}

// RemoveServer 移除某 server 的全部工具（热插拔：拔除）。
func (ix *Index) RemoveServer(ctx context.Context, server string) error {
	_, err := ix.st.Exec(ctx, `DELETE FROM tool_index WHERE server=?`, server)
	return err
}

// SearchOptions 检索选项。
type SearchOptions struct {
	Category string // 非空则按分类过滤
	Limit    int    // <=0 默认 20
}

// Search 关键词检索工具元数据（不含 schema）。query 为空时按 limit 列出。
// 排序：名称命中优先，其次名称。
func (ix *Index) Search(ctx context.Context, query string, opt SearchOptions) ([]ToolMeta, error) {
	limit := opt.Limit
	if limit <= 0 {
		limit = 20
	}
	q := strings.TrimSpace(strings.ToLower(query))
	var (
		sql  string
		args []any
	)
	if q == "" {
		sql = `SELECT server,name,title,short_desc,category FROM tool_index`
		if opt.Category != "" {
			sql += ` WHERE category=?`
			args = append(args, opt.Category)
		}
		sql += ` ORDER BY server,name LIMIT ?`
		args = append(args, limit)
	} else {
		like := "%" + q + "%"
		sql = `SELECT server,name,title,short_desc,category FROM tool_index
               WHERE (lower(name) LIKE ? OR lower(short_desc) LIKE ? OR lower(title) LIKE ? OR lower(category) LIKE ?)`
		args = append(args, like, like, like, like)
		if opt.Category != "" {
			sql += ` AND category=?`
			args = append(args, opt.Category)
		}
		sql += ` ORDER BY (CASE WHEN lower(name) LIKE ? THEN 0 ELSE 1 END), server, name LIMIT ?`
		args = append(args, like, limit)
	}
	rows, err := ix.st.DB().QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ToolMeta
	for rows.Next() {
		var m ToolMeta
		if err := rows.Scan(&m.Server, &m.Name, &m.Title, &m.ShortDesc, &m.Category); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Schema 懒取某工具的完整 inputSchema。命中返回 schema+true。
func (ix *Index) Schema(ctx context.Context, server, name string) (json.RawMessage, bool, error) {
	var s string
	err := ix.st.DB().QueryRowContext(ctx, `SELECT schema FROM tool_index WHERE server=? AND name=?`, server, name).Scan(&s)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return nil, false, nil
		}
		return nil, false, err
	}
	if s == "" {
		s = "{}"
	}
	return json.RawMessage(s), true, nil
}

// Count 工具总数。
func (ix *Index) Count(ctx context.Context) (int, error) {
	var n int
	err := ix.st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tool_index`).Scan(&n)
	return n, err
}

// ServerCounts 各 server 的工具数。
func (ix *Index) ServerCounts(ctx context.Context) (map[string]int, error) {
	rows, err := ix.st.DB().QueryContext(ctx, `SELECT server, COUNT(*) FROM tool_index GROUP BY server`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		out[s] = n
	}
	return out, rows.Err()
}

// Categories 全部分类（去重、排序）。
func (ix *Index) Categories(ctx context.Context) ([]string, error) {
	rows, err := ix.st.DB().QueryContext(ctx, `SELECT DISTINCT category FROM tool_index WHERE category<>'' ORDER BY category`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
