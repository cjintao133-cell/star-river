package mobilebind

// 内核"内功"桥：记忆卫生(hygiene)、长时身份/轨迹(identity)、cron 下次时间(cronexpr)、
// 生命周期钩子(hooks)。均为纯内核能力，供 App/调度/审计使用。

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/cjintao133-cell/star-river/kernel/cronexpr"
	"github.com/cjintao133-cell/star-river/kernel/hippocampus"
	"github.com/cjintao133-cell/star-river/kernel/hooks"
	"github.com/cjintao133-cell/star-river/kernel/hygiene"
	"github.com/cjintao133-cell/star-river/kernel/identity"
)

// ===== 生命周期钩子 =====

var hooksReg = hooks.NewRegistry()

// HooksFire 触发事件，返回结果 JSON 数组。payloadJSON 为对象字符串（可空）。
func HooksFire(event, payloadJSON string) string {
	var payload map[string]any
	if strings.TrimSpace(payloadJSON) != "" {
		_ = json.Unmarshal([]byte(payloadJSON), &payload)
	}
	res, aborted := hooksReg.Fire(event, payload)
	b, _ := json.Marshal(map[string]any{"results": res, "aborted": aborted})
	return string(b)
}

// HooksCount 某事件钩子数量。
func HooksCount(event string) int { return hooksReg.Count(event) }

// HooksClear 清空全部钩子。
func HooksClear() string { hooksReg.Clear(); return "OK" }

// ===== 记忆卫生 =====

// HygieneAnalyze 对传入记忆做卫生分析（纯函数，不改数据）。
// memoriesJSON 为 []hippocampus.Memory；configJSON 为 hygiene.Config；nowMs<=0 用当前时间。
// 返回 {prune:{drop_idx,kept,dropped}, duplicates:[{keep_idx,drop_idxs}]}。
func HygieneAnalyze(memoriesJSON, configJSON string, nowMs int64) string {
	var mems []hippocampus.Memory
	if strings.TrimSpace(memoriesJSON) != "" {
		if err := json.Unmarshal([]byte(memoriesJSON), &mems); err != nil {
			return "ERR:bad memories json"
		}
	}
	var cfg hygiene.Config
	if strings.TrimSpace(configJSON) != "" {
		_ = json.Unmarshal([]byte(configJSON), &cfg)
	}
	if nowMs <= 0 {
		nowMs = nowMillis()
	}
	rep := hygiene.Prune(mems, nowMs, cfg)
	dups := hygiene.Duplicates(mems, cfg)
	b, _ := json.Marshal(map[string]any{"prune": rep, "duplicates": dups})
	return string(b)
}

// ===== cron 下次时间 =====

// CronNext 返回严格晚于 afterMs 的下一个触发时刻（毫秒）。afterMs<=0 用当前。失败返回 "ERR:..."。
func CronNext(expr string, afterMs int64) string {
	s, err := cronexpr.Parse(expr)
	if err != nil {
		return "ERR:" + err.Error()
	}
	base := msToTime(afterMs)
	next := s.Next(base)
	if next.IsZero() {
		return "ERR:no match within 5y"
	}
	b, _ := json.Marshal(map[string]any{"next_ms": next.UnixMilli(), "next": next.Format("2006-01-02 15:04")})
	return string(b)
}

// ===== 长时身份 / 轨迹 =====

const (
	metaIdentityKey = "identity.v1"
	metaTrajKey     = "identity.trajectory.v1"
	trajCap         = 500
)

// identityStore 用 app_meta 持久化身份与轨迹。
type identityStore struct{}

func (identityStore) LoadIdentity() (identity.Identity, bool, error) {
	st := storeRef()
	if st == nil {
		return identity.Identity{}, false, nil
	}
	v := metaGet(st, metaIdentityKey)
	if v == "" {
		return identity.Identity{}, false, nil
	}
	var id identity.Identity
	if json.Unmarshal([]byte(v), &id) != nil {
		return identity.Identity{}, false, nil
	}
	return id, true, nil
}

func (identityStore) SaveIdentity(id identity.Identity) error {
	st := storeRef()
	if st == nil {
		return nil
	}
	if b, err := json.Marshal(id); err == nil {
		metaSet(st, metaIdentityKey, string(b))
	}
	return nil
}

func (identityStore) AppendEvent(e identity.Event) error {
	st := storeRef()
	if st == nil {
		return nil
	}
	var evs []identity.Event
	if v := metaGet(st, metaTrajKey); v != "" {
		_ = json.Unmarshal([]byte(v), &evs)
	}
	evs = append(evs, e)
	if len(evs) > trajCap {
		evs = evs[len(evs)-trajCap:]
	}
	if b, err := json.Marshal(evs); err == nil {
		metaSet(st, metaTrajKey, string(b))
	}
	return nil
}

func (identityStore) ListEvents(limit int) ([]identity.Event, error) {
	st := storeRef()
	if st == nil {
		return nil, nil
	}
	var evs []identity.Event
	if v := metaGet(st, metaTrajKey); v != "" {
		_ = json.Unmarshal([]byte(v), &evs)
	}
	if limit > 0 && len(evs) > limit {
		evs = evs[len(evs)-limit:]
	}
	return evs, nil
}

var identityMu sync.Mutex

func identityTracker() *identity.Tracker {
	identityMu.Lock()
	defer identityMu.Unlock()
	return identity.New(identityStore{})
}

// GetIdentity 取（或创建默认）身份 JSON。
func GetIdentity(defaultName, defaultRole string) string {
	id, err := identityTracker().Ensure(defaultName, defaultRole)
	if err != nil {
		return "ERR:" + err.Error()
	}
	b, _ := json.Marshal(id)
	return string(b)
}

// SetIdentity 覆盖身份（JSON: identity.Identity）。
func SetIdentity(idJSON string) string {
	var id identity.Identity
	if err := json.Unmarshal([]byte(idJSON), &id); err != nil {
		return "ERR:bad json"
	}
	if err := identityTracker().Set(id); err != nil {
		return "ERR:" + err.Error()
	}
	return "OK"
}

// RecordTrajectory 记录一条轨迹事件。
func RecordTrajectory(kind, summary, ref string) string {
	if err := identityTracker().Record(identity.Event{Kind: kind, Summary: summary, Ref: ref}); err != nil {
		return "ERR:" + err.Error()
	}
	return "OK"
}

// GetTrajectory 最近 n 条轨迹 JSON 数组。
func GetTrajectory(limit int) string {
	evs, err := identityTracker().Recent(limit)
	if err != nil {
		return "ERR:" + err.Error()
	}
	b, _ := json.Marshal(evs)
	return string(b)
}

// SummarizeTrajectory 最近 n 条轨迹的"当前进展"文本（供注入 system）。
func SummarizeTrajectory(limit int) string {
	s, err := identityTracker().Summarize(limit)
	if err != nil {
		return "ERR:" + err.Error()
	}
	return s
}

// ===== 小工具 =====

func nowMillis() int64 { return time.Now().UnixMilli() }

func msToTime(ms int64) time.Time {
	if ms <= 0 {
		return time.Now()
	}
	return time.UnixMilli(ms)
}
