package mobilebind

// P1 MCP Server 模式桥：把内核自身能力暴露为一组 MCP 工具（外部 agent 可调用本内核）。
// 传输层由宿主决定；这里提供单条 JSON-RPC 处理 McpServerHandle + 工具清单 McpServerManifest。

import (
	"encoding/json"

	"github.com/cjintao133-cell/star-river/kernel/mcpserver"
)

func mcpToolDefs() []mcpserver.ToolDef {
	obj := func(props map[string]any, req ...string) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": req}
	}
	str := map[string]any{"type": "string"}
	num := map[string]any{"type": "number"}
	return []mcpserver.ToolDef{
		{Name: "memory_search", Description: "语义/关键词检索本地长期记忆", InputSchema: obj(map[string]any{"query": str, "k": num}, "query")},
		{Name: "memory_add", Description: "写入一条分层记忆（semantic/episodic/procedural）", InputSchema: obj(map[string]any{"kind": str, "text": str, "importance": num}, "text")},
		{Name: "task_list", Description: "列出任务/目标（可按会话）", InputSchema: obj(map[string]any{"session_id": str, "limit": num})},
		{Name: "task_get", Description: "取单个任务详情", InputSchema: obj(map[string]any{"id": str}, "id")},
		{Name: "skills_list", Description: "列出本地技能包元数据", InputSchema: obj(map[string]any{})},
		{Name: "skills_match", Description: "按查询检索技能包", InputSchema: obj(map[string]any{"query": str, "k": num}, "query")},
		{Name: "sessions_list", Description: "列出会话", InputSchema: obj(map[string]any{})},
		{Name: "characters_list", Description: "列出角色", InputSchema: obj(map[string]any{})},
	}
}

func argStr(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func argInt(args map[string]any, key string, def int) int {
	if args == nil {
		return def
	}
	if v, ok := args[key]; ok {
		if f, ok := v.(float64); ok {
			return int(f)
		}
	}
	return def
}

func mcpExec(name string, args map[string]any) (string, bool, error) {
	switch name {
	case "memory_search":
		return SearchMemory(argStr(args, "query"), argInt(args, "k", 5)), false, nil
	case "memory_add":
		imp := 0.5
		if v, ok := args["importance"].(float64); ok {
			imp = v
		}
		r := AddMemory(argStr(args, "kind"), argStr(args, "text"), imp)
		return r, false, nil
	case "task_list":
		return TaskList(argStr(args, "session_id"), argInt(args, "limit", 50)), false, nil
	case "task_get":
		return TaskGet(argStr(args, "id")), false, nil
	case "skills_list":
		return SkillsList(), false, nil
	case "skills_match":
		return SkillsMatch(argStr(args, "query"), argInt(args, "k", 5)), false, nil
	case "sessions_list":
		return ListSessions(), false, nil
	case "characters_list":
		return ListCharacters(), false, nil
	default:
		return "unknown tool: " + name, true, nil
	}
}

// McpServerManifest 返回工具清单 JSON（供外部 MCP 客户端发现能力）。
func McpServerManifest() string {
	b, _ := json.Marshal(map[string]any{
		"name":            "star-river",
		"protocolVersion": mcpserver.DefaultProtocolVersion,
		"tools":           mcpToolDefs(),
	})
	return string(b)
}

// McpServerHandle 处理一条 MCP JSON-RPC 请求，返回响应串（通知无响应时返回空串）。
func McpServerHandle(reqJSON string) string {
	srv := mcpserver.New(mcpserver.Config{Name: "star-river", Version: "0.1.0", Tools: mcpToolDefs()}, mcpExec)
	resp, has := srv.Handle(reqJSON)
	if !has {
		return ""
	}
	return resp
}
