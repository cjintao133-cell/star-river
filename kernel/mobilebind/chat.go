package mobilebind

// T-M2-04：聊天流式桥。把内核 openai 适配器 + 最小上下文经 gomobile 回调暴露给 Android UI。
// 纪律：桥层只做薄门面（拼装 + 转发），内核逻辑不在桥层。
// gomobile 类型限制：导出函数只收有限类型；流式经 ChatSink 接口回调（Kotlin 侧实现）。

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cjintao133-cell/star-river/kernel/adapter/openai"
	"github.com/cjintao133-cell/star-river/kernel/core/types"
	"github.com/cjintao133-cell/star-river/kernel/orchestrator"
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
	c := openai.NewWithHeaders(baseURL, apiKey, currentHeaders())
	req := openai.ChatRequest{
		Model:     model,
		MaxTokens: 1024,
		Messages:  orchestrator.BuildMinimal(system, nil, userMessage(user), orchestrator.Limits{MaxMessages: 20, MaxChars: 8000, MaxPersonaChars: 12000, MaxUserChars: 8000}),
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

// UsageSink 可选的用量回调：模型返回后回调一条用量（含缓存命中）。
type UsageSink interface {
	OnUsage(promptTokens int64, cachedTokens int64, completionTokens int64)
}

// chatParams 采样参数（JSON 解析，缺省字段留 nil=不传）。
type chatParams struct {
	Temperature      *float64 `json:"temperature"`
	TopP             *float64 `json:"top_p"`
	MaxTokens        int      `json:"max_tokens"`
	FrequencyPenalty *float64 `json:"frequency_penalty"`
	PresencePenalty  *float64 `json:"presence_penalty"`
}

// ChatStreamEx 带采样参数与用量回调的流式对话（ChatStream 的超集）。
// paramsJSON 如 {"temperature":0.7,"top_p":0.9,"max_tokens":4096}，可空。
// usage 可空；非空时末尾回调 OnUsage（promptTokens/cachedTokens/completionTokens）。
func ChatStreamEx(baseURL, apiKey, model, system, user, paramsJSON string, sink ChatSink, usage UsageSink) {
	if sink == nil {
		return
	}
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(model) == "" {
		sink.OnError("endpoint/model 未配置")
		return
	}
	var pp chatParams
	if strings.TrimSpace(paramsJSON) != "" {
		_ = json.Unmarshal([]byte(paramsJSON), &pp)
	}
	maxTok := pp.MaxTokens
	if maxTok <= 0 {
		maxTok = 4096
	}
	c := openai.NewWithHeaders(baseURL, apiKey, currentHeaders())
	req := openai.ChatRequest{
		Model:            model,
		MaxTokens:        maxTok,
		Messages:         orchestrator.BuildMinimal(system, nil, userMessage(user), orchestrator.Limits{MaxMessages: 20, MaxChars: 8000, MaxPersonaChars: 12000, MaxUserChars: 8000}),
		Temperature:      pp.Temperature,
		TopP:             pp.TopP,
		FrequencyPenalty: pp.FrequencyPenalty,
		PresencePenalty:  pp.PresencePenalty,
	}
	res, err := c.Chat(context.Background(), req, func(e types.StreamEvent) {
		switch e.Kind {
		case types.StreamDelta:
			if e.Text != "" {
				sink.OnDelta(e.Text)
			}
		case types.StreamError:
			// 统一经返回值 OnError
		}
	})
	if err != nil {
		sink.OnError(redactKey(err.Error(), apiKey))
		return
	}
	if usage != nil {
		usage.OnUsage(res.Usage.PromptTokens, res.Usage.CachedTokens, res.Usage.CompletionTokens)
	}
	sink.OnDone(res.Model)
}
