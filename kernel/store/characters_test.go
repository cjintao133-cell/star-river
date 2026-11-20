package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func newStoreWithChars(t *testing.T) (*Store, *CharacterRepo) {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	repo, err := NewCharacterRepo(st)
	if err != nil {
		t.Fatal(err)
	}
	return st, repo
}

func TestCharacterCRUD(t *testing.T) {
	ctx := context.Background()
	_, repo := newStoreWithChars(t)

	c := Character{
		ID: "c1", Name: "示例角色", Description: "示例角色", Personality: "温和", Scenario: "示例",
		FirstMes: "哼", AlternateGreetings: []string{"a", "b"}, MesExample: "e.g.",
		SystemPrompt: "sp", PostHistoryInstructions: "phi",
	}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	// Get
	got, err := repo.Get(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "示例角色" || got.Personality != "温和" || len(got.AlternateGreetings) != 2 || got.AlternateGreetings[1] != "b" {
		t.Fatalf("Get 不对: %+v", got)
	}
	if got.CreatedAt == 0 || got.UpdatedAt == 0 {
		t.Error("时间戳应填")
	}
	// List（不含 blob）
	list, err := repo.List(ctx)
	if err != nil || len(list) != 1 || list[0].ID != "c1" {
		t.Fatalf("List 不对: %+v %v", list, err)
	}
	// Update
	got.Name = "示例角色改"
	got.AlternateGreetings = []string{"x"}
	if err := repo.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	got2, _ := repo.Get(ctx, "c1")
	if got2.Name != "示例角色改" || len(got2.AlternateGreetings) != 1 {
		t.Errorf("Update 不对: %+v", got2)
	}
	// Delete
	if err := repo.Delete(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, "c1"); !errors.Is(err, ErrCharacterNotFound) {
		t.Errorf("删除后应 NotFound, got %v", err)
	}
}

func TestCharacterAvatarPersona(t *testing.T) {
	ctx := context.Background()
	_, repo := newStoreWithChars(t)
	_ = repo.Create(ctx, Character{ID: "c1", Name: "n"})
	if err := repo.SetAvatar(ctx, "c1", []byte("AVATAR")); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetPersonaImage(ctx, "c1", []byte("PERSONA")); err != nil {
		t.Fatal(err)
	}
	c, _ := repo.Get(ctx, "c1")
	if string(c.Avatar) != "AVATAR" || string(c.PersonaImage) != "PERSONA" {
		t.Errorf("头像/人设图未存: %q %q", c.Avatar, c.PersonaImage)
	}
	// List 不含 blob
	list, _ := repo.List(ctx)
	if len(list[0].Avatar) != 0 {
		t.Error("List 不应含 avatar")
	}
	// 更新不存在角色
	if err := repo.SetAvatar(ctx, "ghost", []byte("x")); !errors.Is(err, ErrCharacterNotFound) {
		t.Errorf("不存在应 NotFound, got %v", err)
	}
}

func TestCharacterErrors(t *testing.T) {
	ctx := context.Background()
	_, repo := newStoreWithChars(t)
	if err := repo.Create(ctx, Character{ID: "", Name: "x"}); !errors.Is(err, ErrInvalidCharacter) {
		t.Error("空 id 应 ErrInvalidCharacter")
	}
	if err := repo.Create(ctx, Character{ID: "x", Name: ""}); !errors.Is(err, ErrInvalidCharacter) {
		t.Error("空 name 应 ErrInvalidCharacter")
	}
	if _, err := repo.Get(ctx, "nope"); !errors.Is(err, ErrCharacterNotFound) {
		t.Error("Get 不存在应 NotFound")
	}
	if err := repo.Update(ctx, Character{ID: "nope", Name: "x"}); !errors.Is(err, ErrCharacterNotFound) {
		t.Error("Update 不存在应 NotFound")
	}
	if err := repo.Delete(ctx, "nope"); !errors.Is(err, ErrCharacterNotFound) {
		t.Error("Delete 不存在应 NotFound")
	}
	if _, err := NewCharacterRepo(nil); err == nil {
		t.Error("nil store 应报错")
	}
}

func TestCharactersMigrationV4(t *testing.T) {
	st, _ := newStoreWithChars(t)
	v, err := st.SchemaVersion()
	if err != nil || v < 4 {
		t.Errorf("应迁移到 >= v4, got %d %v", v, err)
	}
	// sessions 表应有 character_id 列（ALTER 生效）
	rows, err := st.DB().Query(`PRAGMA table_info(sessions)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if name == "character_id" {
			found = true
		}
	}
	if !found {
		t.Error("sessions 应有 character_id 列")
	}
}

func TestCharacterBookRoundtrip(t *testing.T) {
	ctx := context.Background()
	_, repo := newStoreWithChars(t)
	_ = repo.Create(ctx, Character{ID: "c", Name: "n", CharacterBook: []byte(`{"entries":[{"keys":["a"]}]}`)})
	c, _ := repo.Get(ctx, "c")
	if c.CharacterBook == nil || !containsSub(string(c.CharacterBook), "entries") {
		t.Errorf("世界书往返不对: %s", c.CharacterBook)
	}
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
