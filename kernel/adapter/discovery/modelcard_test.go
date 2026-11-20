package discovery

import (
	"testing"

	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

func TestBuildCardClassification(t *testing.T) {
	// 分类应透传 Classify 结果与来源。
	c := BuildCard(ModelMeta{ID: "gpt-4o", Provider: "openai"}, nil)
	if c.Category != CategoryFlagship || c.ClassSource != SourceMetadata {
		t.Errorf("分类不对: %+v", c)
	}
	c2 := BuildCard(ModelMeta{ID: "some-random-model-xyz"}, nil)
	if c2.Category != CategoryUnknown || c2.ClassSource != SourceFallback {
		t.Errorf("未知分类不对: %+v", c2)
	}
}

func TestBuildCardHeuristicCaps(t *testing.T) {
	// 视觉：gemini 命中
	c := BuildCard(ModelMeta{ID: "gemini-1.5-pro"}, nil)
	if !c.Caps.Vision || c.sourceOf(CapVision) != ConfHeuristic {
		t.Errorf("gemini 应推测视觉: %+v", c)
	}
	// 推理
	c = BuildCard(ModelMeta{ID: "deepseek-r1"}, nil)
	if !c.Caps.Reasoning || c.sourceOf(CapReasoning) != ConfHeuristic {
		t.Errorf("r1 应推测推理: %+v", c)
	}
	// embedding 专属
	c = BuildCard(ModelMeta{ID: "text-embedding-3-large"}, nil)
	if !c.Caps.Embedding || c.Caps.Vision || c.Caps.Tools {
		t.Errorf("embedding 模型只应 embedding: %+v", c)
	}
	// 流式默认推测
	c = BuildCard(ModelMeta{ID: "whatever"}, nil)
	if c.sourceOf(CapStreaming) != ConfHeuristic || !c.Caps.Streaming {
		t.Errorf("流式默认推测: %+v", c)
	}
}

func TestBuildCardMetadataModality(t *testing.T) {
	// 元数据 input_modalities 含 image → 视觉（metadata 级，压过启发式）。
	c := BuildCard(ModelMeta{ID: "some-model", InputModalities: []string{"text", "image"}}, nil)
	if !c.Caps.Vision || c.sourceOf(CapVision) != ConfMetadata {
		t.Errorf("元数据模态应点亮视觉(metadata): %+v", c)
	}
	if len(c.Modalities) != 2 {
		t.Errorf("模态合并不对: %v", c.Modalities)
	}
}

func TestBuildCardProbeOverrides(t *testing.T) {
	// 实测覆盖：meta 无视觉，但探测说有 → probed 压过一切。
	probe := &types.ModelCapabilities{Vision: true, Tools: true, Streaming: true}
	c := BuildCard(ModelMeta{ID: "plain-model"}, probe)
	if !c.Caps.Vision || c.sourceOf(CapVision) != ConfProbed {
		t.Errorf("探测应压过启发式为 probed: %+v", c)
	}
	if c.sourceOf(CapTools) != ConfProbed {
		t.Errorf("tools 应 probed: %+v", c)
	}
}

func TestBuildCardContextLength(t *testing.T) {
	c := BuildCard(ModelMeta{ID: "m", ContextLength: 128000}, nil)
	if c.ContextLength != 128000 {
		t.Errorf("上下文长度应透传: %d", c.ContextLength)
	}
}

func TestBuildCards(t *testing.T) {
	metas := []ModelMeta{{ID: "gpt-4o"}, {ID: "text-embedding-3-small"}, {ID: "x"}}
	// 只对 gpt-4o 实测
	probeFn := func(id string) (*types.ModelCapabilities, bool) {
		if id == "gpt-4o" {
			return &types.ModelCapabilities{Vision: true, Tools: true, Streaming: true}, true
		}
		return nil, false
	}
	cards := BuildCards(metas, probeFn)
	if len(cards) != 3 {
		t.Fatalf("应 3 张卡, got %d", len(cards))
	}
	if cards[0].sourceOf(CapVision) != ConfProbed {
		t.Errorf("gpt-4o 应 probed: %+v", cards[0])
	}
	if !cards[1].Caps.Embedding {
		t.Errorf("embedding 卡不对: %+v", cards[1])
	}
}

func TestHeuristicCapsQwenVL(t *testing.T) {
	h := HeuristicCaps("qwen2.5-vl-72b")
	if !h[CapVision] || !h[CapTools] {
		t.Errorf("qwen-vl 应含视觉+工具: %v", h)
	}
}
