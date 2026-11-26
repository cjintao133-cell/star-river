package codebrain

import (
	"context"
	"errors"
	"testing"
)

func mustCreate(t *testing.T, r *Runner) *Task {
	t.Helper()
	tk, err := r.Create(Task{Title: "办一件事", Goal: "完成目标"})
	if err != nil {
		t.Fatal(err)
	}
	return tk
}

func planFixed(titles ...string) PlanFunc {
	return func(ctx context.Context, t *Task) ([]string, error) { return titles, nil }
}

func stepOK(ctx context.Context, t *Task, s Step) (string, error) { return "done:" + s.Title, nil }

func TestPlanAndRunDone(t *testing.T) {
	st := NewMemStore()
	r := NewRunner(st)
	tk := mustCreate(t, r)
	if tk.Status != StatusPending {
		t.Fatalf("新建应 pending, got %s", tk.Status)
	}
	tk, err := r.Plan(context.Background(), tk.ID, planFixed("a", "b", "c"))
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != StatusPlanned || len(tk.Plan) != 3 {
		t.Fatalf("规划不对: %+v", tk)
	}
	tk, err = r.Run(context.Background(), tk.ID, stepOK, 0)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != StatusDone || tk.Cursor != 3 || len(tk.Results) != 3 {
		t.Fatalf("运行未完成: %+v", tk)
	}
	if tk.Plan[2].Status != StatusDone || tk.Results[2] != "done:c" {
		t.Fatalf("步骤结果不对: %+v", tk.Plan[2])
	}
}

func TestCheckpointResume(t *testing.T) {
	st := NewMemStore()
	r := NewRunner(st)
	tk := mustCreate(t, r)
	_, _ = r.Plan(context.Background(), tk.ID, planFixed("1", "2", "3"))
	tk, _ = r.Step(context.Background(), tk.ID, stepOK)
	if tk.Cursor != 1 || tk.Status != StatusRunning {
		t.Fatalf("一步后 cursor=1, got %+v", tk)
	}
	r2 := NewRunner(st)
	tk2, err := r2.Get(tk.ID)
	if err != nil || tk2.Cursor != 1 {
		t.Fatalf("重启后应从游标 1 继续: %+v %v", tk2, err)
	}
	tk2, err = r2.Run(context.Background(), tk.ID, stepOK, 0)
	if err != nil || tk2.Status != StatusDone || tk2.Cursor != 3 {
		t.Fatalf("续跑未完成: %+v %v", tk2, err)
	}
}

func TestWaitingApprovalThenResume(t *testing.T) {
	st := NewMemStore()
	r := NewRunner(st)
	tk := mustCreate(t, r)
	_, _ = r.Plan(context.Background(), tk.ID, planFixed("危险步骤", "收尾"))
	step := func(ctx context.Context, task *Task, s Step) (string, error) {
		if s.Title == "危险步骤" {
			return "需要确认", ErrWaitingApproval
		}
		return "ok", nil
	}
	tk, err := r.Run(context.Background(), tk.ID, step, 0)
	if err != nil {
		t.Fatal(err)
	}
	if tk.Status != StatusWaitingApproval {
		t.Fatalf("应等待审批, got %s", tk.Status)
	}
	tk, err = r.Resume(tk.ID)
	if err != nil || tk.Status != StatusRunning {
		t.Fatalf("resume: %+v %v", tk, err)
	}
	tk, err = r.Run(context.Background(), tk.ID, stepOK, 0)
	if err != nil || tk.Status != StatusDone {
		t.Fatalf("审批后续跑应完成: %+v %v", tk, err)
	}
}

func TestStepErrorFailsTask(t *testing.T) {
	st := NewMemStore()
	r := NewRunner(st)
	tk := mustCreate(t, r)
	_, _ = r.Plan(context.Background(), tk.ID, planFixed("x"))
	_, err := r.Step(context.Background(), tk.ID, func(ctx context.Context, task *Task, s Step) (string, error) {
		return "", errors.New("boom")
	})
	if err == nil {
		t.Fatal("应返回错误")
	}
	tk, _ = r.Get(tk.ID)
	if tk.Status != StatusFailed || tk.Error != "boom" {
		t.Fatalf("失败状态不对: %+v", tk)
	}
}

func TestPauseResumeCancel(t *testing.T) {
	st := NewMemStore()
	r := NewRunner(st)
	tk := mustCreate(t, r)
	_, _ = r.Plan(context.Background(), tk.ID, planFixed("1"))
	tk, err := r.Pause(tk.ID)
	if err != nil || tk.Status != StatusPaused {
		t.Fatalf("pause: %+v %v", tk, err)
	}
	tk, _ = r.Resume(tk.ID)
	if tk.Status != StatusRunning {
		t.Fatalf("resume: %s", tk.Status)
	}
	tk, _ = r.Cancel(tk.ID)
	if tk.Status != StatusFailed || tk.Error != "canceled" {
		t.Fatalf("cancel: %+v", tk)
	}
	if _, err := r.Pause(tk.ID); !errors.Is(err, ErrTerminal) {
		t.Fatalf("终态再操作应 ErrTerminal, got %v", err)
	}
}

func TestPlanFallbackEmpty(t *testing.T) {
	st := NewMemStore()
	r := NewRunner(st)
	tk, _ := r.Create(Task{Title: "独苗"})
	tk, _ = r.Plan(context.Background(), tk.ID, planFixed())
	if len(tk.Plan) != 1 || tk.Plan[0].Title != "独苗" {
		t.Fatalf("空计划应回退单步用标题: %+v", tk.Plan)
	}
}

func TestMemStoreListDelete(t *testing.T) {
	st := NewMemStore()
	r := NewRunner(st)
	a, _ := r.Create(Task{Title: "a", SessionID: "s1"})
	_, _ = r.Create(Task{Title: "b", SessionID: "s2"})
	list, _ := r.List("s1", 0)
	if len(list) != 1 || list[0].ID != a.ID {
		t.Fatalf("按会话过滤不对: %+v", list)
	}
	if err := r.Delete(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get(a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("删除后应 ErrNotFound, got %v", err)
	}
}
