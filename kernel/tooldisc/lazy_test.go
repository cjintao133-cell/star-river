package tooldisc

import (
	"fmt"
	"testing"
	"time"
)

// fakeSession 记录打开/关闭。
type fakeSession struct {
	server string
	closed bool
}

func (s *fakeSession) Close() error { s.closed = true; return nil }

func TestLazyConnectorLazyConnect(t *testing.T) {
	opened := map[string]int{}
	var sessions []*fakeSession
	factory := func(server string) (Session, error) {
		opened[server]++
		s := &fakeSession{server: server}
		sessions = append(sessions, s)
		return s, nil
	}
	c := NewLazyConnector(factory, time.Minute, nil)
	// 初始不连接任何 server（懒连接）
	if c.LiveCount() != 0 || c.TotalConnects() != 0 {
		t.Fatal("初始不应连接")
	}
	// 取 A 才连 A
	_, err := c.Get("A")
	if err != nil {
		t.Fatal(err)
	}
	if c.LiveCount() != 1 || opened["A"] != 1 || opened["B"] != 0 {
		t.Errorf("只应连接 A: live=%d opened=%v", c.LiveCount(), opened)
	}
	// 再次取 A 复用，不新建
	_, _ = c.Get("A")
	if opened["A"] != 1 {
		t.Errorf("复用不应重连, opened[A]=%d", opened["A"])
	}
}

func TestLazyConnectorIdleReap(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	var sessions []*fakeSession
	factory := func(server string) (Session, error) {
		s := &fakeSession{server: server}
		sessions = append(sessions, s)
		return s, nil
	}
	c := NewLazyConnector(factory, time.Minute, clock)
	_, _ = c.Get("A")
	// 未超时：不回收
	now = now.Add(30 * time.Second)
	if closed := c.Reap(); len(closed) != 0 {
		t.Errorf("未超时不应回收: %v", closed)
	}
	// 超时：回收
	now = now.Add(90 * time.Second)
	closed := c.Reap()
	if len(closed) != 1 || closed[0] != "A" || !sessions[0].closed {
		t.Errorf("超时应回收 A: %v", closed)
	}
	if c.LiveCount() != 0 {
		t.Error("回收后应为 0")
	}
}

func TestLazyConnectorFactoryErrorNotCached(t *testing.T) {
	fail := true
	factory := func(server string) (Session, error) {
		if fail {
			return nil, fmt.Errorf("connect fail")
		}
		return &fakeSession{server: server}, nil
	}
	c := NewLazyConnector(factory, time.Minute, nil)
	if _, err := c.Get("A"); err == nil {
		t.Fatal("应建连失败")
	}
	if c.LiveCount() != 0 {
		t.Error("失败不应缓存")
	}
	fail = false
	if _, err := c.Get("A"); err != nil {
		t.Errorf("重试应成功: %v", err)
	}
}

func TestLazyConnectorCloseAndCloseAll(t *testing.T) {
	factory := func(server string) (Session, error) { return &fakeSession{server: server}, nil }
	c := NewLazyConnector(factory, time.Minute, nil)
	_, _ = c.Get("A")
	_, _ = c.Get("B")
	if err := c.Close("A"); err != nil {
		t.Fatal(err)
	}
	if c.LiveCount() != 1 {
		t.Errorf("关闭 A 后应剩 1, got %d", c.LiveCount())
	}
	c.CloseAll()
	if c.LiveCount() != 0 {
		t.Error("CloseAll 后应 0")
	}
	// 关闭不存在的 server 是 no-op
	if err := c.Close("nope"); err != nil {
		t.Error("关闭不存在不应报错")
	}
}
