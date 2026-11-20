package mobilebind

// T-M2-04：聊天流式桥。把内核 openai 适配器 + 最小上下文经 gomobile 回调暴露给 Android UI。
// 纪律：桥层只做薄门面（拼装 + 转发），内核逻辑不在桥层。
// gomobile 类型限制：导出函数只收有限类型；流式经 ChatSink 接口回调（Kotlin 侧实现）。

import (
	"context"
	"strings"

	"star-river/kernel/adapter/openai"
	"star-river/kernel/core/types"
	"star-river/kernel/orchestrator"
)

// ChatSink 流式回调。Kotlin 侧实现该接口，接收增量文本与结束/错误。
// 约定：成功时先若干次 OnDelta，最后一次 OnDone；失败时仅一次 OnError（不调 OnDone）。
type ChatSink interface {
	OnDelta(text string)
	OnDone(model string)
	OnError(msg string)
}

// redactKey 从错误串里抹掉 key（红线 3，双保险）。
func redactKey(s, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, "[REDACTED]")
}

func userMessage(text string) types.Message {
	return types.Message{Role: types.RoleUser, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: text}}}
}

// ChatStream 同步执行一次流式对话：装配"人设 + 本次用户消息"最小上下文，
// 经 openai 适配器流式调用，边收边回调 sink。本函数阻塞至结束（Kotlin 须在后台线程调用）。
// 失败回调 OnError（已脱敏），不抛异常、不调 OnDone。
func ChatStream(baseURL, apiKey, model, system, user string, sink ChatSink) {
	if sink == nil {
		return
	}
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(model) == "" {
		sink.OnError("endpoint/model 未配置")
		return
	}
	c := openai.New(baseURL, apiKey)
	req := openai.ChatRequest{
		Model:     model,
		MaxTokens: 1024,
		Messages:  orchestrator.BuildMinimal(system, nil, userMessage(user), orchestrator.Limits{MaxMessages: 20, MaxChars: 8000}),
	}
	res, err := c.Chat(context.Background(), req, func(e types.StreamEvent) {
		switch e.Kind {
		case types.StreamDelta:
			if e.Text != "" {
				sink.OnDelta(e.Text)
			}
		case types.StreamReasoning:
			// 推理过程不下发 UI（避免污染正文）。
		case types.StreamError:
			// 错误经 Chat 返回值统一 OnError，此处不重复。
		}
	})
	if err != nil {
		sink.OnError(redactKey(err.Error(), apiKey))
		return
	}
	sink.OnDone(res.Model)
}

// ListModels 拉取模型列表，返回 JSON 数组字符串（每项 {"id":...}），供 UI 模型选择。
// 失败返回错误信息字符串（以 "ERR:" 前缀，UI 据此提示）。空列表返回 "[]"。
func ListModels(baseURL, apiKey string) string {
	if strings.TrimSpace(baseURL) == "" {
		return "ERR:endpoint 未配置"
	}
	c := openai.New(baseURL, apiKey)
	ms, err := c.Models(context.Background())
	if err != nil {
		return "ERR:" + redactKey(err.Error(), apiKey)
	}
	// 手拼 JSON（受控：仅 id 字段，做最小转义）。
	var b strings.Builder
	b.WriteByte('[')
	for i, m := range ms {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"id":"`)
		b.WriteString(jsonEscape(m.ID))
		b.WriteString(`"}`)
	}
	b.WriteByte(']')
	return b.String()
}

// jsonEscape 最小 JSON 字符串转义（模型 id 一般为安全字符，这里保守兜底）。
func jsonEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				b.WriteString(" ")
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}
