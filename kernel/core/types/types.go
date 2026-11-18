// Package types 定义内核基础数据契约：Event、Message、ContentBlock、
// ToolCall、StreamEvent、ModelCapabilities。开源区，纯标准库，无业务逻辑。
//
// 序列化契约（v1 冻结，T-M1-01）：
//   - 时间戳一律 int64 UnixMilli；JSON 数字直解 int64，不经浮点，无精度损失。
//   - ContentBlock 用 Type 判别字段 + omitempty 可选字段，不用 interface/oneof，
//     兼容 gomobile JSON 字符串过桥（过桥方案 M1 裁决）。
//   - ToolCall.Arguments 为 JSON 字符串，不归一化，字节级往返。
//   - 切片字段不加 omitempty：nil 与空切片语义有别（null 与 []）。
//   - Unmarshal 忽略未知字段（向前兼容）；Validate 是结构门禁，
//     失败返回哨兵错误包装字段路径，调用方用 errors.Is 分类。
package types

import (
	"encoding/json"
	"errors"
	"fmt"
)

// 哨兵错误：Validate 失败的分类原因，包装时附字段路径。
var (
	ErrEmptyField   = errors.New("types: required field empty")
	ErrInvalidRole  = errors.New("types: invalid role")
	ErrInvalidBlock = errors.New("types: invalid block type")
	ErrInvalidKind  = errors.New("types: invalid stream kind")
	ErrBadArguments = errors.New("types: arguments not valid JSON")
	ErrNegative     = errors.New("types: negative value")
)

// Role 消息发送者角色。
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Valid 报告 r 是否为已知角色。
func (r Role) Valid() bool {
	switch r {
	case RoleSystem, RoleUser, RoleAssistant, RoleTool:
		return true
	}
	return false
}

// BlockType 内容块判别字段。
type BlockType string

const (
	BlockText       BlockType = "text"
	BlockImage      BlockType = "image"
	BlockAudio      BlockType = "audio"
	BlockReasoning  BlockType = "reasoning"
	BlockToolUse    BlockType = "tool_use"
	BlockToolResult BlockType = "tool_result"
)

// Valid 报告 b 是否为已知块类型。
func (b BlockType) Valid() bool {
	switch b {
	case BlockText, BlockImage, BlockAudio, BlockReasoning, BlockToolUse, BlockToolResult:
		return true
	}
	return false
}

// StreamKind 归一化流事件种类（适配器 SSE 输出契约）。
type StreamKind string

const (
	StreamDelta     StreamKind = "delta"
	StreamReasoning StreamKind = "reasoning"
	StreamToolCall  StreamKind = "tool_call"
	StreamDone      StreamKind = "done"
	StreamError     StreamKind = "error"
)

// Valid 报告 k 是否为已知流事件种类。
func (k StreamKind) Valid() bool {
	switch k {
	case StreamDelta, StreamReasoning, StreamToolCall, StreamDone, StreamError:
		return true
	}
	return false
}

// Event 是 bus 门铃（批准优化#4）：只带主题与快照版本号，
// 订阅者收铃拉最新快照，容忍丢铃；不携带业务 payload。
type Event struct {
	Topic     string `json:"topic"`
	Version   int64  `json:"version"`
	Source    string `json:"source"`
	Timestamp int64  `json:"timestamp"`
}

// Validate 检查门铃结构完整性。
func (e Event) Validate() error {
	if e.Topic == "" {
		return fmt.Errorf("%w: event.topic", ErrEmptyField)
	}
	if e.Source == "" {
		return fmt.Errorf("%w: event.source", ErrEmptyField)
	}
	if e.Version < 0 {
		return fmt.Errorf("%w: event.version", ErrNegative)
	}
	return nil
}

// ToolCall 一次工具调用。Arguments 为原始 JSON 字符串，空串表示无参数。
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Validate 检查调用字段；Arguments 非空时必须是合法 JSON。
func (t ToolCall) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("%w: tool_call.id", ErrEmptyField)
	}
	if t.Name == "" {
		return fmt.Errorf("%w: tool_call.name", ErrEmptyField)
	}
	if t.Arguments != "" && !json.Valid([]byte(t.Arguments)) {
		return fmt.Errorf("%w: tool_call.arguments", ErrBadArguments)
	}
	return nil
}

// ContentBlock 单结构体 + Type 判别，可选字段按类型使用：
// text/reasoning 用 Text；image/audio 用 MIMEType+Data；
// tool_use 用 Call；tool_result 用 CallID+Text（IsError 标失败）。
type ContentBlock struct {
	Type     BlockType `json:"type"`
	Text     string    `json:"text,omitempty"`
	MIMEType string    `json:"mime_type,omitempty"`
	Data     []byte    `json:"data,omitempty"`
	Call     *ToolCall `json:"call,omitempty"`
	CallID   string    `json:"call_id,omitempty"`
	IsError  bool      `json:"is_error,omitempty"`
}

// Validate 按判别类型检查必填字段。Text 允许为空（流式构建中合法）。
func (b ContentBlock) Validate() error {
	if !b.Type.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidBlock, b.Type)
	}
	switch b.Type {
	case BlockImage, BlockAudio:
		if b.MIMEType == "" {
			return fmt.Errorf("%w: block.mime_type", ErrEmptyField)
		}
		if len(b.Data) == 0 {
			return fmt.Errorf("%w: block.data", ErrEmptyField)
		}
	case BlockToolUse:
		if b.Call == nil {
			return fmt.Errorf("%w: block.call", ErrEmptyField)
		}
		if err := b.Call.Validate(); err != nil {
			return err
		}
	case BlockToolResult:
		if b.CallID == "" {
			return fmt.Errorf("%w: block.call_id", ErrEmptyField)
		}
	}
	return nil
}

// Message 一条消息。CreatedAt 为 UnixMilli；Model 为应答模型标记（灾备路由用）。
// Blocks 为空在流式构建中合法，落库门禁由调用方决定。
type Message struct {
	ID        string         `json:"id"`
	SessionID string         `json:"session_id"`
	Role      Role           `json:"role"`
	Blocks    []ContentBlock `json:"blocks"`
	Model     string         `json:"model,omitempty"`
	CreatedAt int64          `json:"created_at"`
}

// Validate 检查消息结构，逐块递归校验。
func (m Message) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("%w: message.id", ErrEmptyField)
	}
	if m.SessionID == "" {
		return fmt.Errorf("%w: message.session_id", ErrEmptyField)
	}
	if !m.Role.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidRole, m.Role)
	}
	for i, b := range m.Blocks {
		if err := b.Validate(); err != nil {
			return fmt.Errorf("message.blocks[%d]: %w", i, err)
		}
	}
	return nil
}

// StreamEvent 归一化流事件。error 种类必须带 ErrCode（失败分类，不许静默成功）；
// Model 为实际应答模型（中转掉包检测，增补 T-M1-06）；RequestID 供排障。
type StreamEvent struct {
	Kind      StreamKind `json:"kind"`
	Text      string     `json:"text,omitempty"`
	Call      *ToolCall  `json:"call,omitempty"`
	CallIndex int        `json:"call_index"`
	ErrCode   string     `json:"err_code,omitempty"`
	RequestID string     `json:"request_id,omitempty"`
	Model     string     `json:"model,omitempty"`
}

// Validate 检查流事件结构。
func (s StreamEvent) Validate() error {
	if !s.Kind.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidKind, s.Kind)
	}
	switch s.Kind {
	case StreamError:
		if s.ErrCode == "" {
			return fmt.Errorf("%w: stream_event.err_code", ErrEmptyField)
		}
	case StreamToolCall:
		if s.Call == nil {
			return fmt.Errorf("%w: stream_event.call", ErrEmptyField)
		}
		if err := s.Call.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ModelCapabilities 模型能力位：视觉/工具/推理/向量（embedding）+ 流式，
// 懒校验粒度按增补 T-M1-06。false 表示不支持或未探测，显式序列化不省略。
type ModelCapabilities struct {
	ModelID   string `json:"model_id"`
	Vision    bool   `json:"vision"`
	Tools     bool   `json:"tools"`
	Reasoning bool   `json:"reasoning"`
	Embedding bool   `json:"embedding"`
	Streaming bool   `json:"streaming"`
}

// Validate 检查模型 id 非空；能力位全 false 合法（尚未探测）。
func (c ModelCapabilities) Validate() error {
	if c.ModelID == "" {
		return fmt.Errorf("%w: model_capabilities.model_id", ErrEmptyField)
	}
	return nil
}
