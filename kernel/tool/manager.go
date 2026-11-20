package tool

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// ServerConfig 一个远程 MCP 服务器配置。
type ServerConfig struct {
	Name        string        // 唯一名（本地标识）
	Endpoint    string        // MCP 端点 URL
	Header      http.Header   // 额外请求头（如 Authorization）
	HTTPClient  *http.Client  // 可为 nil
	CallTimeout time.Duration // 单请求超时（<=0 用默认）
}

// QualifiedTool 归属某服务器的工具（服务器名 + 工具定义）。
type QualifiedTool struct {
	Server string
	Tool   Tool
}

// ServerStatus 服务器状态快照。
type ServerStatus struct {
	Name      string
	Enabled   bool
	ToolCount int
	Init      InitResult
	Err       string
}

// managedServer 管理器内部条目。
type managedServer struct {
	cfg     ServerConfig
	client  *Client
	enabled bool
	tools   []Tool
	init    InitResult
}

// Manager 远程 MCP 服务器管理器，支持**运行时热插拔**：
// 添加/移除/启用/停用服务器均无需重启进程（停用仅从工具聚合中隐藏，连接保留）。
// 并发安全。
type Manager struct {
	mu      sync.RWMutex
	servers map[string]*managedServer
	order   []string
}

// NewManager 建空管理器。
func NewManager() *Manager {
	return &Manager{servers: make(map[string]*managedServer)}
}

// Add 连接并初始化一个服务器、发现其工具，成功后登记（默认启用）。
// 失败不登记，返回错误。
func (m *Manager) Add(ctx context.Context, cfg ServerConfig) error {
	if cfg.Name == "" || cfg.Endpoint == "" {
		return &APIError{Kind: KindProtocol, Message: "server name/endpoint required"}
	}
	m.mu.RLock()
	_, exists := m.servers[cfg.Name]
	m.mu.RUnlock()
	if exists {
		return &APIError{Kind: KindProtocol, Message: "server already exists: " + cfg.Name}
	}
	cl := NewClient(cfg.Endpoint, Options{
		HTTPClient:  cfg.HTTPClient,
		Header:      cfg.Header,
		CallTimeout: cfg.CallTimeout,
	})
	init, err := cl.Initialize(ctx)
	if err != nil {
		return err
	}
	tools, err := cl.ListTools(ctx)
	if err != nil {
		_ = cl.Close(ctx)
		return err
	}
	m.mu.Lock()
	// 双检（并发 Add 同名）
	if _, ok := m.servers[cfg.Name]; ok {
		m.mu.Unlock()
		_ = cl.Close(ctx)
		return &APIError{Kind: KindProtocol, Message: "server already exists: " + cfg.Name}
	}
	m.servers[cfg.Name] = &managedServer{cfg: cfg, client: cl, enabled: true, tools: tools, init: init}
	m.order = append(m.order, cfg.Name)
	m.mu.Unlock()
	return nil
}

// Remove 断开并移除服务器（结束会话）。幂等（不存在返回错误）。
func (m *Manager) Remove(ctx context.Context, name string) error {
	m.mu.Lock()
	ms, ok := m.servers[name]
	if !ok {
		m.mu.Unlock()
		return &APIError{Kind: KindProtocol, Message: "server not found: " + name}
	}
	delete(m.servers, name)
	for i, n := range m.order {
		if n == name {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	m.mu.Unlock()
	if ms.client != nil {
		_ = ms.client.Close(ctx)
	}
	return nil
}

// SetEnabled 启用/停用服务器（停用后其工具不再出现在聚合结果中，但连接保留）。幂等。
func (m *Manager) SetEnabled(name string, v bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ms, ok := m.servers[name]
	if !ok {
		return &APIError{Kind: KindProtocol, Message: "server not found: " + name}
	}
	ms.enabled = v
	return nil
}

// Enabled 查询启用态。第二返回 false 表示服务器不存在。
func (m *Manager) Enabled(name string) (bool, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ms, ok := m.servers[name]
	if !ok {
		return false, false
	}
	return ms.enabled, true
}

// RefreshTools 重新拉取某服务器的工具列表（运行时工具变化）。需服务器存在。
func (m *Manager) RefreshTools(ctx context.Context, name string) error {
	m.mu.RLock()
	ms, ok := m.servers[name]
	m.mu.RUnlock()
	if !ok {
		return &APIError{Kind: KindProtocol, Message: "server not found: " + name}
	}
	tools, err := ms.client.ListTools(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	if cur, ok := m.servers[name]; ok {
		cur.tools = tools
	}
	m.mu.Unlock()
	return nil
}

// Servers 返回全部服务器状态（按添加顺序）。
func (m *Manager) Servers() []ServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ServerStatus, 0, len(m.order))
	for _, n := range m.order {
		ms := m.servers[n]
		out = append(out, ServerStatus{
			Name:      n,
			Enabled:   ms.enabled,
			ToolCount: len(ms.tools),
			Init:      ms.init,
		})
	}
	return out
}

// Tools 聚合所有**启用**服务器的工具（按服务器添加顺序、再按工具声明顺序）。
func (m *Manager) Tools() []QualifiedTool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []QualifiedTool
	for _, n := range m.order {
		ms := m.servers[n]
		if !ms.enabled {
			continue
		}
		for _, t := range ms.tools {
			out = append(out, QualifiedTool{Server: n, Tool: t})
		}
	}
	return out
}

// CallTool 调用指定服务器的某个工具。服务器须存在且启用。
func (m *Manager) CallTool(ctx context.Context, server, toolName string, args map[string]any) (*CallResult, error) {
	m.mu.RLock()
	ms, ok := m.servers[server]
	enabled := ok && ms.enabled
	var cl *Client
	if ok {
		cl = ms.client
	}
	m.mu.RUnlock()
	if !ok {
		return nil, &APIError{Kind: KindProtocol, Message: "server not found: " + server}
	}
	if !enabled {
		return nil, &APIError{Kind: KindProtocol, Message: "server disabled: " + server}
	}
	return cl.CallTool(ctx, toolName, args)
}

// Close 结束全部服务器会话（不删除配置，仅释放连接）。
func (m *Manager) Close(ctx context.Context) {
	m.mu.RLock()
	clients := make([]*Client, 0, len(m.servers))
	for _, ms := range m.servers {
		clients = append(clients, ms.client)
	}
	m.mu.RUnlock()
	for _, cl := range clients {
		_ = cl.Close(ctx)
	}
}
