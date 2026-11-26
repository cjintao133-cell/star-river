// Package trace 提供最小可用的全链路追踪原语（span/trace）。
//
// 设计：本包零内部依赖（仅标准库），通过 Recorder 接口把 span 落库/上报，
// 具体后端（SQLite 等）由入口层实现。用于把一次对话里的 llm/tool/memory/search 等
// 步骤串成一棵 span 树，支撑可观测、回放与评测。
package trace

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// Span 一条追踪片段。
type Span struct {
	TraceID   string         `json:"trace_id"`
	SpanID    string         `json:"span_id"`
	ParentID  string         `json:"parent_id,omitempty"`
	Kind      string         `json:"kind"` // llm|tool|memory|search|compact|...
	Name      string         `json:"name"`
	SessionID string         `json:"session_id,omitempty"`
	Status    string         `json:"status"` // ok|error|running|...
	StartMs   int64          `json:"start_ms"`
	EndMs     int64          `json:"end_ms"`
	Attrs     map[string]any `json:"attrs,omitempty"`
}

// Recorder 接收 span 的后端。StartSpan 在开始时落一条（end=0/status=running），
// FinishSpan 补齐结束时间与状态/属性。
type Recorder interface {
	StartSpan(s Span) error
	FinishSpan(traceID, spanID string, endMs int64, status string, attrs map[string]any) error
}

// IDGen 生成 ID；默认 16 字节随机十六进制。
type IDGen func() string

// Clock 返回毫秒时间戳；默认 time.Now().UnixMilli。
type Clock func() int64

// Tracer 组织 span 生命周期。
type Tracer struct {
	rec Recorder
	id  IDGen
	now Clock
}

// NewTracer 用给定后端建 Tracer（默认 ID/时钟）。
func NewTracer(rec Recorder) *Tracer {
	return &Tracer{rec: rec, id: NewID, now: NowMs}
}

// NewTracerWith 允许注入自定义 ID 生成/时钟（测试用）。
func NewTracerWith(rec Recorder, id IDGen, now Clock) *Tracer {
	if id == nil {
		id = NewID
	}
	if now == nil {
		now = NowMs
	}
	return &Tracer{rec: rec, id: id, now: now}
}

// Begin 开始一个 span。parentTrace 为空则新开一条 trace，否则并入该 trace。
// parentSpan 作为父 span id（可空）。返回生成的 Span（已写入后端）。
func (t *Tracer) Begin(parentTrace, parentSpan, kind, name, sessionID string) (Span, error) {
	traceID := parentTrace
	if traceID == "" {
		traceID = t.id()
	}
	s := Span{
		TraceID:   traceID,
		SpanID:    t.id(),
		ParentID:  parentSpan,
		Kind:      kind,
		Name:      name,
		SessionID: sessionID,
		Status:    "running",
		StartMs:   t.now(),
	}
	if t.rec != nil {
		if err := t.rec.StartSpan(s); err != nil {
			return s, err
		}
	}
	return s, nil
}

// End 结束一个 span：补 end_ms、状态与属性。
func (t *Tracer) End(s Span, status string, attrs map[string]any) error {
	if status == "" {
		status = "ok"
	}
	if t.rec == nil {
		return nil
	}
	return t.rec.FinishSpan(s.TraceID, s.SpanID, t.now(), status, attrs)
}

// NewID 生成 16 字节随机十六进制 ID。
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 极端情况下退化为时间派生（仍保证唯一性概率）。
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b[:])
}

// NowMs 当前毫秒时间戳。
func NowMs() int64 { return time.Now().UnixMilli() }
