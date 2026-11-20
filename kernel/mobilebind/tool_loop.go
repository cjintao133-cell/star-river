package mobilebind

// 工具调用循环（"主脑会用 MCP 工具"的主心骨，T-M6-03 主脑调用链路）。
// 流程：把启用插件的工具定义喂给模型 → 模型若发起 tool_calls → 经 kernel/tool.Manager
// 执行 → 结果回填 → 再次对话，直至无工具调用（或达轮数上限）。全程脱敏错误。
// 红线：只用开源底座 kernel/{adapter/openai,tool,core/types,orchestrator}，不 import 闭源插件。

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"star-river/kernel/adapter/openai"
	"star-river/kernel/core/types"
	"star-river/kernel/orchestrator"
	"star-river/kernel/tool"
)

const toolLoopMaxRounds = 4

// toolSnapshot 取当前启用插件的工具定义 + 工具名→服务器 归属表（加锁后释放再调用）。 */
func toolSnapshot() ([]openai.ToolDef, map[string]string, *tool.Manager) {
	mcpMu.Lock()
	mgr := mcpEnsure()
	qts := mgr.Tools()
	mcpMu.Unlock()

	defs := make([]openai.ToolDef, 0, len(qts))
	owner := make(map[string]string, len(qts))
	for _, qt := range qts {
		if qt.Tool.Name == "" {
			continue
		}
		owner[qt.Tool.Name] = qt.Server
		defs = append(defs, openai.ToolDef{Name: qt.Tool.Name, Description: qt.Tool.Description, Parameters: qt.Tool.InputSchema})
	}
	return defs, owner, mgr
}

// flattenResult 把 MCP 工具结果压成文本（仅 text 内容拼接；其余类型忽略）。
func flattenResult(cr *tool.CallResult) string {
	if cr == nil {
		return ""
	}
	var sb strings.Builder
	for _, c := range cr.Content {
		if c.Type == "text" && c.Text != "" {
			if sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(c.Text)
		}
	}
	if sb.Len() == 0 && len(cr.StructuredContent) > 0 {
		sb.Write(cr.StructuredContent)
	}
	return sb.String()
}

// ChatStreamTools 带工具调用的流式对话。无工具时等价普通对话。sink 收全部正文增量。
func ChatStreamTools(baseURL, apiKey, model, system, user string, sink ChatSink) {
	if sink == nil {
		return
	}
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(model) == "" {
		sink.OnError("endpoint/model 未配置")
		return
	}
	c := openai.New(baseURL, apiKey)
	msgs := orchestrator.BuildMinimal(system, nil, userMessage(user), orchestrator.Limits{MaxMessages: 20, MaxChars: 8000})
	defs, owner, mgr := toolSnapshot()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	for round := 0; round < toolLoopMaxRounds; round++ {
		var roundText strings.Builder
		var calls []types.ToolCall
		req := openai.ChatRequest{Model: model, MaxTokens: 1024, Messages: msgs, Tools: defs}
		res, err := c.Chat(ctx, req, func(e types.StreamEvent) {
			switch e.Kind {
			case types.StreamDelta:
				if e.Text != "" {
					roundText.WriteString(e.Text)
					sink.OnDelta(e.Text)
				}
			case types.StreamToolCall:
				if e.Call != nil {
					calls = append(calls, *e.Call)
				}
			}
		})
		if err != nil {
			sink.OnError(redactKey(err.Error(), apiKey))
			return
		}
		if len(calls) == 0 {
			sink.OnDone(res.Model)
			return
		}
		// 组装 assistant 消息（正文 + 工具调用块）
		asst := types.Message{Role: types.RoleAssistant}
		if roundText.Len() > 0 {
			asst.Blocks = append(asst.Blocks, types.ContentBlock{Type: types.BlockText, Text: roundText.String()})
		}
		for i := range calls {
			asst.Blocks = append(asst.Blocks, types.ContentBlock{Type: types.BlockToolUse, Call: &calls[i]})
		}
		msgs = append(msgs, asst)
		// 逐个执行并回填工具结果
		for _, call := range calls {
			text, isErr := runToolCall(ctx, mgr, owner, call)
			msgs = append(msgs, types.Message{Role: types.RoleTool, Blocks: []types.ContentBlock{
				{Type: types.BlockToolResult, CallID: call.ID, Text: text, IsError: isErr},
			}})
		}
	}
	// 达轮数上限：把已生成文本收尾（结束事件仅一次）。
	sink.OnDone(model)
}

// runToolCall 执行一次工具调用，返回文本与是否错误。
func runToolCall(ctx context.Context, mgr *tool.Manager, owner map[string]string, call types.ToolCall) (string, bool) {
	server, ok := owner[call.Name]
	if !ok {
		return "unknown tool: " + call.Name, true
	}
	var args map[string]any
	if strings.TrimSpace(call.Arguments) != "" {
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			return "bad tool arguments: " + err.Error(), true
		}
	}
	cr, err := mgr.CallTool(ctx, server, call.Name, args)
	if err != nil {
		return "tool error: " + err.Error(), true
	}
	return flattenResult(cr), cr.IsError
}
