// Package subagent 提供轻量子代理：把一个子任务放进"隔离上下文"里执行，避免把父对话的
// 全部历史灌进去撑爆预算；并支持并行跑多个子代理（有并发上限与 panic 隔离）。
//
// 这是"子代理/Agent Teams"的单机轻量版：不引入多进程，只在上下文与执行层做隔离编排。
package subagent

import (
	"context"
	"fmt"
	"sync"

	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

// Spec 一个子代理规格。
type Spec struct {
	Name   string   `json:"name"`
	System string   `json:"system"` // 子代理系统提示（角色/边界）
	Goal   string   `json:"goal"`   // 子任务目标
	Tools  []string `json:"tools,omitempty"`
	Digest string   `json:"digest,omitempty"` // 父上下文摘要（可选，替代完整历史）
}

// BuildMessages 构造子代理的初始消息：隔离的 system + 单条 user（目标）。
// 关键：不携带父对话历史（digest 只放摘要），从而隔离上下文、控制成本。
func BuildMessages(spec Spec) []types.Message {
	sys := spec.System
	if sys == "" {
		sys = "你是一个专注的子代理，只完成被指派的具体子任务。"
	}
	if spec.Digest != "" {
		sys += "\n\n【父任务背景摘要】\n" + spec.Digest
	}
	user := spec.Goal
	if user == "" {
		user = spec.Name
	}
	return []types.Message{
		{Role: types.RoleSystem, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: sys}}},
		{Role: types.RoleUser, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: user}}},
	}
}

// RunFunc 执行一个子代理，返回结果文本。
type RunFunc func(ctx context.Context, spec Spec) (string, error)

// Result 单个子代理执行结果。
type Result struct {
	Name   string `json:"name"`
	Output string `json:"output,omitempty"`
	Err    string `json:"err,omitempty"`
}

// RunParallel 并行执行多个子代理；limit<=0 默认 3。panic 被隔离成 err。
// 返回顺序与输入一致。
func RunParallel(ctx context.Context, specs []Spec, run RunFunc, limit int) []Result {
	if limit <= 0 {
		limit = 3
	}
	results := make([]Result, len(specs))
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i := range specs {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = runOne(ctx, specs[idx], run)
		}(i)
	}
	wg.Wait()
	return results
}

func runOne(ctx context.Context, spec Spec, run RunFunc) (res Result) {
	res.Name = spec.Name
	defer func() {
		if p := recover(); p != nil {
			res.Err = fmt.Sprint("panic: ", p)
		}
	}()
	out, err := run(ctx, spec)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	res.Output = out
	return res
}
