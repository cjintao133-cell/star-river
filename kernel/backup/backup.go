package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/cjintao133-cell/star-river/kernel/store"
)

// Format 归档格式标识 / 版本。
const (
	Format        = "star-river-backup"
	FormatVersion = 1
)

// 错误。
var (
	ErrBadArchive      = errors.New("backup: not a valid star-river backup")
	ErrVersionMismatch = errors.New("backup: unsupported format version")
	ErrCorrupt         = errors.New("backup: integrity check failed")
)

// 归档内数据文件名。
const (
	fileSessions   = "sessions.jsonl"
	fileMessages   = "messages.jsonl"
	fileModelStats = "model_stats.jsonl"
	fileAppMeta    = "app_meta.jsonl"
	fileManifest   = "manifest.json"
)

// Manifest 归档清单。
type Manifest struct {
	Format     string            `json:"format"`
	Version    int               `json:"version"`
	ExportedAt int64             `json:"exported_at"`
	Files      map[string]string `json:"files"` // 文件名 -> sha256 hex
}

// Options 导出选项。
type Options struct {
	// Extras 额外文件（如已加密的 keys.enc、config.json），原样纳入并计入校验。
	Extras map[string][]byte
	// ExportedAt 导出时刻（UnixMilli）；由调用方填（本包不取真实时间以保持可测）。
	ExportedAt int64
}

// Result 导入结果。
type Result struct {
	Manifest Manifest
	Extras   map[string][]byte // 归档中除数据文件与 manifest 外的额外文件
}

// —— 行模型（JSONL）——

type sessionRow struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Model     string `json:"model"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type messageRow struct {
	ID            string `json:"id"`
	SessionID     string `json:"session_id"`
	Role          string `json:"role"`
	ContentCipher []byte `json:"content_cipher"` // encoding/json 自动 base64
	Model         string `json:"model"`
	CreatedAt     int64  `json:"created_at"`
}

type statsRow struct {
	ModelID          string `json:"model_id"`
	Requests         int64  `json:"requests"`
	Success          int64  `json:"success"`
	Fail             int64  `json:"fail"`
	TotalLatencyMs   int64  `json:"total_latency_ms"`
	LastLatencyMs    int64  `json:"last_latency_ms"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	UpdatedAt        int64  `json:"updated_at"`
}

type metaRow struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// WriteArchive 把 st 的数据（+opts.Extras）写成 zip 归档到 w，返回清单。
func WriteArchive(ctx context.Context, st *store.Store, w io.Writer, opts Options) (Manifest, error) {
	files := map[string][]byte{}
	if err := dumpSessions(ctx, st, files); err != nil {
		return Manifest{}, err
	}
	if err := dumpMessages(ctx, st, files); err != nil {
		return Manifest{}, err
	}
	if err := dumpStats(ctx, st, files); err != nil {
		return Manifest{}, err
	}
	if err := dumpMeta(ctx, st, files); err != nil {
		return Manifest{}, err
	}
	for name, data := range opts.Extras {
		if name == "" || name == fileManifest {
			return Manifest{}, fmt.Errorf("%w: invalid extra file name %q", ErrBadArchive, name)
		}
		files[name] = data
	}

	sums := make(map[string]string, len(files))
	for name, data := range files {
		sums[name] = sha256Hex(data)
	}
	man := Manifest{Format: Format, Version: FormatVersion, ExportedAt: opts.ExportedAt, Files: sums}
	manBytes, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return Manifest{}, err
	}

	zw := zip.NewWriter(w)
	if err := writeZipFile(zw, fileManifest, manBytes); err != nil {
		return Manifest{}, err
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if err := writeZipFile(zw, n, files[n]); err != nil {
			return Manifest{}, err
		}
	}
	if err := zw.Close(); err != nil {
		return Manifest{}, err
	}
	return man, nil
}

// ReadArchive 从 data（zip 字节）读取归档并**替换式**导入 st（先清空数据表再写入，事务内原子）。
// 校验格式/版本/每文件完整性；返回清单与额外文件。
//
// 注意：导入为维护操作，要求对 st 无并发写（典型场景：启动时恢复）。内部用底层事务。
func ReadArchive(ctx context.Context, st *store.Store, data []byte) (Result, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Result{}, fmt.Errorf("%w: open zip: %v", ErrBadArchive, err)
	}
	files := map[string][]byte{}
	for _, f := range zr.File {
		b, err := readZipFile(f)
		if err != nil {
			return Result{}, fmt.Errorf("%w: read %s: %v", ErrBadArchive, f.Name, err)
		}
		files[f.Name] = b
	}
	manBytes, ok := files[fileManifest]
	if !ok {
		return Result{}, fmt.Errorf("%w: missing manifest", ErrBadArchive)
	}
	var man Manifest
	if err := json.Unmarshal(manBytes, &man); err != nil {
		return Result{}, fmt.Errorf("%w: bad manifest: %v", ErrBadArchive, err)
	}
	if man.Format != Format {
		return Result{}, fmt.Errorf("%w: unexpected format %q", ErrBadArchive, man.Format)
	}
	if man.Version != FormatVersion {
		return Result{}, fmt.Errorf("%w: got %d want %d", ErrVersionMismatch, man.Version, FormatVersion)
	}
	// 完整性：清单登记的每个文件都必须存在且校验一致。
	for name, want := range man.Files {
		b, ok := files[name]
		if !ok {
			return Result{}, fmt.Errorf("%w: missing file %s", ErrCorrupt, name)
		}
		if sha256Hex(b) != want {
			return Result{}, fmt.Errorf("%w: checksum mismatch %s", ErrCorrupt, name)
		}
	}

	sessions, err := parseJSONL[sessionRow](files[fileSessions])
	if err != nil {
		return Result{}, fmt.Errorf("%w: sessions: %v", ErrBadArchive, err)
	}
	messages, err := parseJSONL[messageRow](files[fileMessages])
	if err != nil {
		return Result{}, fmt.Errorf("%w: messages: %v", ErrBadArchive, err)
	}
	stats, err := parseJSONL[statsRow](files[fileModelStats])
	if err != nil {
		return Result{}, fmt.Errorf("%w: model_stats: %v", ErrBadArchive, err)
	}
	metas, err := parseJSONL[metaRow](files[fileAppMeta])
	if err != nil {
		return Result{}, fmt.Errorf("%w: app_meta: %v", ErrBadArchive, err)
	}

	if err := replaceTables(ctx, st, sessions, messages, stats, metas); err != nil {
		return Result{}, err
	}

	extras := map[string][]byte{}
	for name, b := range files {
		switch name {
		case fileManifest, fileSessions, fileMessages, fileModelStats, fileAppMeta:
			continue
		}
		extras[name] = b
	}
	return Result{Manifest: man, Extras: extras}, nil
}

// —— dump（导出）——

func dumpSessions(ctx context.Context, st *store.Store, out map[string][]byte) error {
	rows, err := st.DB().QueryContext(ctx, `SELECT id,title,model,created_at,updated_at FROM sessions ORDER BY id`)
	if err != nil {
		return fmt.Errorf("backup: dump sessions: %w", err)
	}
	defer rows.Close()
	var buf bytes.Buffer
	for rows.Next() {
		var r sessionRow
		if err := rows.Scan(&r.ID, &r.Title, &r.Model, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return err
		}
		if err := writeJSONL(&buf, r); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	out[fileSessions] = buf.Bytes()
	return nil
}

func dumpMessages(ctx context.Context, st *store.Store, out map[string][]byte) error {
	rows, err := st.DB().QueryContext(ctx, `SELECT id,session_id,role,content_cipher,model,created_at FROM messages ORDER BY seq`)
	if err != nil {
		return fmt.Errorf("backup: dump messages: %w", err)
	}
	defer rows.Close()
	var buf bytes.Buffer
	for rows.Next() {
		var r messageRow
		if err := rows.Scan(&r.ID, &r.SessionID, &r.Role, &r.ContentCipher, &r.Model, &r.CreatedAt); err != nil {
			return err
		}
		if err := writeJSONL(&buf, r); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	out[fileMessages] = buf.Bytes()
	return nil
}

func dumpStats(ctx context.Context, st *store.Store, out map[string][]byte) error {
	rows, err := st.DB().QueryContext(ctx, `SELECT model_id,requests,success,fail,total_latency_ms,last_latency_ms,prompt_tokens,completion_tokens,updated_at FROM model_stats ORDER BY model_id`)
	if err != nil {
		return fmt.Errorf("backup: dump model_stats: %w", err)
	}
	defer rows.Close()
	var buf bytes.Buffer
	for rows.Next() {
		var r statsRow
		if err := rows.Scan(&r.ModelID, &r.Requests, &r.Success, &r.Fail, &r.TotalLatencyMs, &r.LastLatencyMs, &r.PromptTokens, &r.CompletionTokens, &r.UpdatedAt); err != nil {
			return err
		}
		if err := writeJSONL(&buf, r); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	out[fileModelStats] = buf.Bytes()
	return nil
}

func dumpMeta(ctx context.Context, st *store.Store, out map[string][]byte) error {
	rows, err := st.DB().QueryContext(ctx, `SELECT key,value FROM app_meta ORDER BY key`)
	if err != nil {
		return fmt.Errorf("backup: dump app_meta: %w", err)
	}
	defer rows.Close()
	var buf bytes.Buffer
	for rows.Next() {
		var r metaRow
		if err := rows.Scan(&r.Key, &r.Value); err != nil {
			return err
		}
		if err := writeJSONL(&buf, r); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	out[fileAppMeta] = buf.Bytes()
	return nil
}

// replaceTables 在单事务内清空并写入全部数据表。
func replaceTables(ctx context.Context, st *store.Store, sessions []sessionRow, messages []messageRow, stats []statsRow, metas []metaRow) error {
	tx, err := st.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, q := range []string{`DELETE FROM messages`, `DELETE FROM sessions`, `DELETE FROM model_stats`, `DELETE FROM app_meta`} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("backup: clear: %w", err)
		}
	}
	for _, r := range sessions {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sessions(id,title,model,created_at,updated_at) VALUES(?,?,?,?,?)`,
			r.ID, r.Title, r.Model, r.CreatedAt, r.UpdatedAt); err != nil {
			return fmt.Errorf("backup: insert session %s: %w", r.ID, err)
		}
	}
	for _, r := range messages {
		if _, err := tx.ExecContext(ctx, `INSERT INTO messages(id,session_id,role,content_cipher,model,created_at) VALUES(?,?,?,?,?,?)`,
			r.ID, r.SessionID, r.Role, r.ContentCipher, r.Model, r.CreatedAt); err != nil {
			return fmt.Errorf("backup: insert message %s: %w", r.ID, err)
		}
	}
	for _, r := range stats {
		if _, err := tx.ExecContext(ctx, `INSERT INTO model_stats(model_id,requests,success,fail,total_latency_ms,last_latency_ms,prompt_tokens,completion_tokens,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			r.ModelID, r.Requests, r.Success, r.Fail, r.TotalLatencyMs, r.LastLatencyMs, r.PromptTokens, r.CompletionTokens, r.UpdatedAt); err != nil {
			return fmt.Errorf("backup: insert stats %s: %w", r.ModelID, err)
		}
	}
	for _, r := range metas {
		if _, err := tx.ExecContext(ctx, `INSERT INTO app_meta(key,value) VALUES(?,?)`, r.Key, r.Value); err != nil {
			return fmt.Errorf("backup: insert meta %s: %w", r.Key, err)
		}
	}
	return tx.Commit()
}

// —— 小工具 ——

func writeJSONL(buf *bytes.Buffer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	buf.Write(b)
	buf.WriteByte('\n')
	return nil
}

// parseJSONL 逐行解码 JSONL 到 []T；空内容返回空切片。
func parseJSONL[T any](b []byte) ([]T, error) {
	var out []T
	for _, line := range bytes.Split(b, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var v T
		if err := json.Unmarshal(line, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func writeZipFile(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
