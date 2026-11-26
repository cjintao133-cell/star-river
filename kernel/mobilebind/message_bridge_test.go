package mobilebind

import (
	"context"
	"testing"

	"github.com/cjintao133-cell/star-river/kernel/core/types"
	"github.com/cjintao133-cell/star-river/kernel/store"
)

// append → update（改文本）→ 读回 → delete → 读回失败。
func TestUpdateAndDeleteMessage(t *testing.T) {
	dir := t.TempDir()
	if r := InitKernel(dir+"/m.db", b64key()); r != "OK" {
		t.Fatalf("init: %s", r)
	}
	defer CloseKernel()
	sid := mustSession(t, CreateSession("", "编辑测试会话", "m"))
	mid := genMessageID()
	if _, err := kbRepo.AppendMessage(context.Background(), types.Message{
		ID: mid, SessionID: sid, Role: types.RoleUser,
		Blocks: []types.ContentBlock{{Type: types.BlockText, Text: "原始文本"}},
	}); err != nil {
		t.Fatalf("append: %v", err)
	}

	if r := UpdateMessage(sid, mid, "改后文本"); r != "OK" {
		t.Fatalf("update: %s", r)
	}
	rec, err := kbRepo.GetMessage(context.Background(), mid)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(rec.Message.Blocks) == 0 || rec.Message.Blocks[0].Text != "改后文本" {
		t.Fatalf("update not persisted: %+v", rec.Message.Blocks)
	}

	if r := UpdateMessage("nope", mid, "x"); len(r) < 4 || r[:4] != "ERR:" {
		t.Fatalf("expected ERR for wrong session, got %s", r)
	}

	if r := DeleteMessage(sid, mid); r != "OK" {
		t.Fatalf("delete: %s", r)
	}
	if _, err := kbRepo.GetMessage(context.Background(), mid); err != store.ErrMessageNotFound {
		t.Fatalf("expected not found after delete, got %v", err)
	}
	if r := DeleteMessage(sid, mid); len(r) < 4 || r[:4] != "ERR:" {
		t.Fatalf("expected ERR deleting twice, got %s", r)
	}
}
