package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/cjintao133-cell/star-river/kernel/core/types"
	"github.com/cjintao133-cell/star-river/kernel/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func newRepo(t *testing.T, st *store.Store) *store.Repo {
	t.Helper()
	cipher, err := store.NewFieldCipher(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	r, err := store.NewRepo(st, cipher)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// populate 造会话/消息/统计/配置。
func populate(t *testing.T, st *store.Store) {
	t.Helper()
	ctx := context.Background()
	repo := newRepo(t, st)
	if err := repo.CreateSession(ctx, store.Session{ID: "s1", Title: "会话一", Model: "m1", CreatedAt: 100, UpdatedAt: 200}); err != nil {
		t.Fatal(err)
	}
	msgs := []types.Message{
		{ID: "m1", SessionID: "s1", Role: types.RoleUser, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: "你好"}}, CreatedAt: 101},
		{ID: "m2", SessionID: "s1", Role: types.RoleAssistant, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: "在的"}}, Model: "m1", CreatedAt: 102},
	}
	for _, m := range msgs {
		if _, err := repo.AppendMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	sr, _ := store.NewStatsRepo(st)
	if err := sr.Record(ctx, "m1", true, 120, 10, 20); err != nil {
		t.Fatal(err)
	}
	if err := sr.Record(ctx, "m1", false, 80, 5, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Exec(ctx, `INSERT INTO app_meta(key,value) VALUES(?,?)`, "theme", "dark"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Exec(ctx, `INSERT INTO app_meta(key,value) VALUES(?,?)`, "lang", "zh"); err != nil {
		t.Fatal(err)
	}
}

func TestBackupRoundtrip(t *testing.T) {
	ctx := context.Background()
	stA := newStore(t)
	populate(t, stA)

	var buf bytes.Buffer
	man, err := WriteArchive(ctx, stA, &buf, Options{ExportedAt: 12345})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if man.Format != Format || man.Version != FormatVersion || man.ExportedAt != 12345 {
		t.Errorf("manifest 不对: %+v", man)
	}
	if len(man.Files) != 4 {
		t.Errorf("应有 4 个数据文件, got %d", len(man.Files))
	}

	stB := newStore(t)
	res, err := ReadArchive(ctx, stB, buf.Bytes())
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Manifest.Version != FormatVersion {
		t.Errorf("import manifest 不对: %+v", res.Manifest)
	}

	// 强断言：从 stB 再次导出，四个数据文件应与 stA 的导出**逐字节相同**（逐项一致）。
	var buf2 bytes.Buffer
	if _, err := WriteArchive(ctx, stB, &buf2, Options{}); err != nil {
		t.Fatalf("re-export: %v", err)
	}
	f1 := readZipMap(t, buf.Bytes())
	f2 := readZipMap(t, buf2.Bytes())
	for _, name := range []string{fileSessions, fileMessages, fileModelStats, fileAppMeta} {
		if !bytes.Equal(f1[name], f2[name]) {
			t.Errorf("数据文件 %s 往返后不一致\nA=%s\nB=%s", name, f1[name], f2[name])
		}
	}

	// 功能可用性：导入后能正常解密读出会话与消息。
	repoB := newRepo(t, stB)
	got, err := repoB.GetSession(ctx, "s1")
	if err != nil || got.Title != "会话一" || got.Model != "m1" {
		t.Errorf("会话不可读: %+v %v", got, err)
	}
	if m, err := repoB.GetMessage(ctx, "m2"); err != nil || m.Message.Blocks[0].Text != "在的" {
		t.Errorf("消息不可解密读出: %+v %v", m, err)
	}
	// 统计一致
	srB, _ := store.NewStatsRepo(stB)
	stt, ok, err := srB.Get(ctx, "m1")
	if err != nil || !ok || stt.Requests != 2 || stt.Success != 1 || stt.Fail != 1 || stt.TotalLatencyMs != 200 {
		t.Errorf("统计不一致: %+v %v", stt, err)
	}
	// 配置一致
	var v string
	if err := stB.DB().QueryRow(`SELECT value FROM app_meta WHERE key='theme'`).Scan(&v); err != nil || v != "dark" {
		t.Errorf("app_meta 不一致: %q %v", v, err)
	}
}

func TestBackupReplacesExisting(t *testing.T) {
	// 导入是替换式：目标库原有数据应被清掉。
	ctx := context.Background()
	stA := newStore(t)
	populate(t, stA)
	var buf bytes.Buffer
	if _, err := WriteArchive(ctx, stA, &buf, Options{}); err != nil {
		t.Fatal(err)
	}

	stB := newStore(t)
	repoB := newRepo(t, stB)
	_ = repoB.CreateSession(ctx, store.Session{ID: "old", Title: "旧", CreatedAt: 1, UpdatedAt: 1})
	if _, err := ReadArchive(ctx, stB, buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	if _, err := repoB.GetSession(ctx, "old"); err == nil {
		t.Error("替换式导入后旧会话应被清除")
	}
	if _, err := repoB.GetSession(ctx, "s1"); err != nil {
		t.Error("导入后新会话应存在")
	}
}

func TestBackupExtras(t *testing.T) {
	ctx := context.Background()
	stA := newStore(t)
	populate(t, stA)
	keyBlob := []byte("ENCRYPTED-KEY-MATERIAL")
	var buf bytes.Buffer
	if _, err := WriteArchive(ctx, stA, &buf, Options{Extras: map[string][]byte{"keys.enc": keyBlob}}); err != nil {
		t.Fatal(err)
	}
	stB := newStore(t)
	res, err := ReadArchive(ctx, stB, buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(res.Extras["keys.enc"], keyBlob) {
		t.Errorf("额外 key 文件应原样往返: %q", res.Extras["keys.enc"])
	}
}

// —— 篡改/版本/损坏 ——

func readZipMap(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	m := map[string][]byte{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		m[f.Name] = b
	}
	return m
}

func buildZip(t *testing.T, m map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range m {
		w, _ := zw.Create(name)
		_, _ = w.Write(data)
	}
	_ = zw.Close()
	return buf.Bytes()
}

func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func TestBackupVersionMismatch(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	data := map[string][]byte{fileSessions: {}, fileMessages: {}, fileModelStats: {}, fileAppMeta: {}}
	man := Manifest{Format: Format, Version: 999, Files: map[string]string{
		fileSessions: sha(data[fileSessions]), fileMessages: sha(data[fileMessages]),
		fileModelStats: sha(data[fileModelStats]), fileAppMeta: sha(data[fileAppMeta]),
	}}
	mb, _ := json.Marshal(man)
	data[fileManifest] = mb
	_, err := ReadArchive(ctx, st, buildZip(t, data))
	if !errors.Is(err, ErrVersionMismatch) {
		t.Fatalf("应报 ErrVersionMismatch, got %v", err)
	}
}

func TestBackupCorruptChecksum(t *testing.T) {
	ctx := context.Background()
	stA := newStore(t)
	populate(t, stA)
	var buf bytes.Buffer
	if _, err := WriteArchive(ctx, stA, &buf, Options{}); err != nil {
		t.Fatal(err)
	}
	files := readZipMap(t, buf.Bytes())
	files[fileSessions] = append(files[fileSessions], []byte("TAMPERED")...)
	stB := newStore(t)
	_, err := ReadArchive(ctx, stB, buildZip(t, files))
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("篡改应报 ErrCorrupt, got %v", err)
	}
}

func TestBackupMissingManifest(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	_, err := ReadArchive(ctx, st, buildZip(t, map[string][]byte{fileSessions: {}}))
	if !errors.Is(err, ErrBadArchive) {
		t.Fatalf("无清单应报 ErrBadArchive, got %v", err)
	}
}

func TestBackupBadFormat(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	data := map[string][]byte{fileSessions: {}, fileMessages: {}, fileModelStats: {}, fileAppMeta: {}}
	man := Manifest{Format: "some-other-app", Version: FormatVersion, Files: map[string]string{
		fileSessions: sha(data[fileSessions]), fileMessages: sha(data[fileMessages]),
		fileModelStats: sha(data[fileModelStats]), fileAppMeta: sha(data[fileAppMeta]),
	}}
	mb, _ := json.Marshal(man)
	data[fileManifest] = mb
	_, err := ReadArchive(ctx, st, buildZip(t, data))
	if !errors.Is(err, ErrBadArchive) {
		t.Fatalf("错误 format 应报 ErrBadArchive, got %v", err)
	}
}

func TestBackupNotZip(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	_, err := ReadArchive(ctx, st, []byte("not a zip"))
	if !errors.Is(err, ErrBadArchive) {
		t.Fatalf("非 zip 应报 ErrBadArchive, got %v", err)
	}
}

func TestBackupEmptyDB(t *testing.T) {
	ctx := context.Background()
	stA := newStore(t)
	var buf bytes.Buffer
	if _, err := WriteArchive(ctx, stA, &buf, Options{}); err != nil {
		t.Fatal(err)
	}
	stB := newStore(t)
	if _, err := ReadArchive(ctx, stB, buf.Bytes()); err != nil {
		t.Fatalf("空库往返应成功: %v", err)
	}
}

func TestBackupReservedExtraName(t *testing.T) {
	// Extras 用保留文件名（manifest.json）应报错。
	st := newStore(t)
	var buf bytes.Buffer
	_, err := WriteArchive(context.Background(), st, &buf, Options{Extras: map[string][]byte{"manifest.json": []byte("x")}})
	if !errors.Is(err, ErrBadArchive) {
		t.Fatalf("保留文件名应报 ErrBadArchive, got %v", err)
	}
}
