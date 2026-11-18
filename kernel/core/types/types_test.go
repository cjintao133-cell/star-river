package types

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
)

// roundTrip 序列化再反序列化到同类型新值，断言 DeepEqual 无损。
func roundTrip(t *testing.T, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := reflect.New(reflect.TypeOf(v))
	if err := json.Unmarshal(data, out.Interface()); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := out.Elem().Interface(); !reflect.DeepEqual(v, got) {
		t.Fatalf("round trip mismatch:\n in: %#v\nout: %#v\njson: %s", v, got, data)
	}
}

// sampleBlocks 六种块类型各一，含中文/emoji/二进制数据，全部结构合法。
func sampleBlocks() []ContentBlock {
	return []ContentBlock{
		{Type: BlockText, Text: "你好 中文 🌌"},
		{Type: BlockReasoning, Text: "想一想…"},
		{Type: BlockImage, MIMEType: "image/png", Data: []byte{0x00, 0x89, 0x50, 0xFF}},
		{Type: BlockAudio, MIMEType: "audio/wav", Data: []byte{0x52, 0x49, 0x46, 0x46}},
		{Type: BlockToolUse, Call: &ToolCall{ID: "c1", Name: "memory_query", Arguments: `{"q" : "x"}`}},
		{Type: BlockToolResult, CallID: "c1", Text: "结果", IsError: true},
	}
}

func TestEventRoundTrip(t *testing.T) {
	roundTrip(t, Event{Topic: "memory.updated", Version: 42, Source: "adapter", Timestamp: 1731900000123})
}

func TestMessageRoundTrip(t *testing.T) {
	roundTrip(t, Message{
		ID: "m1", SessionID: "s1", Role: RoleAssistant,
		Blocks: sampleBlocks(), Model: "model-x", CreatedAt: 1731900000123,
	})
}

func TestContentBlockRoundTrip(t *testing.T) {
	for _, b := range sampleBlocks() {
		t.Run(string(b.Type), func(t *testing.T) { roundTrip(t, b) })
	}
}

func TestToolCallRoundTripPreservesArguments(t *testing.T) {
	// 设计锚点：Arguments 不归一化，空白与键序字节级保留。
	raw := "{ \"b\" : 2, \"a\" : [1, 2] }"
	c := ToolCall{ID: "c1", Name: "n", Arguments: raw}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got ToolCall
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Arguments != raw {
		t.Fatalf("arguments must survive byte-for-byte:\n in: %q\nout: %q", raw, got.Arguments)
	}
	roundTrip(t, c)
}

func TestStreamEventRoundTrip(t *testing.T) {
	cases := []StreamEvent{
		{Kind: StreamDelta, Text: "增量 中文"},
		{Kind: StreamReasoning, Text: "推理"},
		{Kind: StreamToolCall, Call: &ToolCall{ID: "c1", Name: "n", Arguments: "{}"}, CallIndex: 2},
		{Kind: StreamDone, Model: "model-x", RequestID: "req-1"},
		{Kind: StreamError, ErrCode: "rate_limited", RequestID: "req-2", Text: "429"},
	}
	for _, s := range cases {
		t.Run(string(s.Kind), func(t *testing.T) { roundTrip(t, s) })
	}
}

func TestModelCapabilitiesRoundTrip(t *testing.T) {
	roundTrip(t, ModelCapabilities{ModelID: "m", Vision: true, Tools: true, Reasoning: true, Embedding: true, Streaming: true})
	roundTrip(t, ModelCapabilities{ModelID: "m"})
}

func TestNilAndEmptyBlocksRoundTrip(t *testing.T) {
	// 契约：切片不加 omitempty，nil 与空切片往返后语义不混。
	roundTrip(t, Message{ID: "m", SessionID: "s", Role: RoleUser})
	empty := Message{ID: "m", SessionID: "s", Role: RoleUser, Blocks: []ContentBlock{}}
	data, err := json.Marshal(empty)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(data, []byte(`"blocks":[]`)) {
		t.Fatalf("empty blocks must serialize as [] not null: %s", data)
	}
	roundTrip(t, empty)
}

func TestZeroValuesRoundTrip(t *testing.T) {
	roundTrip(t, Event{})
	roundTrip(t, Message{})
	roundTrip(t, ContentBlock{})
	roundTrip(t, ToolCall{})
	roundTrip(t, StreamEvent{})
	roundTrip(t, ModelCapabilities{})
}

func TestInt64PrecisionRoundTrip(t *testing.T) {
	// int64 直解不经 float64，MaxInt64 不丢精度（时间戳选型锚点）。
	roundTrip(t, Message{ID: "m", SessionID: "s", Role: RoleUser, CreatedAt: math.MaxInt64})
}

func TestUTF8RoundTrip(t *testing.T) {
	s := "中文🌌e\u0301 😀"
	roundTrip(t, ContentBlock{Type: BlockText, Text: s})
	roundTrip(t, ToolCall{ID: "c", Name: "n", Arguments: `{"k":"` + s + `"}`})
}

func TestUnknownFieldsIgnored(t *testing.T) {
	// 向前兼容契约：未知字段忽略，不报错。
	var e Event
	raw := `{"topic":"t","version":1,"source":"s","timestamp":0,"future_field":{"x":1}}`
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatalf("unknown field must be ignored: %v", err)
	}
	if e.Topic != "t" || e.Version != 1 {
		t.Fatalf("known fields must decode: %+v", e)
	}
}

func TestUnmarshalMalformedJSON(t *testing.T) {
	// 失败路径：畸形 JSON 六类型全部必须报错，不许静默成功。
	bad := []string{`{`, `[]`, `"str"`, `nullx`}
	targets := []func() any{
		func() any { return &Event{} },
		func() any { return &Message{} },
		func() any { return &ContentBlock{} },
		func() any { return &ToolCall{} },
		func() any { return &StreamEvent{} },
		func() any { return &ModelCapabilities{} },
	}
	for _, s := range bad {
		for _, mk := range targets {
			if err := json.Unmarshal([]byte(s), mk()); err == nil {
				t.Fatalf("malformed %q must fail for %T", s, mk())
			}
		}
	}
}

func TestValidateFailures(t *testing.T) {
	// 失败路径矩阵：每个错误分支断言 errors.Is 分类原因。
	cases := []struct {
		name string
		v    interface{ Validate() error }
		want error
	}{
		{"event_missing_topic", Event{Source: "s"}, ErrEmptyField},
		{"event_missing_source", Event{Topic: "t"}, ErrEmptyField},
		{"event_negative_version", Event{Topic: "t", Source: "s", Version: -1}, ErrNegative},
		{"message_missing_id", Message{SessionID: "s", Role: RoleUser}, ErrEmptyField},
		{"message_missing_session", Message{ID: "m", Role: RoleUser}, ErrEmptyField},
		{"message_bad_role", Message{ID: "m", SessionID: "s", Role: "alien"}, ErrInvalidRole},
		{"message_bad_block", Message{ID: "m", SessionID: "s", Role: RoleUser, Blocks: []ContentBlock{{Type: "hologram"}}}, ErrInvalidBlock},
		{"block_unknown_type", ContentBlock{Type: "hologram"}, ErrInvalidBlock},
		{"block_image_missing_mime", ContentBlock{Type: BlockImage, Data: []byte{1}}, ErrEmptyField},
		{"block_image_missing_data", ContentBlock{Type: BlockImage, MIMEType: "image/png"}, ErrEmptyField},
		{"block_audio_missing_data", ContentBlock{Type: BlockAudio, MIMEType: "audio/wav"}, ErrEmptyField},
		{"block_tool_use_missing_call", ContentBlock{Type: BlockToolUse}, ErrEmptyField},
		{"block_tool_use_bad_call", ContentBlock{Type: BlockToolUse, Call: &ToolCall{Name: "n"}}, ErrEmptyField},
		{"block_tool_result_missing_call_id", ContentBlock{Type: BlockToolResult}, ErrEmptyField},
		{"tool_call_missing_id", ToolCall{Name: "n"}, ErrEmptyField},
		{"tool_call_missing_name", ToolCall{ID: "c"}, ErrEmptyField},
		{"tool_call_bad_arguments", ToolCall{ID: "c", Name: "n", Arguments: "{oops"}, ErrBadArguments},
		{"stream_unknown_kind", StreamEvent{Kind: "teleport"}, ErrInvalidKind},
		{"stream_error_missing_code", StreamEvent{Kind: StreamError}, ErrEmptyField},
		{"stream_tool_call_missing_call", StreamEvent{Kind: StreamToolCall}, ErrEmptyField},
		{"stream_tool_call_bad_call", StreamEvent{Kind: StreamToolCall, Call: &ToolCall{ID: "c"}}, ErrEmptyField},
		{"caps_missing_model_id", ModelCapabilities{}, ErrEmptyField},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.v.Validate()
			if err == nil {
				t.Fatal("expected failure, got nil (silent success)")
			}
			if !errors.Is(err, c.want) {
				t.Fatalf("want %v, got %v", c.want, err)
			}
		})
	}
}

func TestValidateAcceptsValid(t *testing.T) {
	// 正样本：防 Validate 误杀合法结构。
	vs := []interface{ Validate() error }{
		Event{Topic: "t", Version: 0, Source: "s"},
		Message{ID: "m", SessionID: "s", Role: RoleUser, Blocks: sampleBlocks()},
		Message{ID: "m", SessionID: "s", Role: RoleUser},
		ToolCall{ID: "c", Name: "n"},
		ToolCall{ID: "c", Name: "n", Arguments: "{}"},
		ContentBlock{Type: BlockText},
		ContentBlock{Type: BlockReasoning, Text: "r"},
		StreamEvent{Kind: StreamDelta},
		StreamEvent{Kind: StreamDone},
		ModelCapabilities{ModelID: "m"},
	}
	for i, v := range vs {
		if err := v.Validate(); err != nil {
			t.Fatalf("case %d (%T) must pass: %v", i, v, err)
		}
	}
}

func TestEnumValid(t *testing.T) {
	for _, r := range []Role{RoleSystem, RoleUser, RoleAssistant, RoleTool} {
		if !r.Valid() {
			t.Fatalf("role %q must be valid", r)
		}
	}
	if Role("").Valid() || Role("alien").Valid() {
		t.Fatal("unknown role must be invalid")
	}
	for _, b := range []BlockType{BlockText, BlockImage, BlockAudio, BlockReasoning, BlockToolUse, BlockToolResult} {
		if !b.Valid() {
			t.Fatalf("block type %q must be valid", b)
		}
	}
	if BlockType("").Valid() || BlockType("hologram").Valid() {
		t.Fatal("unknown block type must be invalid")
	}
	for _, k := range []StreamKind{StreamDelta, StreamReasoning, StreamToolCall, StreamDone, StreamError} {
		if !k.Valid() {
			t.Fatalf("stream kind %q must be valid", k)
		}
	}
	if StreamKind("").Valid() || StreamKind("teleport").Valid() {
		t.Fatal("unknown stream kind must be invalid")
	}
}
