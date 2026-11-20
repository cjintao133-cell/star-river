package tool

import (
	"context"
	"sync"
	"time"
)

// StdioManagerConfig stdio 管理器配置。
type StdioManagerConfig struct {
	Stdio             StdioConfig
	InitTimeout       time.Duration // 启动握手超时（<=0 用 15s）
	CallTimeout       time.Duration // 单次请求超时（<=0 用 30s）
	AutoRestart       bool          // 崩溃后自动回收（后续调用/心跳会重新拉起）
	MaxRestarts       int           // 生命周期内自动重启上限（<=0 视为不限）
	HeartbeatInterval time.Duration // 自动心跳间隔（<=0 不自动心跳）
	HeartbeatTimeout  time.Duration // 单次心跳超时（<=0 用 3s）
}

// StdioManagerStatus 管理器状态快照。
type StdioManagerStatus struct {
	Running           bool
	Restarts          int
	LastHeartbeatMs   int64 // UnixMilli，0 表示尚未有心跳
	HeartbeatFailures int
	Init              InitResult
}

// StdioManager 管理一个 stdio MCP 子进程的生命周期：
//   - 拉起（懒启动）、心跳保活、崩溃回收（AutoRestart 时后续使用/心跳自动重启）、
//     崩溃隔离（子进程死不拖垮宿主，单次调用返回错误）。
//
// 回收语义：一次调用若因进程死亡失败，**不在同一次调用里重试**（避免对必崩工具反复点火、白烧重启次数）；
// 进程标记死亡后，下一次使用或心跳会按 AutoRestart 决定是否重新拉起。
type StdioManager struct {
	cfg StdioManagerConfig

	mu          sync.Mutex
	client      *StdioClient
	initialized bool
	startedOnce bool
	restarts    int
	lastHB      time.Time
	hbFailures  int

	hbStop chan struct{}
	hbOnce sync.Once
}

// NewStdioManager 建管理器。
func NewStdioManager(cfg StdioManagerConfig) *StdioManager {
	if cfg.InitTimeout <= 0 {
		cfg.InitTimeout = 15 * time.Second
	}
	if cfg.CallTimeout <= 0 {
		cfg.CallTimeout = stdioDefaultCallTimeout
	}
	if cfg.HeartbeatTimeout <= 0 {
		cfg.HeartbeatTimeout = 3 * time.Second
	}
	return &StdioManager{cfg: cfg}
}

// Start 拉起子进程并完成握手（幂等：已运行则直接返回）。
func (m *StdioManager) Start(ctx context.Context) error {
	_, err := m.ensure(ctx)
	return err
}

// ensure 保证存在已初始化且存活的客户端；必要时按策略（重）启动。
func (m *StdioManager) ensure(ctx context.Context) (*StdioClient, error) {
	m.mu.Lock()
	if m.client != nil && m.client.Alive() && m.initialized {
		cl := m.client
		m.mu.Unlock()
		return cl, nil
	}
	// 曾经启动过但已死亡：是否允许回收取决于 AutoRestart。
	if m.startedOnce && !m.cfg.AutoRestart {
		m.mu.Unlock()
		return nil, &APIError{Kind: KindNetwork, Message: "process dead (auto-restart disabled)"}
	}
	m.mu.Unlock()

	if err := m.dial(ctx); err != nil {
		return nil, err
	}
	m.mu.Lock()
	cl := m.client
	m.mu.Unlock()
	return cl, nil
}

// dial 创建新客户端、拉起子进程、握手。若是死亡后的回收，计一次重启并受 MaxRestarts 约束。
func (m *StdioManager) dial(ctx context.Context) error {
	m.mu.Lock()
	if m.client != nil && m.client.Alive() && m.initialized {
		m.mu.Unlock()
		return nil
	}
	if m.startedOnce && m.cfg.MaxRestarts > 0 && m.restarts >= m.cfg.MaxRestarts {
		m.mu.Unlock()
		return &APIError{Kind: KindNetwork, Message: "max restarts exceeded"}
	}
	if m.startedOnce {
		m.restarts++
	}
	m.mu.Unlock()

	cl := NewStdioClient(m.cfg.Stdio, StdioClientOptions{CallTimeout: m.cfg.CallTimeout})
	ictx, cancel := context.WithTimeout(ctx, m.cfg.InitTimeout)
	_, err := cl.Initialize(ictx)
	cancel()
	if err != nil {
		_ = cl.Close()
		return err
	}

	m.mu.Lock()
	old := m.client
	m.client = cl
	m.initialized = true
	m.startedOnce = true
	m.lastHB = time.Now()
	m.hbFailures = 0
	m.mu.Unlock()
	if old != nil && old != cl {
		_ = old.Close()
	}
	return nil
}

// ListTools 发现工具。
func (m *StdioManager) ListTools(ctx context.Context) ([]Tool, error) {
	cl, err := m.ensure(ctx)
	if err != nil {
		return nil, err
	}
	return cl.ListTools(ctx)
}

// CallTool 调用工具。
func (m *StdioManager) CallTool(ctx context.Context, name string, args map[string]any) (*CallResult, error) {
	cl, err := m.ensure(ctx)
	if err != nil {
		return nil, err
	}
	return cl.CallTool(ctx, name, args)
}

// Ping 心跳一次。
func (m *StdioManager) Ping(ctx context.Context) error {
	cl, err := m.ensure(ctx)
	if err != nil {
		return err
	}
	pctx, cancel := context.WithTimeout(ctx, m.cfg.HeartbeatTimeout)
	defer cancel()
	if err := cl.Ping(pctx); err != nil {
		m.mu.Lock()
		m.hbFailures++
		m.mu.Unlock()
		return err
	}
	m.mu.Lock()
	m.lastHB = time.Now()
	m.hbFailures = 0
	m.mu.Unlock()
	return nil
}

// StartHeartbeat 启动后台心跳协程（HeartbeatInterval<=0 时为无操作）。
func (m *StdioManager) StartHeartbeat() {
	m.hbOnce.Do(func() {
		if m.cfg.HeartbeatInterval <= 0 {
			return
		}
		m.hbStop = make(chan struct{})
		go m.heartbeatLoop(m.hbStop)
	})
}

// heartbeatLoop 周期性 ping；进程死亡且 AutoRestart 时借 ensure 自动回收。
func (m *StdioManager) heartbeatLoop(stop chan struct{}) {
	ticker := time.NewTicker(m.cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), m.cfg.HeartbeatTimeout)
			_ = m.Ping(ctx) // Ping 内部经 ensure 触发回收
			cancel()
		}
	}
}

// Status 状态快照。
func (m *StdioManager) Status() StdioManagerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	var last int64
	if !m.lastHB.IsZero() {
		last = m.lastHB.UnixMilli()
	}
	var init InitResult
	if m.client != nil {
		init = m.client.InitResult()
	}
	return StdioManagerStatus{
		Running:           m.client != nil && m.client.Alive() && m.initialized,
		Restarts:          m.restarts,
		LastHeartbeatMs:   last,
		HeartbeatFailures: m.hbFailures,
		Init:              init,
	}
}

// Stop 停止心跳并关闭子进程。
func (m *StdioManager) Stop() {
	m.hbOnce.Do(func() {})
	if m.hbStop != nil {
		close(m.hbStop)
		m.hbStop = nil
	}
	m.mu.Lock()
	cl := m.client
	m.client = nil
	m.initialized = false
	m.mu.Unlock()
	if cl != nil {
		_ = cl.Close()
	}
}
