package gemini

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// wirePart 响应/流内内容片段。text 用 Text；thought=true 为推理片段（thinking）；
// functionCall 为工具调用（Gemini 不返回 call id，由本包合成稳定 id）。
type wirePart struct {
	Text         string            `json:"text,omitempty"`
	Thought      bool              `json:"thought,omitempty"`
	FunctionCall *wireFunctionCall `json:"functionCall,omitempty"`
}

// wireFunctionCall 工具调用：args 为 JSON 对象。
type wireFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

// wireContent 一条 turn 的内容（roles: user / model）。
type wireContent struct {
	Role  string     `json:"role,omitempty"`
	Parts []wirePart `json:"parts"`
}

// wireResponseCandidate 一个候选回答。
type wireResponseCandidate struct {
	Content      wireContent `json:"content"`
	FinishReason string      `json:"finishReason"`
	Index        int         `json:"index"`
}

// wireUsageMetadata token 用量（T-M3-07 统计数据源）。
type wireUsageMetadata struct {
	PromptTokenCount     int64 `json:"promptTokenCount"`
	CandidatesTokenCount int64 `json:"candidatesTokenCount"`
	TotalTokenCount      int64 `json:"totalTokenCount"`
}

// wirePromptFeedback 提示被拦截（无候选时的内容拒绝信号）。
type wirePromptFeedback struct {
	BlockReason string `json:"blockReason"`
}

// wireGenerateResponse generateContent / streamGenerateContent 的响应chunk外壳。
// 增补⑤：HTTP 200 也可能混入 error，必须判失败。
type wireGenerateResponse struct {
	Candidates     []wireResponseCandidate `json:"candidates"`
	UsageMetadata  *wireUsageMetadata      `json:"usageMetadata,omitempty"`
	PromptFeedback *wirePromptFeedback     `json:"promptFeedback,omitempty"`
	Error          *wireError              `json:"error,omitempty"`
}

// scanSSE 逐行解析 Gemini alt=sse 事件流：
//   - 空行/注释行(:) 忽略；只认 data: 行；每行 data 为完整 JSON（GenerateContentResponse chunk）。
//   - 无 [DONE] 哨兵；流终止由调用方识别 finishReason 判定（见 consumeStream）。
//   - 流内 error 字段 → KindStream 失败，不许静默成功（增补⑤）。
//   - 坏 JSON / 超 1MiB 行 → KindProtocol；读失败 → 传输分类。
func scanSSE(r io.Reader, apiKey string, onChunk func(wireGenerateResponse) error) error {
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
		if data == "" || data == "[DONE]" {
			continue
		}
		var ch wireGenerateResponse
		if err := json.Unmarshal([]byte(data), &ch); err != nil {
			return &APIError{Kind: KindProtocol, Message: "sse data not JSON: " + redact(data, apiKey)}
		}
		if ch.Error != nil {
			return &APIError{Kind: KindStream, Code: ch.Error.Status, Message: redact(ch.Error.Message, apiKey)}
		}
		if err := onChunk(ch); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return &APIError{Kind: KindProtocol, Message: "sse line exceeds 1MiB"}
		}
		return transportError(err, apiKey)
	}
	return nil
}
