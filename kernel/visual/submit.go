package visual

import (
	"context"
	"time"
)

// SubmitRequest 直接提交一个已构建好的 GenerateRequest（用于需要注入运行时数据如垫图字节的场景）。
// recipeID 仅作任务元数据。异步处理，返回 jobID。
func (s *Service) SubmitRequest(ctx context.Context, req GenerateRequest, recipeID string) (string, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return "", &APIError{Kind: "protocol", Message: "service closed"}
	}
	id := s.newID("job")
	job := &Job{
		ID: id, RecipeID: recipeID, Prompt: req.Prompt, Status: JobQueued,
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
