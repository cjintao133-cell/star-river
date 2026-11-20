package visual

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// capturingBackend 记录最近的 GenerateRequest（验证垫图/强度/提示词注入）。
type capturingBackend struct {
	lastRequest GenerateRequest
	imgs        [][]byte
}

func (c *capturingBackend) Name() string { return "capture" }
func (c *capturingBackend) Generate(_ context.Context, req GenerateRequest) (GenerateResult, error) {
	c.lastRequest = req
	return GenerateResult{Images: c.imgs, Model: "capture"}, nil
}

func TestSelfieTemplateValidate(t *testing.T) {
	if err := DefaultSelfieTemplate().Validate(); err != nil {
		t.Fatalf("默认模板应合法: %v", err)
	}
	bad := []SelfieTemplate{
		{ID: "", BasePrompt: "x", Scenes: []SelfieScene{{Key: "a", Prompt: "p"}}},
		{ID: "i", BasePrompt: "", Scenes: []SelfieScene{{Key: "a", Prompt: "p"}}},
		{ID: "i", BasePrompt: "x"},
		{ID: "i", BasePrompt: "x", Scenes: []SelfieScene{{Key: "", Prompt: "p"}}},
		{ID: "i", BasePrompt: "x", Scenes: []SelfieScene{{Key: "a", Prompt: "p"}, {Key: "a", Prompt: "q"}}},
		{ID: "i", BasePrompt: "x", Scenes: []SelfieScene{{Key: "a", Prompt: "p"}}, Strength: 2},
	}
	for i, b := range bad {
		if err := b.Validate(); !errors.Is(err, ErrRecipe) {
			t.Errorf("case %d 应报 ErrRecipe, got %v", i, err)
		}
	}
}

func TestSelfieAvailableGate(t *testing.T) {
	svc, _ := NewService(&capturingBackend{imgs: [][]byte{testPNG(t, 8, 8)}}, NewWatermarker(), 1)
	p, err := NewSelfiePipeline(svc, DefaultSelfieTemplate(), 1)
	if err != nil {
		t.Fatal(err)
	}
	// 无人设图 → 置灰 + 提示
	ok, hint := p.Available(false)
	if ok || hint == "" {
		t.Errorf("无人设图应置灰且有提示: %v %q", ok, hint)
	}
	// 有人设图 → 可用
	if ok, _ := p.Available(true); !ok {
		t.Error("有人设图应可用")
	}
}

func TestSelfieSubmitWithoutImageDisabled(t *testing.T) {
	svc, _ := NewService(&capturingBackend{imgs: [][]byte{testPNG(t, 8, 8)}}, NewWatermarker(), 1)
	p, _ := NewSelfiePipeline(svc, DefaultSelfieTemplate(), 1)
	_, err := p.Submit(context.Background(), nil, "")
	if !errors.Is(err, ErrNoCharacterImage) {
		t.Fatalf("无人设图应返回 ErrNoCharacterImage, got %v", err)
	}
	var de *SelfieDisabledError
	if !errors.As(err, &de) || de.Hint == "" {
		t.Error("应带解锁提示")
	}
	if svc.Gallery().Len() != 0 {
		t.Error("置灰时不应生成")
	}
}

func TestSelfieSubmitWithImage(t *testing.T) {
	cb := &capturingBackend{imgs: [][]byte{testPNG(t, 40, 40)}}
	svc, _ := NewService(cb, NewWatermarker(), 1)
	tpl := DefaultSelfieTemplate()
	p, _ := NewSelfiePipeline(svc, tpl, 42)
	// 指定场景，便于断言
	jobID, err := p.Submit(context.Background(), []byte("persona-image-bytes"), "cafe")
	if err != nil {
		t.Fatal(err)
	}
	job, err := svc.Wait(context.Background(), jobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != JobSucceeded {
		t.Fatalf("应成功: %s %s", job.Status, job.Error)
	}
	// 垫图应被注入
	if string(cb.lastRequest.ImageRef) != "persona-image-bytes" {
		t.Errorf("人设图垫图未注入: %q", cb.lastRequest.ImageRef)
	}
	// 高强度垫图
	if cb.lastRequest.Strength != tpl.Strength {
		t.Errorf("强度应为 %v, got %v", tpl.Strength, cb.lastRequest.Strength)
	}
	// 第一人称构图 + 场景内容进了提示词
	if !strings.Contains(cb.lastRequest.Prompt, "第一人称") || !strings.Contains(cb.lastRequest.Prompt, "咖啡馆") {
		t.Errorf("提示词不对: %q", cb.lastRequest.Prompt)
	}
	// 产出仍带水印入库
	if svc.Gallery().Len() != 1 {
		t.Errorf("应入库 1 张, got %d", svc.Gallery().Len())
	}
	for _, it := range svc.Gallery().List() {
		got, _ := svc.Gallery().Get(it.ID)
		if !HasAIWatermark(got.Data) {
			t.Error("自拍产物也应带 AI 水印")
		}
	}
}

func TestSelfieSceneRandom(t *testing.T) {
	// 同 seed 应确定性；不同 sceneKey 越界报错。
	svc, _ := NewService(&capturingBackend{imgs: [][]byte{testPNG(t, 8, 8)}}, NewWatermarker(), 1)
	p1, _ := NewSelfiePipeline(svc, DefaultSelfieTemplate(), 7)
	s1 := p1.pickScene()
	p2, _ := NewSelfiePipeline(svc, DefaultSelfieTemplate(), 7)
	s2 := p2.pickScene()
	if s1.Key != s2.Key {
		t.Errorf("同 seed 场景应一致: %s vs %s", s1.Key, s2.Key)
	}
	// 未知场景
	if _, err := p1.Submit(context.Background(), []byte("x"), "nope"); !errors.Is(err, ErrRecipe) {
		t.Errorf("未知场景应报错, got %v", err)
	}
}

func TestSelfieNilService(t *testing.T) {
	if _, err := NewSelfiePipeline(nil, DefaultSelfieTemplate(), 1); !errors.Is(err, ErrNoBackend) {
		t.Errorf("nil service 应报错, got %v", err)
	}
	// 非法模板
	if _, err := NewSelfiePipeline(&Service{}, SelfieTemplate{ID: "x"}, 1); !errors.Is(err, ErrRecipe) {
		t.Errorf("非法模板应报错, got %v", err)
	}
}

func TestSelfieTemplateSerializable(t *testing.T) {
	// 模板 JSON 可序列化往返（数据驱动/可整表替换）。
	b, err := json.Marshal(DefaultSelfieTemplate())
	if err != nil {
		t.Fatal(err)
	}
	var back SelfieTemplate
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if err := back.Validate(); err != nil {
		t.Errorf("往返后应仍合法: %v", err)
	}
}
