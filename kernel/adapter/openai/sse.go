package openai

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// wireUsage token 用量（T-M3-07 统计数据源）。
type wireUsage struct {
	PromptTokens         int64 `json:"prompt_tokens"`
	CompletionTokens     int64 `json:"completion_tokens"`
	TotalTokens          int64 `json:"total_tokens"`
	PromptCacheHitTokens int64 `json:"prompt_cache_hit_tokens"` // DeepSeek 风格
	PromptTokensDetails  *struct {
		CachedTokens int64 `json:"cached_tokens"` // OpenAI 风格
	} `json:"prompt_tokens_details"`
}

// wireToolCall 流式 tool_calls 增量片段：首片带 id/name，后续片只带 arguments 续段。
type wireToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// wireDelta 流式增量（delta）与非流式整消息（message）共用结构。
// reasoning_content 为推理模型扩展字段（DeepSeek 系）。
type wireDelta struct {
	Role             string           `json:"role"`
	Content          string           `json:"content"`
	ReasoningContent string           `json:"reasoning_content"`
	ToolCalls        []wireToolCall   `json:"tool_calls"`
	Annotations      []wireAnnotation `json:"annotations"`
}

// wireAnnotation 网页搜索引用标注（url_citation）。
type wireAnnotation struct {
	Type        string `json:"type"`
	URLCitation struct {
		URL   string `json:"url"`
		Title string `json:"title"`
	} `json:"url_citation"`
}

type wireChoice struct {
	Index        int       `json:"index"`
	Delta        wireDelta `json:"delta"`
	Message      wireDelta `json:"message"`
	FinishReason string    `json:"finish_reason"`
}

// wireChunk 流式 chunk 与非流式 completion 外壳共用。
// 增补⑤：HTTP 200 也可能带 error（new-api#6594），必须判失败。
type wireChunk struct {
	ID      string       `json:"id"`
	Model   string       `json:"model"`
	Choices []wireChoice `json:"choices"`
	Usage   *wireUsage   `json:"usage"`
	Error   *wireError   `json:"error"`
}

// scanSSE 逐行解析 SSE（增补⑤容忍差异）：
//   - 空行/注释行(:)/event:/id: 行忽略；只认 data: 行。
//   - data: [DONE] → 正常终止，返回 true。
//   - 缺 [DONE] 由调用方结合 finish_reason 判定（EOF 不在此处报错）。
//   - 末尾空 choices 的 usage chunk 正常上抛，不算错。
//   - 流内 error 字段 → KindStream 失败，不许静默成功。
//   - 坏 JSON / 超 1MiB 行 → KindProtocol；读失败 → 传输分类。
//
// bufio 按行缓冲，网络 chunk 把 UTF-8 多字节切半不影响解析。
// 所有返回错误均为 *APIError（已脱敏）。
func scanSSE(r io.Reader, apiKey string, onChunk func(wireChunk) error) (bool, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return true, nil
		}
		var ch wireChunk
		if err := json.Unmarshal([]byte(data), &ch); err != nil {
			return false, &APIError{Kind: KindProtocol, Message: "sse chunk not JSON: " + redact(data, apiKey)}
		}
		if ch.Error != nil {
			return false, &APIError{Kind: KindStream, Code: ch.Error.CodeString(), Message: redact(ch.Error.Message, apiKey)}
		}
		if err := onChunk(ch); err != nil {
			return false, err
		}
	}
	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return false, &APIError{Kind: KindProtocol, Message: "sse line exceeds 1MiB"}
		}
		return false, transportError(err, apiKey)
	}
	return false, nil
}
