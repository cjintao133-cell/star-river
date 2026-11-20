package openai

import (
	"strings"
	"testing"

	"star-river/kernel/core/types"
)

// 工具调用参数跨 chunk 累计超过 maxToolArgsBytes 必须报错中止（防参数膨胀刷爆内存）。
func TestConsumeStreamToolArgsLimit(t *testing.T) {
	c := &Client{apiKey: "k"}
	seg := strings.Repeat("A", 100*1024) // 100KB/块
	var b strings.Builder
	for i := 0; i < 12; i++ { // 12×100KB=1.2MB > 1MB(单行仍<1MiB)
		b.WriteString(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"","arguments":"` + seg + `"}}]}}]}` + "\n\n")
	}
	b.WriteString("data: [DONE]\n\n")
	_, err := c.consumeStream(strings.NewReader(b.String()), "rid", func(types.StreamEvent) {})
	if err == nil {
		t.Fatal("应因工具参数超限报错")
	}
	ae, ok := err.(*APIError)
	if !ok || !strings.Contains(ae.Message, "arguments exceeded") {
		t.Fatalf("err=%v", err)
	}
}

// 总正文超过 maxStreamTextBytes 必须报错（跨多 chunk 累积）。
func TestConsumeStreamTextLimit(t *testing.T) {
	c := &Client{apiKey: "k"}
	seg := strings.Repeat("x", 900*1024) // 900KB/块 < 1MiB/行
	var b strings.Builder
	for i := 0; i < 10; i++ { // 10×900KB=9MB > 8MB
		b.WriteString(`data: {"choices":[{"delta":{"content":"` + seg + `"}}]}` + "\n\n")
	}
	b.WriteString("data: [DONE]\n\n")
	_, err := c.consumeStream(strings.NewReader(b.String()), "rid", func(types.StreamEvent) {})
	if err == nil {
		t.Fatal("应因正文超限报错")
	}
	if ae, ok := err.(*APIError); !ok || !strings.Contains(ae.Message, "size/event limit") {
		t.Fatalf("err=%v", err)
	}
}
