package mobilebind

// 事件总线桥（bus）：项目"四件套"之一。作为应用级事件背板：
//   - router 灾备切换 → 发布 adapter.route.failover 门铃；
//   - 装配层情绪/记忆/主动事件 → 发布对应 topic；
// 单订阅把最近的 Event 收集进环形缓冲，供 UI 查看（"系统·安全·自愈"里的事件流）。

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/cjintao133-cell/star-river/kernel/bus"
	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

const busRingCap = 50

var (
	busMu     sync.Mutex
	appBus    *bus.Bus
	busRing   []types.Event
	busSubbed bool
)

func ensureBus() *bus.Bus {
	busMu.Lock()
	defer busMu.Unlock()
	if appBus != nil {
		return appBus
	}
	appBus = bus.New(64)
	if !busSubbed {
		busSubbed = true
		if sub, err := appBus.Subscribe(); err == nil {
			go func() {
				for e := range sub.Events() {
					busMu.Lock()
					busRing = append(busRing, e)
					if len(busRing) > busRingCap {
						busRing = busRing[len(busRing)-busRingCap:]
					}
					busMu.Unlock()
				}
			}()
		}
	}
	return appBus
}

// busPublisher 满足 router.Publisher（Publish(types.Event) error）。
func busPublisher() *bus.Bus { return ensureBus() }

// BusPublish 发布一条门铃事件（topic/source）。返回 "OK"/"ERR:..."。
func BusPublish(topic, source string) string {
	b := ensureBus()
	if source == "" {
		source = "app"
	}
	err := b.Publish(types.Event{Topic: topic, Source: source, Version: 0, Timestamp: time.Now().UnixMilli()})
	if err != nil {
		return "ERR:" + err.Error()
	}
	return "OK"
}

// BusRecent 最近事件（JSON 数组，最多 50 条）。
func BusRecent() string {
	busMu.Lock()
	defer busMu.Unlock()
	out := make([]types.Event, len(busRing))
	copy(out, busRing)
	b, _ := json.Marshal(out)
	return string(b)
}
