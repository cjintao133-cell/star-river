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

// Index int8 量化向量索引（并发安全：内部互斥串行化 cgo 调用）。
type Index struct {
	mu     sync.Mutex
	dim    int
	p      *C.SrVecIndex
	closed bool
}

// NewIndex 建索引（dim>0）。
func NewIndex(dim int) (*Index, error) {
	if dim <= 0 {
		return nil, ErrBadDim
	}
	p := C.sr_vec_index_create(C.int(dim))
	if p == nil {
		return nil, ErrC
	}
	ix := &Index{dim: dim, p: p}
	runtime.SetFinalizer(ix, (*Index).finalize)
	return ix, nil
}

// finalize 兜底释放（忘记 Close 时）；REVIEW-NEEDED 句柄生命周期。
func (ix *Index) finalize() { ix.Close() }

// Close 释放底层索引；幂等。
func (ix *Index) Close() error {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if ix.closed {
		return nil
	}
	if ix.p != nil {
		C.sr_vec_index_free(ix.p)
		ix.p = nil
	}
	ix.closed = true
	runtime.SetFinalizer(ix, nil)
	return nil
}

// Len 当前条数。
func (ix *Index) Len() int {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if ix.closed {
		return 0
	}
	return int(C.sr_vec_index_count(ix.p))
}

// Dim 维度。
func (ix *Index) Dim() int { return ix.dim }

// Insert 插入一条 int8 向量（长度须等于 dim）。
func (ix *Index) Insert(id int64, vec []int8) error {
	if len(vec) != ix.dim {
		return ErrBadDim
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if ix.closed {
		return ErrClosed
	}
	// 复制到 C 内存（C 不持 Go 指针）。
	cb := C.CBytes(int8ToBytes(vec))
	defer C.free(cb)
	if C.sr_vec_index_insert(ix.p, C.int64_t(id), (*C.int8_t)(cb)) != 0 {
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
	if len(query) != ix.dim {
		return nil, ErrBadDim
	}
	if k <= 0 {
		return nil, nil
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if ix.closed {
		return nil, ErrClosed
	}
	qb := C.CBytes(int8ToBytes(query))
	defer C.free(qb)

	// 出参 C 缓冲区（C 写入，拷回后 free）。
	idsMem := C.malloc(C.size_t(k) * C.size_t(unsafe.Sizeof(C.int64_t(0))))
	scoresMem := C.malloc(C.size_t(k) * C.size_t(unsafe.Sizeof(C.float(0))))
	defer C.free(idsMem)
	defer C.free(scoresMem)

	n := int(C.sr_vec_index_search(ix.p, (*C.int8_t)(qb), C.int(k),
		(*C.int64_t)(idsMem), (*C.float)(scoresMem)))
	if n <= 0 {
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
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if ix.closed {
		return nil, ErrClosed
	}
	var buf *C.uint8_t
	var ln C.size_t
	if C.sr_vec_index_serialize(ix.p, &buf, &ln) != 0 {
		return nil, ErrC
	}
	defer C.sr_free(unsafe.Pointer(buf))
	return C.GoBytes(unsafe.Pointer(buf), C.int(ln)), nil
}

// Deserialize 从字节重建索引（重启加载用）。
func Deserialize(data []byte) (*Index, error) {
	if len(data) == 0 {
		return nil, ErrC
	}
	cb := C.CBytes(data)
	defer C.free(cb)
	p := C.sr_vec_index_deserialize((*C.uint8_t)(cb), C.size_t(len(data)))
	if p == nil {
		return nil, ErrC
	}
	ix := &Index{p: p, dim: dimFromSerialized(data)}
	runtime.SetFinalizer(ix, (*Index).finalize)
	return ix, nil
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
