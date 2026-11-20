package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"star-river/kernel/core/types"

	"star-river/kernel/secret"
)

func newRepo(t *testing.T) (*Repo, *Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "repo.db")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	c, err := FieldCipherFromVault(secret.NewMemVault(), "store.fieldkey")
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewRepo(st, c)
	if err != nil {
		t.Fatal(err)
	}
	return r, st, path
}

func mkMsg(id, session string, role types.Role, text string, ts int64) types.Message {
	return types.Message{
		ID: id, SessionID: session, Role: role, CreatedAt: ts,
		Blocks: []types.ContentBlock{{Type: types.BlockText, Text: text}},
	}
}

func TestRepoRequiresCipher(t *testing.T) {
	st, _ := Open(filepath.Join(t.TempDir(), "x.db"))
	defer st.Close()
	if _, err := NewRepo(st, nil); !errors.Is(err, ErrNoCipher) {
		t.Errorf("nil cipher 应报 ErrNoCipher")
	}
}

func TestSessionCRUD(t *testing.T) {
	r, _, _ := newRepo(t)
	ctx := context.Background()
	if err := r.CreateSession(ctx, Session{ID: "s1", Title: "第一会话", Model: "claude"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	s, err := r.GetSession(ctx, "s1")
	if err != nil || s.Title != "第一会话" || s.Model != "claude" || s.CreatedAt == 0 {
		t.Fatalf("Get = %+v, %v", s, err)
	}
	if err := r.UpdateSessionTitle(ctx, "s1", "改名了"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	s, _ = r.GetSession(ctx, "s1")
	if s.Title != "改名了" {
		t.Errorf("title = %q", s.Title)
	}
	if err := r.DeleteSession(ctx, "s1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.GetSession(ctx, "s1"); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("删后 Get 应 ErrSessionNotFound, got %v", err)
	}
	// 不存在时更新/删除报 NotFound。
	if err := r.UpdateSessionTitle(ctx, "ghost", "x"); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("update ghost = %v", err)
	}
	if err := r.DeleteSession(ctx, "ghost"); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("delete ghost = %v", err)
	}
}

func TestAppendAndGetMessage(t *testing.T) {
	r, _, _ := newRepo(t)
	ctx := context.Background()
	r.CreateSession(ctx, Session{ID: "s1"})
	msg := mkMsg("m1", "s1", types.RoleUser, "你好示例角色", 1720000000000)
	seq, err := r.AppendMessage(ctx, msg)
	if err != nil || seq <= 0 {
		t.Fatalf("Append: seq=%d err=%v", seq, err)
	}
	rec, err := r.GetMessage(ctx, "m1")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if rec.Message.Role != types.RoleUser || len(rec.Message.Blocks) != 1 || rec.Message.Blocks[0].Text != "你好示例角色" {
		t.Errorf("解密消息不符: %+v", rec.Message)
	}
	if rec.Message.CreatedAt != 1720000000000 {
		t.Errorf("createdAt = %d", rec.Message.CreatedAt)
	}
	if _, err := r.GetMessage(ctx, "ghost"); !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("GetMessage ghost = %v", err)
	}
}

func TestAppendMessageValidation(t *testing.T) {
	r, _, _ := newRepo(t)
	ctx := context.Background()
	r.CreateSession(ctx, Session{ID: "s1"})
	cases := []types.Message{
		{ID: "", SessionID: "s1", Role: types.RoleUser},
		{ID: "m", SessionID: "", Role: types.RoleUser},
		{ID: "m", SessionID: "s1", Role: types.Role("bogus")},
	}
	for i, m := range cases {
		if _, err := r.AppendMessage(ctx, m); !errors.Is(err, ErrInvalidMessage) {
			t.Errorf("case %d err = %v, want ErrInvalidMessage", i, err)
		}
	}
	// 外键：会话不存在。
	if _, err := r.AppendMessage(ctx, mkMsg("mx", "no-such-session", types.RoleUser, "x", 0)); err == nil {
		t.Error("不存在会话应因外键失败")
	}
}

// keyset 分页：逐页取回全部消息，无重复无遗漏，顺序正确（最新在前）。
func TestListMessagesPagination(t *testing.T) {
	r, _, _ := newRepo(t)
	ctx := context.Background()
	r.CreateSession(ctx, Session{ID: "s1"})
	const total = 25
	for i := 0; i < total; i++ {
		if _, err := r.AppendMessage(ctx, mkMsg(fmt.Sprintf("m%02d", i), "s1", types.RoleUser, fmt.Sprintf("内容-%d", i), int64(1000+i))); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	seen := map[string]bool{}
	var order []string
	before := int64(0)
	pages := 0
	for {
		page, err := r.ListMessages(ctx, "s1", 7, before)
		if err != nil {
			t.Fatalf("ListMessages: %v", err)
		}
		pages++
		for _, rec := range page.Records {
			if seen[rec.Message.ID] {
				t.Fatalf("重复消息 %s", rec.Message.ID)
			}
			seen[rec.Message.ID] = true
			order = append(order, rec.Message.ID)
		}
		if !page.HasMore {
			break
		}
		before = page.NextBeforeSeq
	}
	if len(seen) != total {
		t.Fatalf("取回 %d 条, want %d", len(seen), total)
	}
	// 最新在前：第一条应是 m24，最后 m00。
	if order[0] != "m24" || order[len(order)-1] != "m00" {
		t.Errorf("顺序不符: 首 %s 尾 %s", order[0], order[len(order)-1])
	}
	if pages != 4 { // 25 条 / 7 = 4 页
		t.Errorf("pages = %d, want 4", pages)
	}
}

// 会话分页（offset/limit）。
func TestListSessionsPagination(t *testing.T) {
	r, _, _ := newRepo(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		r.CreateSession(ctx, Session{ID: fmt.Sprintf("s%d", i), UpdatedAt: int64(1000 + i)})
	}
	got, err := r.ListSessions(ctx, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "s4" || got[1].ID != "s3" {
		t.Errorf("第一页 = %+v", got)
	}
	got2, _ := r.ListSessions(ctx, 2, 2)
	if len(got2) != 2 || got2[0].ID != "s2" {
		t.Errorf("第二页 = %+v", got2)
	}
}

// 级联删除：删会话连带删其消息（外键 ON DELETE CASCADE 生效）。
func TestDeleteSessionCascades(t *testing.T) {
	r, st, _ := newRepo(t)
	ctx := context.Background()
	r.CreateSession(ctx, Session{ID: "s1"})
	r.CreateSession(ctx, Session{ID: "s2"})
	r.AppendMessage(ctx, mkMsg("a", "s1", types.RoleUser, "x", 0))
	r.AppendMessage(ctx, mkMsg("b", "s1", types.RoleUser, "y", 0))
	r.AppendMessage(ctx, mkMsg("c", "s2", types.RoleUser, "z", 0))

	if err := r.DeleteSession(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	// s1 的消息应全删，s2 的保留。
	n, _ := r.CountMessages(ctx, "s1")
	if n != 0 {
		t.Errorf("s1 消息残留 %d", n)
	}
	n2, _ := r.CountMessages(ctx, "s2")
	if n2 != 1 {
		t.Errorf("s2 消息 = %d, want 1", n2)
	}
	// 主键可直接查证（外键级联靠 PRAGMA foreign_keys=1）。
	var cnt int
	st.DB().QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id='s1'`).Scan(&cnt)
	if cnt != 0 {
		t.Errorf("DB 层 s1 messages = %d", cnt)
	}
}

// 验收核心：强杀/重启后数据完整——
// 写会话+多消息→Close（模拟进程退出）→重开同一库→会话/消息条数、顺序、解密内容全一致。
func TestRestartDataIntegrity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "restart.db")
	keyVault := secret.NewMemVault()
	// 注意：MemVault 进程内有效；真实场景主密钥在 Keystore/DPAPI。此处跨"重开"复用同一 key。
	ciph, _ := FieldCipherFromVault(keyVault, "store.fieldkey")

	// 第一次生命：写数据。
	st1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	r1, _ := NewRepo(st1, ciph)
	ctx := context.Background()
	r1.CreateSession(ctx, Session{ID: "sess-A", Title: "重要对话", Model: "claude"})
	texts := []string{"第一句", "第二句 secret-restart-内容", "第三句"}
	for i, txt := range texts {
		if _, err := r1.AppendMessage(ctx, mkMsg(fmt.Sprintf("msg-%d", i), "sess-A", types.RoleUser, txt, int64(2000+i))); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	if err := st1.Close(); err != nil { // 模拟进程退出
		t.Fatal(err)
	}

	// 第二次生命：重开同库。
	st2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer st2.Close()
	r2, _ := NewRepo(st2, ciph)

	s, err := r2.GetSession(ctx, "sess-A")
	if err != nil || s.Title != "重要对话" || s.Model != "claude" {
		t.Fatalf("重开后会话不符: %+v, %v", s, err)
	}
	page, err := r2.ListMessages(ctx, "sess-A", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != len(texts) {
		t.Fatalf("重开后消息数 = %d, want %d", len(page.Records), len(texts))
	}
	// 最新在前：msg-2, msg-1, msg-0
	for i, want := range []string{"第三句", "第二句 secret-restart-内容", "第一句"} {
		got := page.Records[i].Message.Blocks[0].Text
		if got != want {
			t.Errorf("重开后第 %d 条 = %q, want %q", i, got, want)
		}
	}
}

// 密文落盘：写含敏感明文的消息后，扫描库目录所有文件均无该明文；但能解密读回。
func TestRepoContentEncryptedOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "enc.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ciph, _ := FieldCipherFromVault(secret.NewMemVault(), "store.fieldkey")
	r, _ := NewRepo(st, ciph)
	ctx := context.Background()
	r.CreateSession(ctx, Session{ID: "s1"})
	plaintext := "用户的私密对话 very-secret-repo-content-xyz"
	if _, err := r.AppendMessage(ctx, mkMsg("m1", "s1", types.RoleUser, plaintext, 0)); err != nil {
		t.Fatal(err)
	}
	rec, err := r.GetMessage(ctx, "m1")
	if err != nil || rec.Message.Blocks[0].Text != plaintext {
		t.Fatalf("读回失败: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		if bytes.Contains(b, []byte(plaintext)) {
			t.Fatalf("红线4 违规：明文落盘于 %s", e.Name())
		}
	}
}

func TestNewRepoNilStore(t *testing.T) {
	c, _ := FieldCipherFromVault(secret.NewMemVault(), "k")
	if _, err := NewRepo(nil, c); err == nil {
		t.Error("nil store 应报错")
	}
}

// ListSessions 默认分页（limit<=0 / offset<0 归一）。
func TestListSessionsDefaults(t *testing.T) {
	r, _, _ := newRepo(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		r.CreateSession(ctx, Session{ID: fmt.Sprintf("d%d", i), UpdatedAt: int64(i)})
	}
	got, err := r.ListSessions(ctx, 0, -5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("默认分页应返回全部 3 条, got %d", len(got))
	}
}
