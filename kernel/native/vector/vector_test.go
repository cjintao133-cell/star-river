package vector

import (
	"errors"
	"testing"
)

func TestNewInvalidDim(t *testing.T) {
	if _, err := NewIndex(0); !errors.Is(err, ErrBadDim) {
		t.Errorf("dim0 err = %v", err)
	}
	if _, err := NewIndex(-3); !errors.Is(err, ErrBadDim) {
		t.Errorf("dim-3 err = %v", err)
	}
}

func TestInsertSearchOrder(t *testing.T) {
	ix, err := NewIndex(4)
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	must := func(id int64, v []int8) {
		if err := ix.Insert(id, v); err != nil {
			t.Fatalf("insert %d: %v", id, err)
		}
	}
	must(10, []int8{1, 0, 0, 0})
	must(20, []int8{0, 1, 0, 0})
	must(30, []int8{1, 1, 0, 0})
	if ix.Len() != 3 {
		t.Fatalf("len = %d", ix.Len())
	}
	hits, err := ix.Search([]int8{1, 0, 0, 0}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 || hits[0].ID != 10 || hits[1].ID != 30 || hits[2].ID != 20 {
		t.Fatalf("hits = %+v", hits)
	}
	if !(hits[0].Score > hits[1].Score && hits[1].Score > hits[2].Score) {
		t.Errorf("scores not descending: %+v", hits)
	}
	// top1
	top1, _ := ix.Search([]int8{0, 1, 0, 0}, 1)
	if len(top1) != 1 || top1[0].ID != 20 {
		t.Errorf("top1 for q=[0,1,0,0] = %+v", top1)
	}
}

func TestPersistRestartLoad(t *testing.T) {
	ix, _ := NewIndex(4)
	ix.Insert(10, []int8{1, 0, 0, 0})
	ix.Insert(20, []int8{0, 1, 0, 0})
	ix.Insert(30, []int8{1, 1, 0, 0})
	blob, err := ix.Serialize()
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	ix.Close()

	// 模拟重启：从字节重建。
	ix2, err := Deserialize(blob)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	defer ix2.Close()
	if ix2.Dim() != 4 {
		t.Errorf("restored dim = %d, want 4", ix2.Dim())
	}
	if ix2.Len() != 3 {
		t.Errorf("restored len = %d", ix2.Len())
	}
	hits, _ := ix2.Search([]int8{1, 0, 0, 0}, 3)
	if len(hits) != 3 || hits[0].ID != 10 || hits[1].ID != 30 || hits[2].ID != 20 {
		t.Errorf("restored search = %+v", hits)
	}
}

func TestEmptyAndBadInput(t *testing.T) {
	ix, _ := NewIndex(4)
	defer ix.Close()
	h, err := ix.Search([]int8{1, 0, 0, 0}, 5)
	if err != nil || len(h) != 0 {
		t.Errorf("empty search = %+v, %v", h, err)
	}
	if err := ix.Insert(1, []int8{1, 0, 0}); !errors.Is(err, ErrBadDim) {
		t.Errorf("bad-dim insert err = %v", err)
	}
	if _, err := ix.Search([]int8{1, 0}, 2); !errors.Is(err, ErrBadDim) {
		t.Errorf("bad-dim search err = %v", err)
	}
	if hits, _ := ix.Search([]int8{1, 0, 0, 0}, 0); hits != nil {
		t.Errorf("k=0 should be nil")
	}
}

func TestClosedErrors(t *testing.T) {
	ix, _ := NewIndex(4)
	ix.Close()
	ix.Close() // 幂等
	if err := ix.Insert(1, []int8{1, 0, 0, 0}); !errors.Is(err, ErrClosed) {
		t.Errorf("insert closed err = %v", err)
	}
	if _, err := ix.Search([]int8{1, 0, 0, 0}, 1); !errors.Is(err, ErrClosed) {
		t.Errorf("search closed err = %v", err)
	}
	if _, err := ix.Serialize(); !errors.Is(err, ErrClosed) {
		t.Errorf("serialize closed err = %v", err)
	}
	if ix.Len() != 0 {
		t.Errorf("closed Len = %d", ix.Len())
	}
}

func TestDeserializeGarbage(t *testing.T) {
	if _, err := Deserialize(nil); err == nil {
		t.Error("nil 应报错")
	}
	if _, err := Deserialize([]byte("XXXXgarbage")); err == nil {
		t.Error("坏 magic 应报错")
	}
}
