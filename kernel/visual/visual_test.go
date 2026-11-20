package visual

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"
)

// testPNG 生成一张 w×h 的纯色 PNG。
func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 7), G: uint8(y * 5), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// —— 水印 ——

func TestWatermarkMarksAndDetectable(t *testing.T) {
	raw := testPNG(t, 64, 64)
	if HasAIWatermark(raw) {
		t.Fatal("原始图不应带水印")
	}
	out, mime, err := NewWatermarker().Mark(raw)
	if err != nil {
		t.Fatal(err)
	}
	if mime != "image/png" {
		t.Errorf("mime 应为 png, got %s", mime)
	}
	if !HasAIWatermark(out) {
		t.Error("水印后应可检出 AI 标识")
	}
	// 仍是合法 PNG
	if _, _, err := image.Decode(bytes.NewReader(out)); err != nil {
		t.Errorf("水印后应可解码: %v", err)
	}
	// 像素应被改动（长度/内容变化）
	if bytes.Equal(raw, out) {
		t.Error("水印应改动图像")
	}
}

func TestHasAIWatermarkNegative(t *testing.T) {
	if HasAIWatermark([]byte("not a png")) {
		t.Error("非 PNG 应 false")
	}
	if HasAIWatermark(nil) {
		t.Error("nil 应 false")
	}
}

func TestWatermarkNotImage(t *testing.T) {
	_, _, err := NewWatermarker().Mark([]byte("hello"))
	if !errors.Is(err, ErrNotImage) {
		t.Errorf("非图像应返回 ErrNotImage, got %v", err)
	}
}

func TestWatermarkDefaultParams(t *testing.T) {
	// 零值 Watermarker 也应工作（默认参数）。
	var w Watermarker
	out, _, err := w.Mark(testPNG(t, 32, 32))
	if err != nil || !HasAIWatermark(out) {
		t.Fatalf("零值水印器应工作: %v", err)
	}
}

// —— 配方 ——

func sampleRecipe() Recipe {
	return Recipe{
		ID: "selfie", Name: "自拍",
		PromptTemplate: "{{scene}} 的自拍，二次元风格",
		NegativePrompt: "lowres",
		Slots:          []Slot{{Key: "scene", Label: "场景", Required: true}},
		Params: []Param{
			{Key: "width", Label: "宽", Type: ParamInt, Min: 64, Max: 2048, Default: 512},
			{Key: "height", Label: "高", Type: ParamInt, Min: 64, Max: 2048, Default: 512},
			{Key: "seed", Label: "种子", Type: ParamInt},
			{Key: "strength", Label: "垫图强度", Type: ParamFloat, Min: 0, Max: 1},
		},
		ImageRefPresets: []ImageRefPreset{{Key: "soft", Label: "轻", Strength: 0.3}, {Key: "hard", Label: "重", Strength: 0.8}},
		DefaultModel:    "sdxl",
		DefaultWidth:    512, DefaultHeight: 512,
	}
}

func TestRecipeValidate(t *testing.T) {
	if err := sampleRecipe().Validate(); err != nil {
		t.Fatalf("合法配方应通过: %v", err)
	}
	cases := []Recipe{
		{ID: "", Name: "x", PromptTemplate: "t"},
		{ID: "a", Name: "", PromptTemplate: "t"},
		{ID: "a", Name: "x", PromptTemplate: ""},
		{ID: "a", Name: "x", PromptTemplate: "t", Slots: []Slot{{Key: ""}}},
		{ID: "a", Name: "x", PromptTemplate: "t", Slots: []Slot{{Key: "s"}, {Key: "s"}}},
		{ID: "a", Name: "x", PromptTemplate: "t", Params: []Param{{Key: "p", Type: ParamEnum}}},               // enum 无 options
		{ID: "a", Name: "x", PromptTemplate: "t", Params: []Param{{Key: "p", Type: "weird"}}},                 // 未知类型
		{ID: "a", Name: "x", PromptTemplate: "t", ImageRefPresets: []ImageRefPreset{{Key: "z", Strength: 2}}}, // 强度越界
	}
	for i, r := range cases {
		if err := r.Validate(); !errors.Is(err, ErrRecipe) {
			t.Errorf("case %d 应报 ErrRecipe, got %v", i, err)
		}
	}
}

func TestRecipeBuildRequest(t *testing.T) {
	r := sampleRecipe()
	req, err := r.BuildRequest(map[string]string{
		"scene": "海边", "width": "768", "height": "1024", "seed": "42", "strength": "0.6",
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.Prompt != "海边 的自拍，二次元风格" {
		t.Errorf("prompt 渲染不对: %q", req.Prompt)
	}
	if req.Width != 768 || req.Height != 1024 || req.Seed != 42 {
		t.Errorf("参数应用不对: %+v", req)
	}
	if req.Strength != 0.6 {
		t.Errorf("strength 不对: %v", req.Strength)
	}
	if req.NegativePrompt != "lowres" || req.Model != "sdxl" {
		t.Errorf("负向/模型不对: %+v", req)
	}
}

func TestRecipeRequiredSlot(t *testing.T) {
	r := sampleRecipe()
	if _, err := r.BuildRequest(map[string]string{}); !errors.Is(err, ErrRecipe) {
		t.Errorf("缺必填 slot 应报错, got %v", err)
	}
	// 有默认值则不报错
	r.Slots[0].Default = "默认场景"
	if _, err := r.BuildRequest(nil); err != nil {
		t.Errorf("默认值应补全: %v", err)
	}
}

func TestRecipeDefaultStrength(t *testing.T) {
	r := sampleRecipe()
	if s, ok := r.DefaultImageRefStrength("hard"); !ok || s != 0.8 {
		t.Errorf("预设取强度不对: %v %v", s, ok)
	}
	if _, ok := r.DefaultImageRefStrength("nope"); ok {
		t.Error("未知预设应 false")
	}
}

// —— 画廊 ——

func TestGalleryBasic(t *testing.T) {
	g := NewGallery()
	id := g.Add(GalleryItem{ID: "i1", JobID: "j1", MIME: "image/png", Data: []byte("x"), Watermarked: true})
	if id != "i1" {
		t.Errorf("Add 应返回 id, got %s", id)
	}
	it, ok := g.Get("i1")
	if !ok || !it.Watermarked || string(it.Data) != "x" {
		t.Fatalf("Get 不对: %+v %v", it, ok)
	}
	list := g.List()
	if len(list) != 1 || list[0].Data != nil {
		t.Errorf("List 应不含 Data 且 1 条: %+v", list)
	}
	if g.Len() != 1 {
		t.Errorf("Len 应为 1")
	}
	if _, ok := g.Get("nope"); ok {
		t.Error("不存在应 false")
	}
}

// —— 服务（异步队列 + 强制水印）——

type fakeBackend struct {
	name string
	imgs [][]byte
	err  error
	gate chan struct{} // 可选：阻塞直到关闭（测并发/取消）
}

func (f *fakeBackend) Name() string {
	if f.name == "" {
		return "fake"
	}
	return f.name
}
func (f *fakeBackend) Generate(ctx context.Context, _ GenerateRequest) (GenerateResult, error) {
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return GenerateResult{}, ctx.Err()
		}
	}
	if f.err != nil {
		return GenerateResult{}, f.err
	}
	return GenerateResult{Images: f.imgs, Model: "fake"}, nil
}

func TestServiceSuccessWatermarked(t *testing.T) {
	fb := &fakeBackend{imgs: [][]byte{testPNG(t, 40, 40), testPNG(t, 40, 40)}}
	svc, err := NewService(fb, NewWatermarker(), 2)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	id, err := svc.Submit(ctx, sampleRecipe(), map[string]string{"scene": "夜"})
	if err != nil {
		t.Fatal(err)
	}
	job, err := svc.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != JobSucceeded {
		t.Fatalf("应成功, got %s (%s)", job.Status, job.Error)
	}
	if len(job.ItemIDs) != 2 {
		t.Fatalf("应 2 张, got %d", len(job.ItemIDs))
	}
	// 画廊里每张都应带水印（不可绕过）
	for _, iid := range job.ItemIDs {
		it, ok := svc.Gallery().Get(iid)
		if !ok {
			t.Fatalf("画廊缺条目 %s", iid)
		}
		if !it.Watermarked {
			t.Error("画廊条目应标记水印")
		}
		if !HasAIWatermark(it.Data) {
			t.Error("画廊图像应可检测到 AI 水印")
		}
	}
}

func TestServiceBackendErrorFails(t *testing.T) {
	fb := &fakeBackend{err: &APIError{Kind: "network", Message: "boom"}}
	svc, _ := NewService(fb, NewWatermarker(), 1)
	ctx := context.Background()
	id, _ := svc.Submit(ctx, sampleRecipe(), map[string]string{"scene": "x"})
	job, _ := svc.Wait(ctx, id)
	if job.Status != JobFailed {
		t.Errorf("后端错误应 Failed, got %s", job.Status)
	}
	if svc.Gallery().Len() != 0 {
		t.Error("失败不应有画廊条目")
	}
}

func TestServiceWatermarkFailureBlocksGallery(t *testing.T) {
	// 后端返回非图像字节 → 水印失败 → 任务失败，画廊为空（无法绕过水印入库）。
	fb := &fakeBackend{imgs: [][]byte{[]byte("not-an-image")}}
	svc, _ := NewService(fb, NewWatermarker(), 1)
	ctx := context.Background()
	id, _ := svc.Submit(ctx, sampleRecipe(), map[string]string{"scene": "x"})
	job, _ := svc.Wait(ctx, id)
	if job.Status != JobFailed {
		t.Errorf("水印失败应 Failed, got %s", job.Status)
	}
	if svc.Gallery().Len() != 0 {
		t.Error("未加水印的图像不得入库")
	}
}

func TestServiceSubmitBadRecipe(t *testing.T) {
	svc, _ := NewService(&fakeBackend{}, NewWatermarker(), 1)
	if _, err := svc.Submit(context.Background(), sampleRecipe(), nil); !errors.Is(err, ErrRecipe) {
		t.Errorf("缺必填 slot 应报错, got %v", err)
	}
}

func TestServiceNilBackend(t *testing.T) {
	if _, err := NewService(nil, NewWatermarker(), 1); !errors.Is(err, ErrNoBackend) {
		t.Errorf("无后端应报错, got %v", err)
	}
}

func TestServiceConcurrency(t *testing.T) {
	fb := &fakeBackend{imgs: [][]byte{testPNG(t, 16, 16)}}
	svc, _ := NewService(fb, NewWatermarker(), 2)
	ctx := context.Background()
	var ids []string
	for i := 0; i < 10; i++ {
		id, err := svc.Submit(ctx, sampleRecipe(), map[string]string{"scene": "s"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	for _, id := range ids {
		job, err := svc.Wait(ctx, id)
		if err != nil || job.Status != JobSucceeded {
			t.Fatalf("并发任务应成功: %v %s", err, job.Status)
		}
	}
	if svc.Gallery().Len() != 10 {
		t.Errorf("应 10 张, got %d", svc.Gallery().Len())
	}
}

func TestServiceCancelQueued(t *testing.T) {
	gate := make(chan struct{})
	fb := &fakeBackend{imgs: [][]byte{testPNG(t, 16, 16)}, gate: gate}
	svc, _ := NewService(fb, NewWatermarker(), 1) // 并发 1：第一个占住，第二个排队
	ctx := context.Background()
	id1, _ := svc.Submit(ctx, sampleRecipe(), map[string]string{"scene": "a"})
	id2, _ := svc.Submit(ctx, sampleRecipe(), map[string]string{"scene": "b"})
	// 取消排队中的第二个
	if err := svc.Cancel(id2); err != nil {
		t.Fatal(err)
	}
	close(gate) // 放行第一个及其后
	j1, _ := svc.Wait(ctx, id1)
	if j1.Status != JobSucceeded {
		t.Errorf("第一个应成功, got %s", j1.Status)
	}
	j2, _ := svc.GetJob(id2)
	if j2.Status != JobCanceled {
		t.Errorf("第二个应为取消, got %s", j2.Status)
	}
	if svc.Gallery().Len() != 1 {
		t.Errorf("只有第一个入库, got %d", svc.Gallery().Len())
	}
}

func TestServiceCloseAndUnknown(t *testing.T) {
	svc, _ := NewService(&fakeBackend{imgs: [][]byte{testPNG(t, 8, 8)}}, NewWatermarker(), 1)
	svc.Close()
	if _, err := svc.Submit(context.Background(), sampleRecipe(), map[string]string{"scene": "x"}); err == nil {
		t.Error("关闭后提交应报错")
	}
	if _, err := svc.Wait(context.Background(), "ghost"); err == nil {
		t.Error("未知任务 Wait 应报错")
	}
	if err := svc.Cancel("ghost"); err == nil {
		t.Error("未知任务 Cancel 应报错")
	}
}

func TestServiceWaitTimeout(t *testing.T) {
	gate := make(chan struct{})
	defer close(gate)
	fb := &fakeBackend{imgs: [][]byte{testPNG(t, 8, 8)}, gate: gate}
	svc, _ := NewService(fb, NewWatermarker(), 1)
	id, _ := svc.Submit(context.Background(), sampleRecipe(), map[string]string{"scene": "x"})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := svc.Wait(ctx, id); err == nil {
		t.Error("Wait 超时应返回 ctx 错误")
	}
}
