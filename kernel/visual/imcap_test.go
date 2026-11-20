package visual

import (
	"context"
	"strings"
	"testing"
)

func TestModelCapability(t *testing.T) {
	cases := []struct {
		model      string
		canImage   bool
		canImg2Img bool
	}{
		{"dall-e-3", true, false},
		{"dall-e-2", true, true},
		{"gpt-image-1", true, true},
		{"gemini-2.5-flash-image", true, true},
		{"gemini-3-pro-image", true, true},
		{"Kwai-Kolors/Kolors", true, true},
		{"Qwen/Qwen-Image-Edit", true, true},
		{"whisper-1", false, false},
		{"text-embedding-3-large", false, false},
		{"BAAI/bge-m3", false, false},
		{"", true, true}, // 未知乐观
	}
	for _, c := range cases {
		got := ModelCapability(c.model)
		if got.CanImage != c.canImage || got.CanImg2Img != c.canImg2Img {
			t.Errorf("%q → %+v, want {image=%v img2img=%v}", c.model, got, c.canImage, c.canImg2Img)
		}
	}
}

// 降级纯文生图自拍：不传垫图、提示词含场景+外貌、Extra.degraded=true、仍带水印。
func TestSelfieSubmitTextOnly(t *testing.T) {
	cb := &capturingBackend{imgs: [][]byte{testPNG(t, 40, 40)}}
	svc, _ := NewService(cb, NewWatermarker(), 1)
	p, _ := NewSelfiePipeline(svc, DefaultSelfieTemplate(), 42)
	jobID, err := p.SubmitTextOnly(context.Background(), "cafe", "银色长发，蓝色眼睛")
	if err != nil {
		t.Fatal(err)
	}
	job, err := svc.Wait(context.Background(), jobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != JobSucceeded {
		t.Fatalf("status %s %s", job.Status, job.Error)
	}
	if len(cb.lastRequest.ImageRef) != 0 {
		t.Errorf("降级不应带垫图，got %d bytes", len(cb.lastRequest.ImageRef))
	}
	if !strings.Contains(cb.lastRequest.Prompt, "咖啡馆") || !strings.Contains(cb.lastRequest.Prompt, "银色长发") {
		t.Errorf("提示词应含场景+外貌: %q", cb.lastRequest.Prompt)
	}
	if cb.lastRequest.Extra["degraded"] != true {
		t.Errorf("Extra.degraded 应为 true")
	}
	for _, it := range svc.Gallery().List() {
		got, _ := svc.Gallery().Get(it.ID)
		if !HasAIWatermark(got.Data) {
			t.Error("降级自拍产物也应带 AI 水印")
		}
	}
}
