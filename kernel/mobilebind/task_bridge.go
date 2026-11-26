package mobilebind

// P0-1 任务/目标运行时桥：
//   - sqlTaskStore：用内核 SQLite(tasks 表) 实现 codebrain.Store，任务跨会话持久、可断点续跑。
//   - 模型规划/执行器：taskPlanLLM / taskStepLLM 为可替换桩（测试注入假实现，真机走模型）。
//   - 桥函数：Create/Plan/Run/Step/List/Get/Pause/Resume/Cancel/Delete。
// 危险步骤自动转入 waiting_approval（接 P0-3 审批语义）。

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/cjintao133-cell/star-river/kernel/codebrain"
)

// ---- SQLite Store ----

type sqlTaskStore struct{}

func (sqlTaskStore) Save(t *codebrain.Task) error {
	st := storeRef()
	if st == nil {
		return errors.New("no store")
	}
	plan, _ := json.Marshal(t.Plan)
	results, _ := json.Marshal(t.Results)
	_, err := st.Exec(context.Background(),
		`INSERT OR REPLACE INTO tasks(id,session_id,character_id,title,goal,status,plan_json,cursor,results_json,error,created_at,updated_at)
         VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.SessionID, t.CharacterID, t.Title, t.Goal, string(t.Status), string(plan), t.Cursor, string(results), t.Error, t.CreatedAt, t.UpdatedAt)
	return err
}

func (sqlTaskStore) Load(id string) (*codebrain.Task, error) {
	st := storeRef()
	if st == nil {
		return nil, errors.New("no store")
	}
	row := st.DB().QueryRow(`SELECT id,session_id,character_id,title,goal,status,plan_json,cursor,results_json,error,created_at,updated_at FROM tasks WHERE id=?`, id)
	t, err := scanTask(row.Scan)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return nil, codebrain.ErrNotFound
		}
		return nil, err
	}
	return t, nil
}

func (sqlTaskStore) List(sessionID string, limit int) ([]*codebrain.Task, error) {
	st := storeRef()
	if st == nil {
		return nil, errors.New("no store")
	}
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT id,session_id,character_id,title,goal,status,plan_json,cursor,results_json,error,created_at,updated_at FROM tasks`
	args := []any{}
	if sessionID != "" {
		q += ` WHERE session_id=?`
		args = append(args, sessionID)
	}
	q += ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := st.Query(context.Background(), q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*codebrain.Task, 0, limit)
	for rows.Next() {
		t, err := scanTask(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (sqlTaskStore) Delete(id string) error {
	st := storeRef()
	if st == nil {
		return errors.New("no store")
	}
	_, err := st.Exec(context.Background(), `DELETE FROM tasks WHERE id=?`, id)
	return err
}

func scanTask(sc func(...any) error) (*codebrain.Task, error) {
	var t codebrain.Task
	var status, planJSON, resultsJSON string
	if err := sc(&t.ID, &t.SessionID, &t.CharacterID, &t.Title, &t.Goal, &status, &planJSON, &t.Cursor, &resultsJSON, &t.Error, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	t.Status = codebrain.Status(status)
	_ = json.Unmarshal([]byte(planJSON), &t.Plan)
	_ = json.Unmarshal([]byte(resultsJSON), &t.Results)
	return &t, nil
}

func taskRunner() *codebrain.Runner { return codebrain.NewRunner(sqlTaskStore{}) }

// ---- 可替换的模型桩 ----

// taskCapSink 捕获一次性补全的输出。
type taskCapSink struct {
	buf strings.Builder
	err string
}

func (s *taskCapSink) OnDelta(t string) { s.buf.WriteString(t) }
func (s *taskCapSink) OnDone(m string)  {}
func (s *taskCapSink) OnError(e string) { s.err = e }

// taskStepLLM 单次补全：真机走 ChatStream；测试可替换。
var taskStepLLM = func(ctx context.Context, baseURL, apiKey, model, system, user string) (string, error) {
	s := &taskCapSink{}
	ChatStream(baseURL, apiKey, model, system, user, s)
	if s.err != "" {
		return "", errors.New(s.err)
	}
	return strings.TrimSpace(s.buf.String()), nil
}

// taskPlanLLM 用模型把目标拆成步骤标题；测试可替换。
var taskPlanLLM = func(ctx context.Context, baseURL, apiKey, model, goal string) ([]string, error) {
	system := "你是任务规划器。把用户目标拆成 3-8 个可执行步骤，只输出 JSON 字符串数组（如 [\"步骤1\",\"步骤2\"]），不要任何多余文字。"
	out, err := taskStepLLM(ctx, baseURL, apiKey, model, system, "目标："+goal)
	if err != nil {
		return nil, err
	}
	return parseStepTitles(out), nil
}

// parseStepTitles 稳健解析步骤标题数组（去代码围栏、取首个 [...]、失败按行回退）。
func parseStepTitles(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "["); i >= 0 {
		if j := strings.LastIndex(s, "]"); j > i {
			var arr []string
			if json.Unmarshal([]byte(s[i:j+1]), &arr) == nil && len(arr) > 0 {
				out := make([]string, 0, len(arr))
				for _, x := range arr {
					if t := strings.TrimSpace(x); t != "" {
						out = append(out, t)
					}
				}
				if len(out) > 0 {
					return out
				}
			}
		}
	}
	// 回退：按行
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "-*0123456789.、) "))
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// isRiskyStep 危险步骤（删除/发送/支付/转账等）→ 等待审批。
func isRiskyStep(title string) bool {
	t := strings.ToLower(title)
	for _, w := range []string{"删除", "删掉", "清空", "转账", "支付", "付款", "购买", "发送", "发布", "delete", "remove", "pay", "transfer", "send", "publish"} {
		if strings.Contains(t, w) {
			return true
		}
	}
	return false
}

func makeTaskStepFn(baseURL, apiKey, model string) codebrain.StepFunc {
	return func(ctx context.Context, t *codebrain.Task, s codebrain.Step) (string, error) {
		if isRiskyStep(s.Title) {
			return "该步骤涉及敏感/不可逆操作，等待用户确认", codebrain.ErrWaitingApproval
		}
		system := "你是任务执行器。根据总体目标与当前步骤，给出简洁、具体的执行结果（中文）。"
		prior := strings.Join(t.Results, "\n")
		user := "总体目标：" + t.Goal + "\n当前步骤：" + s.Title + "\n已完成：\n" + prior
		return taskStepLLM(ctx, baseURL, apiKey, model, system, user)
	}
}

// ---- 桥 ----

func taskErr(err error) string {
	if err == nil {
		return "ERR:unknown"
	}
	return "ERR:" + err.Error()
}

func taskToJSON(t *codebrain.Task) string {
	b, _ := json.Marshal(t)
	return string(b)
}

// TaskCreate 新建任务（pending），返回任务 JSON。
func TaskCreate(sessionID, characterID, title, goal string) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	if strings.TrimSpace(title) == "" && strings.TrimSpace(goal) == "" {
		return "ERR:title/goal empty"
	}
	t, err := taskRunner().Create(codebrain.Task{SessionID: sessionID, CharacterID: characterID, Title: title, Goal: goal})
	if err != nil {
		return taskErr(err)
	}
	return taskToJSON(t)
}

// TaskPlan 用模型规划步骤，返回任务 JSON。
func TaskPlan(id, baseURL, apiKey, model string) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	r := taskRunner()
	t, err := r.Plan(context.Background(), id, func(ctx context.Context, t *codebrain.Task) ([]string, error) {
		return taskPlanLLM(ctx, baseURL, apiKey, model, t.Goal)
	})
	if err != nil {
		return taskErr(err)
	}
	return taskToJSON(t)
}

// TaskRun 连续执行（maxSteps<=0 默认 50），返回任务 JSON。
func TaskRun(id, baseURL, apiKey, model string, maxSteps int) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	t, err := taskRunner().Run(context.Background(), id, makeTaskStepFn(baseURL, apiKey, model), maxSteps)
	if err != nil && t == nil {
		return taskErr(err)
	}
	// 单步失败会返回 (task, err)：仍返回任务 JSON（含 failed 状态）便于 UI 展示。
	return taskToJSON(t)
}

// TaskStep 执行单步，返回任务 JSON。
func TaskStep(id, baseURL, apiKey, model string) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	t, err := taskRunner().Step(context.Background(), id, makeTaskStepFn(baseURL, apiKey, model))
	if err != nil && t == nil {
		return taskErr(err)
	}
	return taskToJSON(t)
}

// TaskList 列出任务（sessionID 空=全部），返回 JSON 数组。
func TaskList(sessionID string, limit int) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	list, err := taskRunner().List(sessionID, limit)
	if err != nil {
		return taskErr(err)
	}
	b, _ := json.Marshal(list)
	return string(b)
}

// TaskGet 取任务 JSON。
func TaskGet(id string) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	t, err := taskRunner().Get(id)
	if err != nil {
		return taskErr(err)
	}
	return taskToJSON(t)
}

// TaskPause 暂停。
func TaskPause(id string) string {
	return taskOp(id, func(r *codebrain.Runner) (*codebrain.Task, error) { return r.Pause(id) })
}

// TaskResume 恢复。
func TaskResume(id string) string {
	return taskOp(id, func(r *codebrain.Runner) (*codebrain.Task, error) { return r.Resume(id) })
}

// TaskCancel 取消。
func TaskCancel(id string) string {
	return taskOp(id, func(r *codebrain.Runner) (*codebrain.Task, error) { return r.Cancel(id) })
}

func taskOp(id string, fn func(*codebrain.Runner) (*codebrain.Task, error)) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	t, err := fn(taskRunner())
	if err != nil {
		return taskErr(err)
	}
	return taskToJSON(t)
}

// TaskDelete 删除，返回 "OK"/"ERR:..."。
func TaskDelete(id string) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	if err := taskRunner().Delete(id); err != nil {
		return taskErr(err)
	}
	return "OK"
}
