package mobilebind

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cjintao133-cell/star-river/kernel/codebrain"
)

func stubLLM(t *testing.T, plan []string, stepText string) {
	t.Helper()
	oldStep, oldPlan := taskStepLLM, taskPlanLLM
	taskPlanLLM = func(ctx context.Context, b, k, m, goal string) ([]string, error) { return plan, nil }
	taskStepLLM = func(ctx context.Context, b, k, m, sys, user string) (string, error) { return stepText, nil }
	t.Cleanup(func() { taskStepLLM, taskPlanLLM = oldStep, oldPlan })
}

func TestTaskCreatePlanRun(t *testing.T) {
	initTestKernel(t)
	stubLLM(t, []string{"步骤一", "步骤二"}, "已完成")

	var tk map[string]any
	if err := json.Unmarshal([]byte(TaskCreate("s1", "c1", "测试任务", "把事办成")), &tk); err != nil {
		t.Fatalf("create: %v raw=%s", err, TaskCreate("s1", "c1", "测试任务", "把事办成"))
	}
	id, _ := tk["id"].(string)
	if id == "" || tk["status"] != "pending" {
		t.Fatalf("create 返回不对: %+v", tk)
	}

	_ = json.Unmarshal([]byte(TaskPlan(id, "http://x", "k", "m")), &tk)
	if tk["status"] != "planned" {
		t.Fatalf("plan 后应 planned: %+v", tk)
	}
	if len(tk["plan"].([]any)) != 2 {
		t.Fatalf("应有 2 步: %+v", tk["plan"])
	}

	_ = json.Unmarshal([]byte(TaskRun(id, "http://x", "k", "m", 0)), &tk)
	if tk["status"] != "done" {
		t.Fatalf("run 后应 done: %+v", tk)
	}
	if int(tk["cursor"].(float64)) != 2 {
		t.Fatalf("cursor 应为 2: %+v", tk)
	}
}

func TestTaskRiskStepWaitsApproval(t *testing.T) {
	initTestKernel(t)
	stubLLM(t, []string{"删除旧文件", "收尾"}, "ok")
	var tk map[string]any
	_ = json.Unmarshal([]byte(TaskCreate("s1", "", "清理", "清理并删除")), &tk)
	id := tk["id"].(string)
	_ = json.Unmarshal([]byte(TaskPlan(id, "", "", "")), &tk)
	_ = json.Unmarshal([]byte(TaskRun(id, "", "", "", 0)), &tk)
	if tk["status"] != "waiting_approval" {
		t.Fatalf("危险步骤应等待审批: %+v", tk)
	}
	// 恢复
	_ = json.Unmarshal([]byte(TaskResume(id)), &tk)
	if tk["status"] != "running" {
		t.Fatalf("resume 后应 running: %+v", tk)
	}
}

func TestTaskListGetDelete(t *testing.T) {
	initTestKernel(t)
	var a map[string]any
	_ = json.Unmarshal([]byte(TaskCreate("s1", "", "A", "g")), &a)
	id := a["id"].(string)
	_ = TaskCreate("s2", "", "B", "g")

	var list []map[string]any
	_ = json.Unmarshal([]byte(TaskList("s1", 10)), &list)
	if len(list) != 1 || list[0]["id"] != id {
		t.Fatalf("按会话列出不对: %+v", list)
	}
	var got map[string]any
	_ = json.Unmarshal([]byte(TaskGet(id)), &got)
	if got["title"] != "A" {
		t.Fatalf("get 不对: %+v", got)
	}
	if r := TaskDelete(id); r != "OK" {
		t.Fatalf("delete: %s", r)
	}
	if r := TaskGet(id); !strings.HasPrefix(r, "ERR") {
		t.Fatalf("删除后应 ERR: %s", r)
	}
}

func TestTaskCreateValidation(t *testing.T) {
	initTestKernel(t)
	if r := TaskCreate("", "", "", ""); !strings.HasPrefix(r, "ERR") {
		t.Fatalf("空标题/目标应 ERR: %s", r)
	}
	if r := TaskPlan("nope", "", "", ""); !strings.HasPrefix(r, "ERR") {
		t.Fatalf("不存在任务 plan 应 ERR: %s", r)
	}
}

func TestParseStepTitles(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"```json\n[\"a\",\"b\"]\n```", 2},
		{"[ \"x\" , \"y\" , \"z\" ]", 3},
		{"- 步骤1\n- 步骤2", 2},
		{"1. 甲\n2. 乙", 2},
	}
	for _, c := range cases {
		got := parseStepTitles(c.in)
		if len(got) != c.want {
			t.Errorf("parse %q => %v (want %d)", c.in, got, c.want)
		}
	}
}

func TestIsRiskyStep(t *testing.T) {
	for _, s := range []string{"删除旧文件", "发送邮件", "pay invoice", "transfer money"} {
		if !isRiskyStep(s) {
			t.Errorf("%q 应判危险", s)
		}
	}
	for _, s := range []string{"读取文件", "总结内容", "calculate sum"} {
		if isRiskyStep(s) {
			t.Errorf("%q 不应判危险", s)
		}
	}
}

var _ = codebrain.ErrNotFound
