package mobilebind

// 工具调用循环（"主脑会用 MCP 工具"主心骨，T-M6-03）+ 工具发现（接入 kernel/tooldisc 两段式）。
//
// 上下文优化（防撑爆）：
//   - 工具总数 <= inlineToolCap：直接下发全部 schema（简单场景）。
//   - 工具总数 >  inlineToolCap：只下发 tool_search 元工具 + 按用户消息关键词预筛的少量工具；
//     模型可用 tool_search 搜索其余工具，命中后下一轮再懒取 schema 注入。footprint 有界。
//
// 红线：只用开源底座 kernel/{adapter/openai,tool,tooldisc,core/types,orchestrator}，不 import 伴侣插件。

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"star-river/kernel/adapter/openai"
	"star-river/kernel/core/types"
	"star-river/kernel/orchestrator"
	"star-river/kernel/tool"
	"star-river/kernel/tooldisc"
)

const (
	toolLoopMaxRounds = 4
	inlineToolCap     = 24 // 超过此数量则启用两段式发现
	prefilterLimit    = 12 // 两段式下按需预筛下发条数
)

var (
	tdMu  sync.Mutex
	tdIdx *tooldisc.Index
)

// toolIndex 返回 tooldisc 索引（基于内核 SQLite，惰性建表）。无 store 时返回 nil。
func toolIndex() *tooldisc.Index {
	tdMu.Lock()
	defer tdMu.Unlock()
	if tdIdx != nil {
		return tdIdx
	}
	kbMu.Lock()
	st := kbStore
	kbMu.Unlock()
	if st == nil {
		return nil
	}
	ix, err := tooldisc.NewIndex(st)
	if err != nil {
		return nil
	}
	tdIdx = ix
	return tdIdx
}

// syncIndex 把当前启用插件工具同步进索引（按 server 整体替换）。返回 (总工具数, 存在的server集合)。
func syncIndex(ctx context.Context, qts []tool.QualifiedTool) int {
	ix := toolIndex()
	if ix == nil {
		return len(qts)
	}
	byServer := map[string][]tooldisc.IndexedTool{}
	for _, qt := range qts {
		byServer[qt.Server] = append(byServer[qt.Server], tooldisc.IndexedTool{Tool: qt.Tool})
	}
	for srv, items := range byServer {
		_ = ix.ReplaceServer(ctx, srv, items)
	}
	return len(qts)
}

// scoreTool 关键词重叠打分（name/title/desc 命中用户词）。
func scoreTool(m tooldisc.ToolMeta, terms []string) int {
	if len(terms) == 0 {
		return 0
	}
	hay := strings.ToLower(m.Name + " " + m.Title + " " + m.ShortDesc + " " + m.Category)
	score := 0
	for _, t := range terms {
		if t != "" && strings.Contains(hay, t) {
			score++
		}
	}
	return score
}

// flattenResult 把 MCP 工具结果压成文本（仅 text 内容拼接；其余忽略）。
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

// keywords 从用户文本粗切关键词（空白/标点分隔，保底用小写子串）。
func keywords(s string) []string {
	s = strings.ToLower(s)
	f := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '\n' || r == '\t' || r == '，' || r == '。' || r == ',' || r == '.' || r == '?' || r == '！' || r == '!'
	})
	out := make([]string, 0, len(f)*2)
	for _, w := range f {
		r := []rune(w)
		if len(r) < 2 {
			continue
		}
		out = append(out, w)
		// 中文无空格：额外生成 2 字 bigram，提升与中文描述的匹配（英文名仍可能不中，靠 tool_search 兜底）
		if len(r) >= 2 && hasCJK(w) {
			for i := 0; i+2 <= len(r); i++ {
				out = append(out, string(r[i:i+2]))
			}
		}
	}
	return out
}

// hasCJK 是否含中日韩字符。
func hasCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

// toolPlan 计算本轮下发的工具定义（可能为空=不带工具）。
func toolPlan(ctx context.Context, qts []tool.QualifiedTool, userMsg string) []openai.ToolDef {
	total := syncIndex(ctx, qts)
	defs := make([]openai.ToolDef, 0, len(qts))
	for _, qt := range qts {
		if qt.Tool.Name == "" {
			continue
		}
		defs = append(defs, openai.ToolDef{Name: qt.Tool.Name, Description: qt.Tool.Description, Parameters: qt.Tool.InputSchema})
	}
	if total <= inlineToolCap {
		return defs // 小量：直发全部
	}
	// 大量：tool_search + 关键词预筛
	ix := toolIndex()
	if ix == nil {
		return defs // 无索引可用：退回直发（保持可用性）
	}
	out := []openai.ToolDef{searchToolDef(ix)}
	metas, _ := ix.Search(ctx, "", tooldisc.SearchOptions{Limit: 500})
	terms := keywords(userMsg)
	sort.SliceStable(metas, func(i, j int) bool {
		return scoreTool(metas[i], terms) > scoreTool(metas[j], terms)
	})
	added := 0
	for _, m := range metas {
		if added >= prefilterLimit {
			break
		}
		if m.Name == tooldisc.SearchToolName {
			continue
		}
		if sc, ok, serr := ix.Schema(ctx, m.Server, m.Name); ok && serr == nil && len(sc) > 0 {
			out = append(out, openai.ToolDef{Name: m.Name, Description: m.ShortDesc, Parameters: sc})
			added++
		}
	}
	return out
}

// searchToolDef 把 tooldisc 的 tool_search 定义转为 openai.ToolDef。
func searchToolDef(ix *tooldisc.Index) openai.ToolDef {
	d := tooldisc.NewSearchTool(ix).Definition()
	return openai.ToolDef{Name: d.Name, Description: d.Description, Parameters: d.InputSchema}
}

// ChatStreamTools 带工具调用的流式对话（纯文本用户消息）。
func ChatStreamTools(baseURL, apiKey, model, system, user string, sink ChatSink) {
	if sink == nil {
		return
	}
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(model) == "" {
		sink.OnError("endpoint/model 未配置")
		return
	}
	send := buildSender(baseURL, apiKey)
	sys := assembleSystem(system, user) // 注入情绪语气 + 相关记忆
	msgs := orchestrator.BuildMinimal(sys, nil, userMessage(user), orchestrator.Limits{MaxMessages: 20, MaxChars: 8000, MaxPersonaChars: 12000, MaxUserChars: 8000})
	runChatLoop(send, apiKey, model, msgs, user, &capSink{inner: sink, user: user})
}

// ChatStreamImage 多模态：用户消息带一张图（base64）+ 文本，仍走工具循环。
func ChatStreamImage(baseURL, apiKey, model, system, user, imageB64, imageMIME string, sink ChatSink) {
	if sink == nil {
		return
	}
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(model) == "" {
		sink.OnError("endpoint/model 未配置")
		return
	}
	um := userMessage(user)
	if img, ok := decodeImage(imageB64); ok {
		if strings.TrimSpace(imageMIME) == "" {
			imageMIME = "image/png"
		}
		imgBlock := types.ContentBlock{Type: types.BlockImage, MIMEType: imageMIME, Data: img}
		um.Blocks = append([]types.ContentBlock{imgBlock}, um.Blocks...)
	}
	send := buildSender(baseURL, apiKey)
	sys := assembleSystem(system, user)
	msgs := orchestrator.BuildMinimal(sys, nil, um, orchestrator.Limits{MaxMessages: 20, MaxChars: 8000, MaxPersonaChars: 12000, MaxUserChars: 8000})
	runChatLoop(send, apiKey, model, msgs, user, &capSink{inner: sink, user: user})
}

// decodeImage 解 base64 图像（去 data URI 前缀）。空/非法返回 false。
func decodeImage(b64 string) ([]byte, bool) {
	b64 = strings.TrimSpace(b64)
	if b64 == "" {
		return nil, false
	}
	if i := strings.Index(b64, ","); i >= 0 && strings.Contains(b64[:i], "base64") {
		b64 = b64[i+1:]
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return data, true
}

// runChatLoop 执行工具调用循环。sink 收全部正文增量。
func runChatLoop(send chatSend, apiKey, model string, msgs []types.Message, userMsg string, sink ChatSink) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	mcpMu.Lock()
	mgr := mcpEnsure()
	qts := mgr.Tools()
	mcpMu.Unlock()
	defs := toolPlan(ctx, qts, userMsg)
	owner := make(map[string]string, len(qts))
	for _, qt := range qts {
		owner[qt.Tool.Name] = qt.Server
	}
	ix := toolIndex()

	for round := 0; round < toolLoopMaxRounds; round++ {
		var roundText strings.Builder
		var calls []types.ToolCall
		emitFn := func(e types.StreamEvent) {
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
		}
		modelName, err := send(ctx, model, msgs, defs, emitFn)
		// 兜底：模型不支持 tools → 去掉工具重试一次。
		if err != nil && len(defs) > 0 && roundText.Len() == 0 && len(calls) == 0 {
			defs = nil
			modelName, err = send(ctx, model, msgs, nil, emitFn)
		}
		if err != nil {
			sink.OnError(redactKey(err.Error(), apiKey))
			return
		}
		if len(calls) == 0 {
			sink.OnDone(modelName)
			return
		}
		asst := types.Message{Role: types.RoleAssistant}
		if roundText.Len() > 0 {
			asst.Blocks = append(asst.Blocks, types.ContentBlock{Type: types.BlockText, Text: roundText.String()})
		}
		for i := range calls {
			asst.Blocks = append(asst.Blocks, types.ContentBlock{Type: types.BlockToolUse, Call: &calls[i]})
		}
		msgs = append(msgs, asst)

		have := map[string]bool{}
		for _, d := range defs {
			have[d.Name] = true
		}
		for _, call := range calls {
			if call.Name == tooldisc.SearchToolName && ix != nil {
				// 两段式第一步：执行 tool_search，并懒加载命中工具的 schema 供下一轮
				res2, _ := tooldisc.NewSearchTool(ix).Execute(ctx, json.RawMessage(call.Arguments))
				txt := flattenResult(res2)
				msgs = append(msgs, types.Message{Role: types.RoleTool, Blocks: []types.ContentBlock{
					{Type: types.BlockToolResult, CallID: call.ID, Text: txt},
				}})
				defs = lazyAddDiscovered(ctx, ix, call.Arguments, defs, have)
				continue
			}
			text, isErr := runToolCall(ctx, mgr, owner, call)
			msgs = append(msgs, types.Message{Role: types.RoleTool, Blocks: []types.ContentBlock{
				{Type: types.BlockToolResult, CallID: call.ID, Text: text, IsError: isErr},
			}})
		}
	}
	sink.OnDone(model)
}

// lazyAddDiscovered 依据 tool_search 的 query 命中项，把尚未下发的工具 schema 追加到 defs（去重有上限）。
func lazyAddDiscovered(ctx context.Context, ix *tooldisc.Index, argsJSON string, defs []openai.ToolDef, have map[string]bool) []openai.ToolDef {
	var p struct {
		Query string `json:"query"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &p)
	metas, err := ix.Search(ctx, p.Query, tooldisc.SearchOptions{Limit: prefilterLimit})
	if err != nil {
		return defs
	}
	add := 0
	for _, m := range metas {
		if add >= prefilterLimit {
			break
		}
		if have[m.Name] || m.Name == tooldisc.SearchToolName {
			continue
		}
		if sc, ok, serr := ix.Schema(ctx, m.Server, m.Name); ok && serr == nil && len(sc) > 0 {
			defs = append(defs, openai.ToolDef{Name: m.Name, Description: m.ShortDesc, Parameters: sc})
			have[m.Name] = true
			add++
		}
	}
	return defs
}

// runToolCall 执行一次 MCP 工具调用，返回文本与是否错误。
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

// capSink 包裹真实 sink：累积正文，结束时记录一轮（记忆编码 + 情绪演化）。
type capSink struct {
	inner ChatSink
	user  string
	buf   strings.Builder
}

func (c *capSink) OnDelta(t string) { c.buf.WriteString(t); c.inner.OnDelta(t) }
func (c *capSink) OnDone(m string)  { observeTurn(c.user, c.buf.String()); c.inner.OnDone(m) }
func (c *capSink) OnError(e string) { c.inner.OnError(e) }
