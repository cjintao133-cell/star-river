package visual

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ErrNoBackend 未配置后端。
var ErrNoBackend = errors.New("visual: backend required")

// JobStatus 任务状态。
type JobStatus string

const (
	JobQueued    JobStatus = "queued"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
	JobCanceled  JobStatus = "canceled"
)

// Job 异步生成任务。
type Job struct {
	ID        string
	RecipeID  string
	Prompt    string
	Status    JobStatus
	Error     string
	ItemIDs   []string
	CreatedAt int64 // UnixMilli
	UpdatedAt int64
}

func terminal(s JobStatus) bool {
	return s == JobSucceeded || s == JobFailed || s == JobCanceled
}

// Service 视觉管线：配方→请求→异步队列→后端生成→**强制水印**→画廊。
//
// AI 标识不可关：Service 持有 Watermarker（构造注入，零值可用），处理每个输出图像时
// 强制 wm.Mark；无任何"关闭水印"的 API。未加水印或加水印失败的图像不会进入画廊。
type Service struct {
	backend     ImageBackend
	wm          Watermarker
	gallery     *Gallery
	concurrency int

	mu     sync.Mutex
	jobs   map[string]*Job
	order  []string
	done   map[string]chan struct{}
	sem    chan struct{}
	seq    int64
	closed bool
}

// NewService 建管线。backend 必填；concurrency<=0 用 2。
func NewService(backend ImageBackend, wm Watermarker, concurrency int) (*Service, error) {
	if backend == nil {
		return nil, ErrNoBackend
	}
	if concurrency <= 0 {
		concurrency = 2
	}
	return &Service{
		backend:     backend,
		wm:          wm,
		gallery:     NewGallery(),
		concurrency: concurrency,
		jobs:        make(map[string]*Job),
		done:        make(map[string]chan struct{}),
		sem:         make(chan struct{}, concurrency),
	}, nil
}

// Gallery 暴露画廊（只读用途）。
func (s *Service) Gallery() *Gallery { return s.gallery }

// Backend 适配器名。
func (s *Service) Backend() string { return s.backend.Name() }

// newID 生成唯一 id（时间 + 原子序号，无需外部依赖）。
func (s *Service) newID(prefix string) string {
	n := atomic.AddInt64(&s.seq, 1)
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), n)
}

// Submit 提交生成任务（依据配方 + 输入构建请求），异步处理，返回 jobID。
func (s *Service) Submit(ctx context.Context, recipe Recipe, inputs map[string]string) (string, error) {
	req, err := recipe.BuildRequest(inputs)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return "", &APIError{Kind: "protocol", Message: "service closed"}
	}
	id := s.newID("job")
	job := &Job{
		ID: id, RecipeID: recipe.ID, Prompt: req.Prompt, Status: JobQueued,
		CreatedAt: time.Now().UnixMilli(), UpdatedAt: time.Now().UnixMilli(),
	}
	s.jobs[id] = job
	s.order = append(s.order, id)
	doneCh := make(chan struct{})
	s.done[id] = doneCh
	s.mu.Unlock()

	go s.run(ctx, id, req, doneCh)
	return id, nil
}

// run 执行单个任务：取并发令牌 → 原子置 Running → 生成 → 逐图强制水印 → 入画廊。
func (s *Service) run(ctx context.Context, id string, req GenerateRequest, doneCh chan struct{}) {
	defer close(doneCh)

	// 并发限流。
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		s.failOrCancel(id, ctx.Err())
		return
	}
	// 原子状态迁移：仅 Queued→Running；若已被取消/终态则放弃。
	if !s.beginRun(id) {
		return
	}

	res, err := s.backend.Generate(ctx, req)
	if err != nil {
		s.failOrCancel(id, err)
		return
	}
	// 生成期间被取消则不写画廊、不置成功。
	if s.statusOf(id) == JobCanceled {
		return
	}

	var itemIDs []string
	for _, raw := range res.Images {
		marked, mime, err := s.wm.Mark(raw) // 强制水印
		if err != nil {
			s.failOrCancel(id, fmt.Errorf("watermark: %w", err))
			return
		}
		itemID := s.newID("img")
		s.gallery.Add(GalleryItem{
			ID: itemID, JobID: id, MIME: mime, Data: marked, Watermarked: true,
			Meta: map[string]any{"model": res.Model, "prompt": req.Prompt},
		})
		itemIDs = append(itemIDs, itemID)
	}

	s.mu.Lock()
	job := s.jobs[id]
	if job != nil && job.Status == JobRunning {
		job.Status = JobSucceeded
		job.ItemIDs = itemIDs
		job.UpdatedAt = time.Now().UnixMilli()
	}
	s.mu.Unlock()
}

// beginRun 原子地把 Queued→Running；返回是否成功（false 表示已取消/非排队态）。
func (s *Service) beginRun(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.jobs[id]
	if j == nil || j.Status != JobQueued {
		return false
	}
	j.Status = JobRunning
	j.UpdatedAt = time.Now().UnixMilli()
	return true
}

// failOrCancel 区分取消与失败（不覆盖已终态）。
func (s *Service) failOrCancel(id string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.jobs[id]
	if j == nil || terminal(j.Status) {
		return
	}
	if errors.Is(err, context.Canceled) {
		j.Status = JobCanceled
	} else {
		j.Status = JobFailed
		j.Error = err.Error()
	}
	j.UpdatedAt = time.Now().UnixMilli()
}

// statusOf 读状态。
func (s *Service) statusOf(id string) JobStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j := s.jobs[id]; j != nil {
		return j.Status
	}
	return ""
}

// GetJob 任务快照。
func (s *Service) GetJob(id string) (Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return Job{}, false
	}
	cp := *j
	cp.ItemIDs = append([]string(nil), j.ItemIDs...)
	return cp, true
}

// ListJobs 全部任务（快照，按提交顺序）。
func (s *Service) ListJobs() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Job, 0, len(s.order))
	for _, id := range s.order {
		j := s.jobs[id]
		cp := *j
		cp.ItemIDs = append([]string(nil), j.ItemIDs...)
		out = append(out, cp)
	}
	return out
}

// Wait 阻塞直到任务进入终态或 ctx 结束。
func (s *Service) Wait(ctx context.Context, id string) (Job, error) {
	s.mu.Lock()
	ch, ok := s.done[id]
	s.mu.Unlock()
	if !ok {
		return Job{}, &APIError{Kind: "protocol", Message: "unknown job: " + id}
	}
	select {
	case <-ch:
		j, _ := s.GetJob(id)
		return j, nil
	case <-ctx.Done():
		return Job{}, ctx.Err()
	}
}

// Cancel 取消任务（排队中→直接取消；运行中→标记取消，尽力而为；已终态→无操作）。
func (s *Service) Cancel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.jobs[id]
	if j == nil {
		return &APIError{Kind: "protocol", Message: "unknown job: " + id}
	}
	if terminal(j.Status) {
		return nil
	}
	j.Status = JobCanceled
	j.UpdatedAt = time.Now().UnixMilli()
	return nil
}

// Close 关闭服务（不再接受新任务；在途任务继续）。
func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
}
