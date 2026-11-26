package subagent

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestBuildMessagesIsolated(t *testing.T) {
	msgs := BuildMessages(Spec{Name: "子1", System: "你是检索子代理", Goal: "找资料", Digest: "父任务：写报告"})
	if len(msgs) != 2 {
		t.Fatalf("应 2 条消息: %d", len(msgs))
	}
	sys := msgs[0].Blocks[0].Text
	if !strings.Contains(sys, "检索子代理") || !strings.Contains(sys, "父任务：写报告") {
		t.Fatalf("system 应含角色+摘要: %q", sys)
	}
	if msgs[1].Blocks[0].Text != "找资料" {
		t.Fatalf("user 应为目标: %q", msgs[1].Blocks[0].Text)
	}
}

func TestBuildMessagesDefaults(t *testing.T) {
	msgs := BuildMessages(Spec{Name: "onlyname"})
	if len(msgs) != 2 || msgs[1].Blocks[0].Text != "onlyname" {
		t.Fatalf("空 goal 应回退 name: %+v", msgs)
	}
}

func TestRunParallelOrderAndPanic(t *testing.T) {
	specs := []Spec{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	run := func(ctx context.Context, s Spec) (string, error) {
		switch s.Name {
		case "a":
			return "A", nil
		case "b":
			panic("boom")
		case "c":
			return "", errors.New("cerr")
		}
		return "", nil
	}
	res := RunParallel(context.Background(), specs, run, 2)
	if len(res) != 3 {
		t.Fatalf("应 3 结果: %d", len(res))
	}
	if res[0].Output != "A" || res[0].Name != "a" {
		t.Fatalf("顺序/结果不对: %+v", res[0])
	}
	if !strings.Contains(res[1].Err, "panic") {
		t.Fatalf("panic 应隔离: %+v", res[1])
	}
	if res[2].Err != "cerr" {
		t.Fatalf("错误应记录: %+v", res[2])
	}
}

func TestRunParallelEmpty(t *testing.T) {
	res := RunParallel(context.Background(), nil, func(ctx context.Context, s Spec) (string, error) { return "", nil }, 0)
	if len(res) != 0 {
		t.Fatalf("空输入应空输出")
	}
}
