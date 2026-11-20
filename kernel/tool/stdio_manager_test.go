package tool

import (
	"context"
	"testing"
	"time"
)

func TestStdioManagerLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m := NewStdioManager(StdioManagerConfig{Stdio: helperConfig()})
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	if !m.Status().Running {
		t.Error("应运行中")
	}
	tools, err := m.ListTools(ctx)
	if err != nil || len(tools) != 3 {
		t.Fatalf("ListTools: %v %d", err, len(tools))
	}
	res, err := m.CallTool(ctx, "echo", map[string]any{"q": "x"})
	if err != nil || res.Content[0].Text != "echo:x" {
		t.Fatalf("CallTool: %v %+v", err, res)
	}
	if err := m.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestStdioManagerCrashRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m := NewStdioManager(StdioManagerConfig{Stdio: helperConfig(), AutoRestart: true, MaxRestarts: 5})
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	// 正常工作
	if _, err := m.CallTool(ctx, "echo", map[string]any{"q": "1"}); err != nil {
		t.Fatal(err)
	}
	// 触发崩溃（此调用本身可能返回错误）
	_, _ = m.CallTool(ctx, "crash", nil)
	// 崩溃后下一次调用应自动重启并成功
	res, err := m.CallTool(ctx, "echo", map[string]any{"q": "2"})
	if err != nil {
		t.Fatalf("崩溃后应自动重启成功: %v", err)
	}
	if res.Content[0].Text != "echo:2" {
		t.Errorf("重启后调用结果不对: %+v", res)
	}
	if m.Status().Restarts < 1 {
		t.Errorf("应记录至少 1 次重启, got %d", m.Status().Restarts)
	}
}

func TestStdioManagerCrashNoAutoRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m := NewStdioManager(StdioManagerConfig{Stdio: helperConfig(), AutoRestart: false})
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	_, _ = m.CallTool(ctx, "crash", nil)
	// 无自动重启：进程已死，下次调用失败（但宿主不崩）
	_, err := m.CallTool(ctx, "echo", nil)
	if err == nil {
		t.Error("无自动重启崩溃后应报错")
	}
}

func TestStdioManagerMaxRestarts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m := NewStdioManager(StdioManagerConfig{Stdio: helperConfig(), AutoRestart: true, MaxRestarts: 1})
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	// 第一次崩溃 → 允许重启一次
	_, _ = m.CallTool(ctx, "crash", nil)
	if _, err := m.CallTool(ctx, "echo", nil); err != nil {
		t.Fatalf("第一次重启应成功: %v", err)
	}
	// 第二次崩溃 → 超过 MaxRestarts=1 → 重启被拒
	_, _ = m.CallTool(ctx, "crash", nil)
	if _, err := m.CallTool(ctx, "echo", nil); err == nil {
		t.Error("超过 MaxRestarts 应报错")
	}
	if m.Status().Restarts > 1 {
		t.Errorf("重启次数不应超过 1, got %d", m.Status().Restarts)
	}
}

func TestStdioManagerHeartbeat(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m := NewStdioManager(StdioManagerConfig{
		Stdio:             helperConfig(),
		HeartbeatInterval: 50 * time.Millisecond,
		HeartbeatTimeout:  500 * time.Millisecond,
	})
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	m.StartHeartbeat()

	// 等心跳跑几拍
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if m.Status().LastHeartbeatMs > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	st := m.Status()
	if st.LastHeartbeatMs == 0 {
		t.Fatal("心跳未记录")
	}
	if st.HeartbeatFailures != 0 {
		t.Errorf("健康进程不应有失败, got %d", st.HeartbeatFailures)
	}
	if !st.Running {
		t.Error("应运行中")
	}
}

func TestStdioManagerHeartbeatRecoversCrash(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m := NewStdioManager(StdioManagerConfig{
		Stdio:             helperConfig(),
		AutoRestart:       true,
		MaxRestarts:       5,
		HeartbeatInterval: 40 * time.Millisecond,
		HeartbeatTimeout:  300 * time.Millisecond,
	})
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	m.StartHeartbeat()
	// 杀子进程后，心跳应检测到并自动重启
	_, _ = m.CallTool(ctx, "crash", nil)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if m.Status().Running {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !m.Status().Running {
		t.Error("心跳应自动重启进程")
	}
	// 重启后可用
	if _, err := m.CallTool(ctx, "echo", map[string]any{"q": "r"}); err != nil {
		t.Fatalf("心跳恢复后调用应成功: %v", err)
	}
}
