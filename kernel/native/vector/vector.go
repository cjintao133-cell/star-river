// Package vector 是 native/vector C 静态库的 cgo 薄封装（T-M3-01）。
//
// 指针纪律（红线 7，REVIEW-NEEDED）：C 侧 SrVecIndex 是不透明句柄，
// 本包持句柄并在 New 时 SetFinalizer 兜底、提供显式 Close；C 从不持 Go 指针
// （入参经 C.CBytes 复制成 C 内存，出参 C.malloc 后拷回再 free）。
// 链接纪律：先 make -f native/Makefile vector 产出 build/native/<goos>-<goarch>/libvector.a。
package vector

/*
#cgo CFLAGS: -I${SRCDIR}/../../../native/vector
#cgo linux,amd64 LDFLAGS: ${SRCDIR}/../../../build/native/linux-amd64/libvector.a
#cgo linux,amd64 LDFLAGS: -lm
#cgo android,arm64 LDFLAGS: -lm ${SRCDIR}/../../../build/native/android-arm64/libvector.a
#cgo windows,amd64 LDFLAGS: ${SRCDIR}/../../../build/native/windows-amd64/libvector.a
#include <stdlib.h>
#include "vector.h"
*/
import "C"

import (
	"errors"
	"math"
	"runtime"
	"sync"
	"unsafe"
)

// 错误。
var (
	ErrClosed = errors.New("vector: index closed")
	ErrBadDim = errors.New("vector: dim mismatch or non-positive")
	ErrC      = errors.New("vector: native call failed")
)

// vecOwner 共享所有权状态：锁/句柄/关闭标记/finalizer 全集中于此。
// Index 只持一个 *vecOwner 指针；按值复制 Index 只复制指针 → 单一 owner，杜绝双句柄双释放
// （修复 astra 评审 P0：vector Index 按值复制导致两份锁/closed 指向同一 C 指针）。
type vecOwner struct {
	mu     sync.Mutex
	dim    int
	p      *C.SrVecIndex
	closed bool
}

// Index int8 量化向量索引（并发安全）。薄包装，按值复制安全（共享同一 vecOwner）。
type Index struct {
	o *vecOwner
}

// NewIndex 建索引（dim>0）。
func NewIndex(dim int) (*Index, error) {
	if dim <= 0 || dim > math.MaxInt32 {
		return nil, ErrBadDim
	}
	p := C.sr_vec_index_create(C.int(dim))
	if p == nil {
		return nil, ErrC
	}
	o := &vecOwner{dim: dim, p: p}
	runtime.SetFinalizer(o, (*vecOwner).finalize)
	return &Index{o: o}, nil
}

// finalize 兜底释放（owner 回收时）；REVIEW-NEEDED 句柄生命周期。
func (o *vecOwner) finalize() { o.close() }

func (o *vecOwner) close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return nil
	}
	if o.p != nil {
		C.sr_vec_index_free(o.p)
		o.p = nil
	}
	o.closed = true
	runtime.SetFinalizer(o, nil)
	return nil
}

// Close 释放底层索引；幂等。
func (ix *Index) Close() error { return ix.o.close() }

// Len 当前条数。
func (ix *Index) Len() int {
	o := ix.o
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return 0
	}
	return int(C.sr_vec_index_count(o.p))
}

// Dim 维度。
func (ix *Index) Dim() int { return ix.o.dim }

// Insert 插入一条 int8 向量（长度须等于 dim）。
func (ix *Index) Insert(id int64, vec []int8) error {
	o := ix.o
	if len(vec) != o.dim {
		return ErrBadDim
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return ErrClosed
	}
	// 复制到 C 内存（C 不持 Go 指针）。
	cb := C.CBytes(int8ToBytes(vec))
	defer C.free(cb)
	if C.sr_vec_index_insert(o.p, C.int64_t(id), (*C.int8_t)(cb)) != 0 {
		return ErrC
	}
	return nil
}

// Hit 一条检索结果。
type Hit struct {
	ID    int64
	Score float32
}

// Search 返回 top-k（余弦降序）。
func (ix *Index) Search(query []int8, k int) ([]Hit, error) {
	o := ix.o
	if len(query) != o.dim {
		return nil, ErrBadDim
	}
	if k <= 0 {
		return nil, nil
	}
	const maxK = 1024 // 业务检索上限：防止 k 过大导致 C.malloc 巨量分配/分配失败崩进程
	if k > maxK {
		k = maxK
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return nil, ErrClosed
	}
	qb := C.CBytes(int8ToBytes(query))
	defer C.free(qb)

	// 出参 C 缓冲区（C 写入，拷回后 free）。
	idsMem := C.malloc(C.size_t(k) * C.size_t(unsafe.Sizeof(C.int64_t(0))))
	scoresMem := C.malloc(C.size_t(k) * C.size_t(unsafe.Sizeof(C.float(0))))
	defer C.free(idsMem)
	defer C.free(scoresMem)

	n := int(C.sr_vec_index_search(o.p, (*C.int8_t)(qb), C.int(k),
		(*C.int64_t)(idsMem), (*C.float)(scoresMem)))
	if n < 0 || n > k {
		return nil, ErrC // 越界返回视为错误，防止用错误长度构造切片
	}
	if n == 0 {
		return nil, nil
	}
	ids := unsafe.Slice((*int64)(idsMem), n)
	scores := unsafe.Slice((*float32)(scoresMem), n)
	hits := make([]Hit, n)
	for i := 0; i < n; i++ {
		hits[i] = Hit{ID: ids[i], Score: scores[i]}
	}
	return hits, nil
}

// Serialize 序列化为字节（持久化用；由调用方存入 store/文件）。
func (ix *Index) Serialize() ([]byte, error) {
	o := ix.o
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return nil, ErrClosed
	}
	var buf *C.uint8_t
	var ln C.size_t
	if C.sr_vec_index_serialize(o.p, &buf, &ln) != 0 {
		return nil, ErrC
	}
	defer C.sr_free(unsafe.Pointer(buf))
	if ln > math.MaxInt32 {
		return nil, ErrC // 超出 C.int 可表示范围，避免 GoBytes 负长度崩溃/截断
	}
	if ln > 0 && buf == nil {
		return nil, ErrC
	}
	return C.GoBytes(unsafe.Pointer(buf), C.int(ln)), nil
}

// Deserialize 从字节重建索引（重启加载用）。
func Deserialize(data []byte) (*Index, error) {
	if len(data) < 8 {
		return nil, ErrC
	}
	if d := dimFromSerialized(data); d <= 0 || d > math.MaxInt32 {
		return nil, ErrC // 头部维度非法，拒绝整块数据交给 C
	}
	cb := C.CBytes(data)
	defer C.free(cb)
	p := C.sr_vec_index_deserialize((*C.uint8_t)(cb), C.size_t(len(data)))
	if p == nil {
		return nil, ErrC
	}
	o := &vecOwner{p: p, dim: dimFromSerialized(data)}
	runtime.SetFinalizer(o, (*vecOwner).finalize)
	return &Index{o: o}, nil
}

// dimFromSerialized 从序列化头解析维度（与 vector.c 格式一致：magic4|dim i32le|count i32le）。
func dimFromSerialized(data []byte) int {
	if len(data) < 8 {
		return 0
	}
	return int(int32(uint32(data[4]) | uint32(data[5])<<8 | uint32(data[6])<<16 | uint32(data[7])<<24))
}

// int8ToBytes 按位复制 int8→byte（不改数值）。
func int8ToBytes(v []int8) []byte {
	b := make([]byte, len(v))
	for i, x := range v {
		b[i] = byte(x)
	}
	return b
}
