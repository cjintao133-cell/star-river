package anthropic

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// wireUsage Anthropic 用量：input_tokens / output_tokens（非 openai 的 prompt/completion）。
type wireUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// wireContentBlock 响应/流内内容块。text 用 Text；tool_use 用 ID/Name/Input；
// thinking 用 Thinking（推理模型扩展，非流式整块）。
type wireContentBlock struct {
	Type     string          `json:"type"`
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Text     string          `json:"text"`
	Thinking string          `json:"thinking"`
	Input    json.RawMessage `json:"input"`
}

// wireDelta 流增量。text_delta 用 Text；input_json_delta 用 PartialJSON（tool_use 参数续段）；
// thinking_delta 用 Thinking；message_delta.delta.stop_reason 用 StopReason。
type wireDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	PartialJSON string `json:"partial_json"`
	Thinking    string `json:"thinking"`
	StopReason  string `json:"stop_reason"`
}

// wireMessage message_start 事件里的 message 外壳（含初始 usage）。
type wireMessage struct {
	ID    string    `json:"id"`
	Model string    `json:"model"`
	Usage wireUsage `json:"usage"`
}

// wireEvent Anthropic SSE 事件。event: 行的类型与 data.type 一致，本包统一按 data.type 分发
// （更可靠，不信任单独 event 行）。
type wireEvent struct {
	Type         string            `json:"type"`
	Index        int               `json:"index"`
	Message      *wireMessage      `json:"message"`
	ContentBlock *wireContentBlock `json:"content_block"`
	Delta        *wireDelta        `json:"delta"`
	Usage        *wireUsage        `json:"usage"`
	Error        *wireError        `json:"error"`
}

// scanEvents 逐行解析 Anthropic SSE（event:/data: 两行一组）：
//   - 空行/注释行(:) 忽略；event:/id:/retry: 行忽略，只认 data: 行。
//   - 每行 data 为完整 JSON 事件，解析后回调；坏 JSON / 超 1MiB 行 → KindProtocol。
//   - 无 [DONE] 哨兵；流终止由调用方识别 message_stop 事件判定（见 consumeStream）。
//   - 读失败 → 传输分类（transportError）。
//
// bufio 按行缓冲，网络 chunk 把 UTF-8 多字节切半不影响解析。
// 所有返回错误均为 *APIError（已脱敏）。
func scanEvents(r io.Reader, apiKey string, onEvent func(wireEvent) error) error {
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
		if data == "" {
			continue
		}
		var ev wireEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return &APIError{Kind: KindProtocol, Message: "sse data not JSON: " + redact(data, apiKey)}
		}
		if err := onEvent(ev); err != nil {
			if errors.Is(err, errStopScan) {
				return nil
			}
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

// errStopScan 内部哨兵：调用方收到终止事件（message_stop）后可提前结束扫描，
// 避免等待连接 EOF 导致把已完成的回复误判为超时。
var errStopScan = errors.New("anthropic: stop scan")
