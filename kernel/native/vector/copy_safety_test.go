package vector

// P0 回归：Index 按值复制后共享同一 owner，复制体与原体操作同一底层，Close 幂等，无双释放 panic。

import "testing"

func TestIndexValueCopySafe(t *testing.T) {
	ix, err := NewIndex(4)
	if err != nil { t.Fatal(err) }
	cp := *ix // 按值复制（旧实现会复制 mutex/指针/closed → 双句柄）

	// 通过副本插入，原体应可见（共享 owner）
	if err := cp.Insert(1, []int8{1, 0, 0, 0}); err != nil { t.Fatalf("copy insert: %v", err) }
	if ix.Len() != 1 { t.Fatalf("共享 owner：原体应见 1 条，实际 %d", ix.Len()) }

	// 副本搜索、原体序列化都可正常工作
	if _, err := cp.Search([]int8{1, 0, 0, 0}, 1); err != nil { t.Fatalf("copy search: %v", err) }
	if _, err := ix.Serialize(); err != nil { t.Fatalf("orig serialize: %v", err) }

	// 两处 Close 均幂等，不得 panic / 双释放
	if err := cp.Close(); err != nil { t.Fatalf("copy close: %v", err) }
	if err := ix.Close(); err != nil { t.Fatalf("orig close: %v", err) }
	if ix.Len() != 0 { t.Fatalf("关闭后 Len 应为 0") }
}
