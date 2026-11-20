package tooldisc

import (
	"sync"
	"time"
)

// Session 一条到某个 MCP server 的连接（由宿主实现，如包装 tool.Client）。Close 释放连接。
type Session interface {
	Close() error
}

// Connector 懒连接管理：不是所有 server 都常连，只有被检索命中/调用时才连，空闲超时断开。
type Connector interface {
	// Get 返回 server 的活动会话；未连接则按需建连（懒连接）。
	Get(server string) (Session, error)
	// Close 主动断开某 server。
	Close(server string) error
	// CloseAll 断开全部。
	CloseAll()
}

// Factory 建连工厂（由宿主提供；测试可注入 fake）。
type Factory func(server string) (Session, error)

// LazyConnector 实现 Connector：首次 Get 时建连，记录最近使用时刻；Reap 关闭空闲超时的连接。
// 并发安全。时钟可注入（便于确定性测试）。
type LazyConnector struct {
	mu        sync.Mutex
	factory   Factory
	idle      time.Duration
	now       func() time.Time
	live      map[string]*liveConn
	connected int // 累计建连次数（测试/观测用）
}

type liveConn struct {
	sess     Session
	lastUsed time.Time
}

// NewLazyConnector 建懒连接器。idle<=0 用 5 分钟；now 为 nil 用 time.Now。
func NewLazyConnector(factory Factory, idle time.Duration, now func() time.Time) *LazyConnector {
	if idle <= 0 {
		idle = 5 * time.Minute
	}
	if now == nil {
		now = time.Now
	}
	return &LazyConnector{factory: factory, idle: idle, now: now, live: make(map[string]*liveConn)}
}

// Get 取会话；未连接则建连（懒）。连接失败不缓存。
func (c *LazyConnector) Get(server string) (Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if lc, ok := c.live[server]; ok {
		lc.lastUsed = c.now()
		return lc.sess, nil
	}
	sess, err := c.factory(server)
	if err != nil {
		return nil, err
	}
	c.live[server] = &liveConn{sess: sess, lastUsed: c.now()}
	c.connected++
	return sess, nil
}

// Reap 关闭空闲超时的连接（now-lastUsed > idle）。
func (c *LazyConnector) Reap() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	var closed []string
	for server, lc := range c.live {
		if now.Sub(lc.lastUsed) > c.idle {
			_ = lc.sess.Close()
			delete(c.live, server)
			closed = append(closed, server)
		}
	}
	return closed
}

// Close 断开某 server（不存在返回 nil）。
func (c *LazyConnector) Close(server string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if lc, ok := c.live[server]; ok {
		delete(c.live, server)
		return lc.sess.Close()
	}
	return nil
}

// CloseAll 断开全部。
func (c *LazyConnector) CloseAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for server, lc := range c.live {
		_ = lc.sess.Close()
		delete(c.live, server)
	}
}

// LiveCount 当前活动连接数。
func (c *LazyConnector) LiveCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.live)
}

// TotalConnects 累计建连次数。
func (c *LazyConnector) TotalConnects() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}
