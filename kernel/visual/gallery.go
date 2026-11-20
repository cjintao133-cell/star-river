package visual

import (
	"sync"
	"time"
)

// GalleryItem 画廊条目（生成物 + 元数据；Data 为已加水印的图像字节）。
type GalleryItem struct {
	ID          string
	JobID       string
	MIME        string
	Data        []byte
	Watermarked bool
	Meta        map[string]any
	CreatedAt   int64 // UnixMilli
}

// Gallery 内存画廊（UI 押后；此为核心数据模型，持久化/界面后续接入）。
// 仅经 Service 流水线写入（水印已强制）。并发安全。
type Gallery struct {
	mu    sync.RWMutex
	items map[string]*GalleryItem
	order []string
}

// NewGallery 空画廊。
func NewGallery() *Gallery { return &Gallery{items: make(map[string]*GalleryItem)} }

// Add 加入一条条目，返回其 id。Watermarked 由调用方（Service）保证为 true。
func (g *Gallery) Add(it GalleryItem) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if it.CreatedAt == 0 {
		it.CreatedAt = time.Now().UnixMilli()
	}
	cp := it
	g.items[it.ID] = &cp
	g.order = append(g.order, it.ID)
	return it.ID
}

// Get 取条目副本（含 Data）。
func (g *Gallery) Get(id string) (GalleryItem, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	it, ok := g.items[id]
	if !ok {
		return GalleryItem{}, false
	}
	return *it, true
}

// List 返回全部条目元数据（不含 Data，节省内存；按加入顺序）。
func (g *Gallery) List() []GalleryItem {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]GalleryItem, 0, len(g.order))
	for _, id := range g.order {
		it := g.items[id]
		out = append(out, GalleryItem{
			ID: it.ID, JobID: it.JobID, MIME: it.MIME,
			Watermarked: it.Watermarked, Meta: it.Meta, CreatedAt: it.CreatedAt,
		})
	}
	return out
}

// Len 条目数。
func (g *Gallery) Len() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.items)
}
