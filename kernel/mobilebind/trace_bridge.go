package mobilebind

// P0-5 全链路追踪桥：把 kernel/trace 的 span 落到内核 SQLite（traces 表），
// 供 UI/审计/回放查询。零插件依赖，仅 store + trace。

import (
	"context"
	"encoding/json"

	"github.com/cjintao133-cell/star-river/kernel/trace"
)

// storeTraceRec 用内核 SQLite 作 trace 后端。
type storeTraceRec struct{}

func (storeTraceRec) StartSpan(s trace.Span) error {
	st := storeRef()
	if st == nil {
		return nil
	}
	attrs, _ := json.Marshal(s.Attrs)
	_, err := st.Exec(context.Background(),
		`INSERT OR REPLACE INTO traces(trace_id,span_id,parent_id,kind,name,session_id,start_ms,end_ms,status,attrs_json)
         VALUES(?,?,?,?,?,?,?,?,?,?)`,
		s.TraceID, s.SpanID, s.ParentID, s.Kind, s.Name, s.SessionID, s.StartMs, s.EndMs, s.Status, string(attrs))
	return err
}

func (storeTraceRec) FinishSpan(traceID, spanID string, endMs int64, status string, attrs map[string]any) error {
	st := storeRef()
	if st == nil {
		return nil
	}
	ac, _ := json.Marshal(attrs)
	_, err := st.Exec(context.Background(),
		`UPDATE traces SET end_ms=?, status=?, attrs_json=? WHERE trace_id=? AND span_id=?`,
		endMs, status, string(ac), traceID, spanID)
	return err
}

func traceTracer() *trace.Tracer { return trace.NewTracer(storeTraceRec{}) }

// TraceStart 开始一个 span。parentTrace/parentSpan 为空则新开一条 trace。
// 返回 JSON {trace_id, span_id} / "ERR:..."。
func TraceStart(parentTrace, parentSpan, kind, name, sessionID string) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	s, err := traceTracer().Begin(parentTrace, parentSpan, kind, name, sessionID)
	if err != nil {
		return "ERR:" + err.Error()
	}
	b, _ := json.Marshal(map[string]string{"trace_id": s.TraceID, "span_id": s.SpanID})
	return string(b)
}

// TraceEnd 结束一个 span，补状态与属性（attrsJSON 为对象字符串，可空）。
func TraceEnd(traceID, spanID, status, attrsJSON string) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	var attrs map[string]any
	if attrsJSON != "" {
		if err := json.Unmarshal([]byte(attrsJSON), &attrs); err != nil {
			return "ERR:bad attrs json"
		}
	}
	if err := traceTracer().End(trace.Span{TraceID: traceID, SpanID: spanID}, status, attrs); err != nil {
		return "ERR:" + err.Error()
	}
	return "OK"
}

type traceRow struct {
	TraceID   string `json:"trace_id"`
	SpanID    string `json:"span_id"`
	ParentID  string `json:"parent_id,omitempty"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	SessionID string `json:"session_id,omitempty"`
	StartMs   int64  `json:"start_ms"`
	EndMs     int64  `json:"end_ms"`
	Status    string `json:"status"`
	AttrsJSON string `json:"attrs_json,omitempty"`
}

func (r *traceRow) scan(sc func(...any) error) error {
	return sc(&r.TraceID, &r.SpanID, &r.ParentID, &r.Kind, &r.Name, &r.SessionID, &r.StartMs, &r.EndMs, &r.Status, &r.AttrsJSON)
}

const traceCols = `trace_id,span_id,parent_id,kind,name,session_id,start_ms,end_ms,status,attrs_json`

// ListTraces 最近 span 列表（按开始时间倒序）。
func ListTraces(limit int) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	st := storeRef()
	if st == nil {
		return "ERR:no store"
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := st.Query(context.Background(), `SELECT `+traceCols+` FROM traces ORDER BY start_ms DESC LIMIT ?`, limit)
	if err != nil {
		return "ERR:" + err.Error()
	}
	defer rows.Close()
	out := make([]traceRow, 0, limit)
	for rows.Next() {
		var r traceRow
		if err := r.scan(rows.Scan); err != nil {
			return "ERR:" + err.Error()
		}
		out = append(out, r)
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// GetTrace 返回一条 trace 的所有 span（按开始时间正序）。
func GetTrace(traceID string) string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	st := storeRef()
	if st == nil {
		return "ERR:no store"
	}
	rows, err := st.Query(context.Background(), `SELECT `+traceCols+` FROM traces WHERE trace_id=? ORDER BY start_ms ASC`, traceID)
	if err != nil {
		return "ERR:" + err.Error()
	}
	defer rows.Close()
	out := make([]traceRow, 0, 8)
	for rows.Next() {
		var r traceRow
		if err := r.scan(rows.Scan); err != nil {
			return "ERR:" + err.Error()
		}
		out = append(out, r)
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// ExportTrace 导出整条 trace 的 JSON（GetTrace 的别名，语义更明确）。
func ExportTrace(traceID string) string { return GetTrace(traceID) }

// ClearTraces 清空全部 trace。
func ClearTraces() string {
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	st := storeRef()
	if st == nil {
		return "ERR:no store"
	}
	if _, err := st.Exec(context.Background(), `DELETE FROM traces`); err != nil {
		return "ERR:" + err.Error()
	}
	return "OK"
}
