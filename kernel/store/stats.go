package store

import (
	"context"
	"errors"
	"time"
)

// T-M3-07 模型本地统计：每模型请求数/成功/失败/延迟/token 本地累计（仅本地 SQLite）。
// 作为灾备路由（备用排序）与自动标灰（失败率超阈值）的数据源。

// ModelStats 某模型的累计统计。
type ModelStats struct {
	ModelID          string
	Requests         int64
	Success          int64
	Fail             int64
	TotalLatencyMs   int64
	LastLatencyMs    int64
	PromptTokens     int64
	CompletionTokens int64
	UpdatedAt        int64 // UnixMilli
}

// FailureRate 失败率（0..1）；无请求返回 0。
func (s ModelStats) FailureRate() float64 {
	if s.Requests <= 0 {
		return 0
	}
	return float64(s.Fail) / float64(s.Requests)
}

// AvgLatencyMs 平均延迟（ms）；无请求返回 0。
func (s ModelStats) AvgLatencyMs() float64 {
	if s.Requests <= 0 {
		return 0
	}
	return float64(s.TotalLatencyMs) / float64(s.Requests)
}

// StatsRepo 模型统计仓储（写经 Store 单写队列，原子 upsert）。
type StatsRepo struct {
	st  *Store
	now func() int64
}

// NewStatsRepo 建统计仓储。
func NewStatsRepo(st *Store) (*StatsRepo, error) {
	if st == nil {
		return nil, errors.New("store: nil store")
	}
	return &StatsRepo{st: st, now: func() int64 { return time.Now().UnixMilli() }}, nil
}

// Record 记一次请求结果（成功/失败、延迟、token）。原子 upsert 累加。
// modelID 空则报错（无意义统计）。
func (r *StatsRepo) Record(ctx context.Context, modelID string, ok bool, latencyMs, promptTok, completionTok int64) error {
	if modelID == "" {
		return errors.New("store: empty model id")
	}
	succ, fail := int64(0), int64(0)
	if ok {
		succ = 1
	} else {
		fail = 1
	}
	if latencyMs < 0 {
		latencyMs = 0
	}
	if promptTok < 0 {
		promptTok = 0
	}
	if completionTok < 0 {
		completionTok = 0
	}
	_, err := r.st.Exec(ctx, `
		INSERT INTO model_stats(model_id,requests,success,fail,total_latency_ms,last_latency_ms,prompt_tokens,completion_tokens,updated_at)
		VALUES(?,1,?,?,?,?,?,?,?)
		ON CONFLICT(model_id) DO UPDATE SET
			requests=requests+1,
			success=success+excluded.success,
			fail=fail+excluded.fail,
			total_latency_ms=total_latency_ms+excluded.total_latency_ms,
			last_latency_ms=excluded.last_latency_ms,
			prompt_tokens=prompt_tokens+excluded.prompt_tokens,
			completion_tokens=completion_tokens+excluded.completion_tokens,
			updated_at=excluded.updated_at`,
		modelID, succ, fail, latencyMs, latencyMs, promptTok, completionTok, r.now())
	return err
}

// Get 取某模型统计；不存在返回 ok=false, err=nil。
func (r *StatsRepo) Get(ctx context.Context, modelID string) (ModelStats, bool, error) {
	var s ModelStats
	rows, err := r.st.Query(ctx, `
		SELECT model_id,requests,success,fail,total_latency_ms,last_latency_ms,prompt_tokens,completion_tokens,updated_at
		FROM model_stats WHERE model_id=?`, modelID)
	if err != nil {
		return s, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		if e := rows.Err(); e != nil {
			return s, false, e
		}
		return s, false, nil
	}
	if err := rows.Scan(&s.ModelID, &s.Requests, &s.Success, &s.Fail, &s.TotalLatencyMs, &s.LastLatencyMs, &s.PromptTokens, &s.CompletionTokens, &s.UpdatedAt); err != nil {
		return s, false, err
	}
	return s, true, nil
}

// All 返回全部模型统计。
func (r *StatsRepo) All(ctx context.Context) ([]ModelStats, error) {
	rows, err := r.st.Query(ctx, `
		SELECT model_id,requests,success,fail,total_latency_ms,last_latency_ms,prompt_tokens,completion_tokens,updated_at
		FROM model_stats ORDER BY model_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ModelStats
	for rows.Next() {
		var s ModelStats
		if err := rows.Scan(&s.ModelID, &s.Requests, &s.Success, &s.Fail, &s.TotalLatencyMs, &s.LastLatencyMs, &s.PromptTokens, &s.CompletionTokens, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// FailureRate 便捷：某模型失败率（无记录返回 0,false）。
func (r *StatsRepo) FailureRate(ctx context.Context, modelID string) (float64, bool, error) {
	s, ok, err := r.Get(ctx, modelID)
	if err != nil || !ok {
		return 0, ok, err
	}
	return s.FailureRate(), true, nil
}
