// Package mcpserver 把一组本地工具暴露为 MCP（Model Context Protocol）服务器。
//
// 实现 MCP JSON-RPC 2.0 的核心方法：initialize / notifications/initialized / tools/list /
// tools/call / ping。传输层（stdio 或 HTTP）由入口层决定；本包只做「一条请求进、一条响应出」
// 的纯处理，便于任意宿主复用与单测。零内部依赖（仅标准库）。
package mcpserver

import (
	"encoding/json"
	"errors"
)

// 默认协议版本。
const DefaultProtocolVersion = "2024-11-05"

// ToolDef 一个暴露的工具。
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Executor 执行工具，返回文本结果与是否出错。
type Executor func(name string, args map[string]any) (text string, isError bool, err error)

// StructuredExecutor 可选：返回结构化结果（对齐 MCP 2026 结构化工具输出）。
type StructuredExecutor func(name string, args map[string]any) (text string, structured any, isError bool, err error)

// Config 服务器配置。
type Config struct {
	Name            string
	Version         string
	ProtocolVersion string
	Tools           []ToolDef
	Structured      StructuredExecutor // 可选；设置后 tools/call 返回 structuredContent
}

// Server MCP 服务器。
type Server struct {
	cfg Config
	ex  Executor
}

// New 建服务器。ex 可为 nil（tools/call 将返回未实现）。
func New(cfg Config, ex Executor) *Server {
	if cfg.ProtocolVersion == "" {
		cfg.ProtocolVersion = DefaultProtocolVersion
	}
	if cfg.Name == "" {
		cfg.Name = "star-river"
	}
	return &Server{cfg: cfg, ex: ex}
}

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Handle 处理一条 JSON-RPC 请求。hasResp=false 表示是通知（无需响应）。
func (s *Server) Handle(reqJSON string) (resp string, hasResp bool) {
	var req rpcReq
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return errResp(nil, -32700, "parse error"), true
	}
	isNotification := len(req.ID) == 0 || string(req.ID) == "null"

	switch req.Method {
	case "initialize":
		// 回显客户端请求的协议版本（协商）；未提供则用服务端默认。
		ver := s.cfg.ProtocolVersion
		var ip struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if len(req.Params) > 0 && json.Unmarshal(req.Params, &ip) == nil && ip.ProtocolVersion != "" {
			ver = ip.ProtocolVersion
		}
		return okResp(req.ID, map[string]any{
			"protocolVersion": ver,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": s.cfg.Name, "version": s.cfg.Version},
		}), true

	case "notifications/initialized", "initialized":
		return "", false // 通知，无响应

	case "ping":
		return okResp(req.ID, map[string]any{}), true

	case "tools/list":
		return okResp(req.ID, map[string]any{"tools": s.cfg.Tools}), true

	case "tools/call":
		return s.handleCall(req, isNotification)

	default:
		if isNotification {
			return "", false
		}
		return errResp(req.ID, -32601, "method not found: "+req.Method), true
	}
}

func (s *Server) handleCall(req rpcReq, isNotification bool) (string, bool) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	_ = json.Unmarshal(req.Params, &p)
	if p.Name == "" {
		if isNotification {
			return "", false
		}
		return errResp(req.ID, -32602, "missing tool name"), true
	}
	if s.cfg.Structured != nil {
		text, structured, isErr, err := s.cfg.Structured(p.Name, p.Arguments)
		if err != nil {
			return okResp(req.ID, structuredResult(err.Error(), nil, true)), true
		}
		return okResp(req.ID, structuredResult(text, structured, isErr)), true
	}
	if s.ex == nil {
		return okResp(req.ID, contentResult("MCP 执行器未配置", true)), true
	}
	text, isErr, err := s.ex(p.Name, p.Arguments)
	if err != nil {
		// 工具执行错误按 MCP 约定放 isError=true，而非 JSON-RPC 错误。
		return okResp(req.ID, contentResult(err.Error(), true)), true
	}
	return okResp(req.ID, contentResult(text, isErr)), true
}

func contentResult(text string, isErr bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isErr,
	}
}

// structuredResult 带 structuredContent 的结果（structured 为 nil 时退化为纯文本）。
func structuredResult(text string, structured any, isErr bool) map[string]any {
	m := contentResult(text, isErr)
	if structured != nil {
		m["structuredContent"] = structured
	}
	return m
}

func okResp(id json.RawMessage, result any) string {
	return marshal(map[string]any{"jsonrpc": "2.0", "id": rawID(id), "result": result})
}

func errResp(id json.RawMessage, code int, msg string) string {
	return marshal(map[string]any{"jsonrpc": "2.0", "id": rawID(id), "error": map[string]any{"code": code, "message": msg}})
}

func rawID(id json.RawMessage) any {
	if len(id) == 0 {
		return nil
	}
	return json.RawMessage(id)
}

func marshal(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"marshal error"}}`
	}
	return string(b)
}

// ErrNotImplemented 便于上层。
var ErrNotImplemented = errors.New("mcpserver: not implemented")
