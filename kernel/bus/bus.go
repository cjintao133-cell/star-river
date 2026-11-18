package bus

import (
	"errors"
	"sync"
	"sync/atomic"

	"star-river/kernel/core/types"
)

// ErrClosed 在总线关闭后由 Subscribe/Publish 返回。
var ErrClosed = errors.New("bus: closed")

// DefaultBufferSize 是 New 参数 ≤0 时的默认订阅缓冲深度。
const DefaultBufferSize = 64

// Bus 门铃广播总线（批准优化#4）：事件只带主题与版本号，订阅者收铃拉
// 最新快照，容忍丢铃。命令/查询不走 bus，直接接口调用（docs/architecture.md）。
// 背压：每订阅有界缓冲，满时丢弃最旧并计数；发布路径永不阻塞，
// 慢订阅者不影响发布者与其他订阅者（T-M1-02 验收）。
// v1 订阅全部主题，主题过滤在消费侧。并发安全。
type Bus struct {
	mu      sync.Mutex
	subs    map[*Subscription]struct{}
	bufSize int
	closed  bool
}

// New 创建总线；bufferSize ≤0 时用 DefaultBufferSize。
func New(bufferSize int) *Bus {
	if bufferSize <= 0 {
		bufferSize = DefaultBufferSize
	}
	return &Bus{subs: make(map[*Subscription]struct{}), bufSize: bufferSize}
}

// Subscribe 注册一个广播订阅；总线关闭后返回 ErrClosed。
func (b *Bus) Subscribe() (*Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrClosed
	}
	s := &Subscription{bus: b, ch: make(chan types.Event, b.bufSize)}
	b.subs[s] = struct{}{}
	return s, nil
}

// Publish 校验并广播门铃。结构非法返回 types 哨兵错误（不广播）；
// 总线关闭后返回 ErrClosed。对慢订阅者永不阻塞。
func (b *Bus) Publish(e types.Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrClosed
	}
	for s := range b.subs {
		s.deliver(e)
	}
	return nil
}

// Close 关闭总线与全部订阅通道；幂等。
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for s := range b.subs {
		close(s.ch)
		delete(b.subs, s)
	}
}

// Subscription 单个订阅者的有界信箱。
type Subscription struct {
	bus     *Bus
	ch      chan types.Event
	dropped atomic.Uint64
}

// Events 返回接收通道；退订或总线关闭后通道关闭，range 自然退出。
func (s *Subscription) Events() <-chan types.Event { return s.ch }

// Dropped 返回因缓冲满被丢弃（丢最旧）的事件累计数。
func (s *Subscription) Dropped() uint64 { return s.dropped.Load() }

// Unsubscribe 注销并关闭通道；幂等，总线关闭后调用安全。
func (s *Subscription) Unsubscribe() {
	s.bus.mu.Lock()
	defer s.bus.mu.Unlock()
	if _, ok := s.bus.subs[s]; ok {
		delete(s.bus.subs, s)
		close(s.ch)
	}
}

// deliver 仅在持有 bus.mu 时调用：发布者串行化，且无并发 close。
// 缓冲满时非阻塞弹出最旧再装入；弹出可能因消费者刚腾空而失败，
// 两种情形都保证至少一个空位且此后只有消费者会取走，末尾发送必立即成功。
func (s *Subscription) deliver(e types.Event) {
	select {
	case s.ch <- e:
		return
	default:
	}
	select {
	case <-s.ch:
		s.dropped.Add(1)
	default:
	}
	s.ch <- e
}
