package bus

import (
	"errors"
	"sync"
	"testing"
	"time"

	"star-river/kernel/core/types"
)

func ev(version int64) types.Event {
	return types.Event{Topic: "t", Version: version, Source: "test", Timestamp: version}
}

func mustSubscribe(t *testing.T, b *Bus) *Subscription {
	t.Helper()
	s, err := b.Subscribe()
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	return s
}

func recv(t *testing.T, s *Subscription) types.Event {
	t.Helper()
	select {
	case e, ok := <-s.Events():
		if !ok {
			t.Fatal("channel closed unexpectedly")
		}
		return e
	case <-time.After(2 * time.Second):
		t.Fatal("receive timeout")
		return types.Event{}
	}
}

func assertClosed(t *testing.T, s *Subscription) {
	t.Helper()
	select {
	case _, ok := <-s.Events():
		if ok {
			t.Fatal("expected closed channel, got event")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("channel not closed after unsubscribe/close")
	}
}

func TestBasicPubSubOrder(t *testing.T) {
	b := New(8)
	s := mustSubscribe(t, b)
	for i := int64(1); i <= 5; i++ {
		if err := b.Publish(ev(i)); err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
	}
	for i := int64(1); i <= 5; i++ {
		if got := recv(t, s); got.Version != i {
			t.Fatalf("FIFO broken: want version %d, got %d", i, got.Version)
		}
	}
	if s.Dropped() != 0 {
		t.Fatalf("no drops expected, got %d", s.Dropped())
	}
}

func TestMultipleSubscribersAllReceive(t *testing.T) {
	b := New(4)
	subs := []*Subscription{mustSubscribe(t, b), mustSubscribe(t, b), mustSubscribe(t, b)}
	if err := b.Publish(ev(7)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	for i, s := range subs {
		if got := recv(t, s); got.Version != 7 {
			t.Fatalf("subscriber %d: want 7, got %d", i, got.Version)
		}
	}
}

func TestBackpressureDropOldestAndCount(t *testing.T) {
	// 验收锚点：缓冲 4，无消费者，发 10 → 丢最旧 6，留最新 4。
	b := New(4)
	s := mustSubscribe(t, b)
	for i := int64(1); i <= 10; i++ {
		if err := b.Publish(ev(i)); err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
	}
	if got := s.Dropped(); got != 6 {
		t.Fatalf("dropped: want 6, got %d", got)
	}
	for i := int64(7); i <= 10; i++ {
		if got := recv(t, s); got.Version != i {
			t.Fatalf("oldest not dropped: want %d, got %d", i, got.Version)
		}
	}
}

func TestSlowSubscriberIsolation(t *testing.T) {
	// 验收锚点：一个永不读取的订阅者不得阻塞发布者，也不得影响快订阅者。
	// 锁步设计：每发一条等快订阅者收到并核对序号再发下一条，排除调度抖动；
	// 若发布路径被慢订阅者阻塞或快者被饿死，超时即失败。
	b := New(2)
	slow := mustSubscribe(t, b)
	fast := mustSubscribe(t, b)
	const n = 1000

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := int64(1); i <= n; i++ {
			if err := b.Publish(ev(i)); err != nil {
				t.Errorf("publish %d: %v", i, err)
				return
			}
			select {
			case e, ok := <-fast.Events():
				if !ok {
					t.Errorf("fast channel closed at %d", i)
					return
				}
				if e.Version != i {
					t.Errorf("fast subscriber lost/duplicated at %d: %d", i, e.Version)
					return
				}
			case <-time.After(2 * time.Second):
				t.Errorf("fast subscriber starved at %d", i)
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("publisher blocked by slow subscriber")
	}
	if fast.Dropped() != 0 {
		t.Fatalf("fast subscriber must not drop, got %d", fast.Dropped())
	}
	if d := slow.Dropped(); d != n-2 {
		t.Fatalf("slow dropped: want %d, got %d", n-2, d)
	}
	if got := recv(t, slow); got.Version != n-1 {
		t.Fatalf("slow buffer must keep newest: want %d, got %d", n-1, got.Version)
	}
	if got := recv(t, slow); got.Version != n {
		t.Fatalf("slow buffer must keep newest: want %d, got %d", n, got.Version)
	}
}

func TestReceivedPlusDroppedInvariant(t *testing.T) {
	// 并发不变量：每订阅 收到数 + 丢弃数 == 发布总数（不丢账不重账）。
	b := New(3)
	s := mustSubscribe(t, b)
	const producers, perProducer = 4, 250

	received := make(chan int, 1)
	go func() {
		n := 0
		for range s.Events() {
			n++
		}
		received <- n
	}()

	var wg sync.WaitGroup
	for p := 0; p < producers; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := int64(1); i <= perProducer; i++ {
				if err := b.Publish(ev(i)); err != nil {
					t.Errorf("publish: %v", err)
				}
			}
		}()
	}
	wg.Wait()
	s.Unsubscribe()

	var n int
	select {
	case n = <-received:
	case <-time.After(5 * time.Second):
		t.Fatal("consumer did not finish")
	}
	total := uint64(n) + s.Dropped()
	if total != producers*perProducer {
		t.Fatalf("accounting broken: received %d + dropped %d != %d", n, s.Dropped(), producers*perProducer)
	}
}

func TestUnsubscribeIdempotentAndIsolated(t *testing.T) {
	b := New(4)
	s1 := mustSubscribe(t, b)
	s2 := mustSubscribe(t, b)
	s1.Unsubscribe()
	s1.Unsubscribe() // 幂等，不得 panic
	assertClosed(t, s1)
	if err := b.Publish(ev(1)); err != nil {
		t.Fatalf("publish after unsubscribe: %v", err)
	}
	if got := recv(t, s2); got.Version != 1 {
		t.Fatalf("remaining subscriber must receive, got %d", got.Version)
	}
}

func TestCloseSemantics(t *testing.T) {
	b := New(4)
	s1 := mustSubscribe(t, b)
	s2 := mustSubscribe(t, b)
	b.Close()
	b.Close() // 幂等
	assertClosed(t, s1)
	assertClosed(t, s2)
	s1.Unsubscribe() // 关闭后退订安全
	if err := b.Publish(ev(1)); !errors.Is(err, ErrClosed) {
		t.Fatalf("publish after close: want ErrClosed, got %v", err)
	}
	if _, err := b.Subscribe(); !errors.Is(err, ErrClosed) {
		t.Fatalf("subscribe after close: want ErrClosed, got %v", err)
	}
}

func TestPublishInvalidEventRejected(t *testing.T) {
	// 失败路径：非法门铃必须报错且不广播（不许静默成功）。
	b := New(4)
	s := mustSubscribe(t, b)
	if err := b.Publish(types.Event{}); !errors.Is(err, types.ErrEmptyField) {
		t.Fatalf("want ErrEmptyField, got %v", err)
	}
	if err := b.Publish(types.Event{Topic: "t", Source: "s", Version: -1}); !errors.Is(err, types.ErrNegative) {
		t.Fatalf("want ErrNegative, got %v", err)
	}
	select {
	case e := <-s.Events():
		t.Fatalf("invalid event must not broadcast, got %+v", e)
	default:
	}
}

func TestPublishNoSubscribers(t *testing.T) {
	b := New(4)
	if err := b.Publish(ev(1)); err != nil {
		t.Fatalf("publish without subscribers must succeed: %v", err)
	}
}

func TestDefaultBufferSizeBoundary(t *testing.T) {
	// 边界：0 与负数都落到默认深度。
	for _, size := range []int{0, -3} {
		b := New(size)
		s := mustSubscribe(t, b)
		for i := int64(1); i <= DefaultBufferSize+1; i++ {
			if err := b.Publish(ev(i)); err != nil {
				t.Fatalf("publish %d: %v", i, err)
			}
		}
		if d := s.Dropped(); d != 1 {
			t.Fatalf("size %d: dropped want 1, got %d", size, d)
		}
		if got := recv(t, s); got.Version != 2 {
			t.Fatalf("size %d: oldest must be version 2, got %d", size, got.Version)
		}
	}
}
