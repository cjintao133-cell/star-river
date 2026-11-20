// livecheck：真机级（真实端点/模型）冒烟测试器。年尺度模拟覆盖不到的项在这里真跑：
//   - selftest : 纯本地。合成图→visual.Watermarker.Mark→断言 HasAIWatermark→落盘。免费，验证水印管线。
//   - chat     : 真实端点 1 次最小对话（证明 endpoint/key/协议通）。
//   - image    : 真实端点 1 次生图→强制水印→断言 HasAIWatermark→落盘（自拍/生图管线真跑）。
//   - all      : selftest + chat + image。
//
// 安全：key 只从文件读、不进 argv、不打印；开销硬闸（-maxreq 计费请求数上限）；禁跟随重定向（适配器已 netx）。
// 用法：
//
//	go run ./cmd/livecheck -mode selftest
//	go run ./cmd/livecheck -mode all -run -base https://host/v1 -model gpt-4o-mini -imgmodel gpt-image-1
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"time"

	"github.com/cjintao133-cell/star-river/kernel/adapter/openai"
	"github.com/cjintao133-cell/star-river/kernel/core/types"
	"github.com/cjintao133-cell/star-river/kernel/mobilebind"
	"github.com/cjintao133-cell/star-river/kernel/visual"
)

var gBackend, gDialect string

func readKey(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func redact(s, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, "[REDACTED]")
}

// synthPNG 合成一张 96x96 渐变图（无需外部素材/字体）。
func synthPNG() ([]byte, error) {
	const w, h = 96, 96
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 2), G: uint8(y * 2), B: 160, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// selftest 纯本地验证水印：合成图→Mark→断言可检出→落盘。
func selftest() bool {
	raw, err := synthPNG()
	if err != nil {
		fmt.Println("[selftest] 合成图失败:", err)
		return false
	}
	if visual.HasAIWatermark(raw) {
		fmt.Println("[selftest] ⚠ 合成原图竟已带 AI 水印（不应）")
		return false
	}
	marked, mime, err := visual.NewWatermarker().Mark(raw)
	if err != nil {
		fmt.Println("[selftest] Mark 失败:", err)
		return false
	}
	ok := visual.HasAIWatermark(marked)
	_ = os.MkdirAll("build", 0o755)
	out := "build/livecheck-selftest.png"
	_ = os.WriteFile(out, marked, 0o644)
	fmt.Printf("[selftest] 合成=%dB → 水印后=%dB mime=%s 可检出=%v → %s\n", len(raw), len(marked), mime, ok, out)
	if !ok {
		fmt.Println("[selftest] ❌ 水印不可检出（红线10 风险）")
		return false
	}
	fmt.Println("[selftest] ✅ 水印管线 PASS（Mark 后 HasAIWatermark=true）")
	return true
}

// runSelfie 真·自拍端到端：InitKernel→建角→设人设图→VisualInit(硅基)→VisualSelfie→验水印→落盘。
// 走 App 完全相同的 mobilebind 全链路（含人设图垫图 + 自拍模板 + 强制水印）。
func runSelfie(base, key, imgModel string) bool {
	if base == "" || imgModel == "" {
		fmt.Println("[selfie] 缺 -base/-imgmodel，跳过")
		return false
	}
	dir, err := os.MkdirTemp("", "sr-selfie-")
	if err != nil {
		fmt.Println("[selfie] tmpdir:", err)
		return false
	}
	defer os.RemoveAll(dir)
	ck := make([]byte, 32)
	for i := range ck {
		ck[i] = byte(i + 1)
	}
	if r := mobilebind.InitKernel(dir+"/sr.db", base64.StdEncoding.EncodeToString(ck)); r != "OK" {
		fmt.Println("[selfie] InitKernel:", redact(r, key))
		return false
	}
	defer mobilebind.CloseKernel()
	persona, err := synthPNG()
	if err != nil {
		fmt.Println("[selfie] 合成人设图失败:", err)
		return false
	}
	if r := mobilebind.CreateCharacter(`{"id":"c1","name":"示例角色"}`); r != "OK" {
		fmt.Println("[selfie] CreateCharacter:", redact(r, key))
		return false
	}
	if r := mobilebind.SetCharacterPersona("c1", persona); r != "OK" {
		fmt.Println("[selfie] SetCharacterPersona:", redact(r, key))
		return false
	}
	cfgJSON, _ := json.Marshal(map[string]any{"base_url": base, "api_key": key, "model": imgModel, "backend": gBackend, "dialect": gDialect})
	if r := mobilebind.VisualInit(string(cfgJSON)); r != "OK" {
		fmt.Println("[selfie] VisualInit:", redact(r, key))
		return false
	}
	t0 := time.Now()
	raw := mobilebind.VisualSelfie("c1", "seaside")
	if strings.HasPrefix(raw, "ERR:") {
		fmt.Println("[selfie] ❌ VisualSelfie:", redact(raw, key))
		return false
	}
	var res struct {
		ImageB64 string `json:"image_b64"`
		MIME     string `json:"mime"`
	}
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		fmt.Println("[selfie] 解析结果失败:", redact(err.Error(), key))
		return false
	}
	img, err := base64.StdEncoding.DecodeString(res.ImageB64)
	if err != nil {
		fmt.Println("[selfie] b64 解码失败:", err)
		return false
	}
	wm := visual.HasAIWatermark(img)
	_ = os.MkdirAll("build", 0o755)
	out := "build/livecheck-selfie.png"
	_ = os.WriteFile(out, img, 0o644)
	fmt.Printf("[selfie] 用时=%dms mime=%s bytes=%d 强制水印=%v → %s\n", time.Since(t0).Milliseconds(), res.MIME, len(img), wm, out)
	if !wm {
		fmt.Println("[selfie] ❌ 自拍图未带 AI 水印（红线10）")
		return false
	}
	fmt.Println("[selfie] ✅ 真·自拍全链路(人设图垫图+模板+强制水印) PASS")
	return true
}

// runModels 列出该 key 可用模型（/models，通常免费），标注疑似画图模型，便于挑 imgmodel。
func runModels(base, key string) bool {
	if base == "" {
		fmt.Println("[models] 缺 -base，跳过")
		return false
	}
	c := openai.New(base, key)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ms, err := c.Models(ctx)
	if err != nil {
		fmt.Println("[models] ❌ 失败:", redact(err.Error(), key))
		return false
	}
	fmt.Printf("[models] ✅ 共 %d 个模型\n", len(ms))
	imgHint := []string{"flux", "kolors", "sd", "stable-diffusion", "z-image", "image", "dall", "cogview", "kling", "seedream", "doubao-seedream"}
	for _, m := range ms {
		low := strings.ToLower(m.ID)
		tag := ""
		for _, h := range imgHint {
			if strings.Contains(low, h) {
				tag = "  <== 疑似画图"
				break
			}
		}
		fmt.Printf("  - %s%s\n", m.ID, tag)
	}
	return true
}

// runChat 真实端点 1 次最小对话。
func runChat(base, key, model string) bool {
	if base == "" || model == "" {
		fmt.Println("[chat] 缺 -base/-model，跳过")
		return false
	}
	c := openai.New(base, key)
	req := openai.ChatRequest{
		Model:     model,
		MaxTokens: 32,
		Messages:  []types.Message{{Role: types.RoleUser, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: "只回复两个字：收到"}}}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var sb strings.Builder
	t0 := time.Now()
	res, err := c.Chat(ctx, req, func(e types.StreamEvent) {
		if e.Kind == types.StreamDelta && e.Text != "" {
			sb.WriteString(e.Text)
		}
	})
	if err != nil {
		fmt.Println("[chat] ❌ 失败:", redact(err.Error(), key))
		return false
	}
	reply := strings.TrimSpace(sb.String())
	fmt.Printf("[chat] ✅ model=%s 用时=%dms 回复=%q\n", res.Model, time.Since(t0).Milliseconds(), reply)
	return true
}

// makeImageBackend 按 -backend 选择图像后端。
func makeImageBackend(base, key, model, backendName string) visual.ImageBackend {
	switch strings.ToLower(strings.TrimSpace(backendName)) {
	case "siliconflow", "siliconflow-image", "sf":
		return &visual.SiliconFlowImageBackend{BaseURL: base, APIKey: key, Model: model}
	case "generic", "generic-image", "compatible":
		d := visual.DialectGeneric()
		switch strings.ToLower(strings.TrimSpace(gDialect)) {
		case "openai":
			d = visual.DialectOpenAI()
		case "siliconflow", "sf":
			d = visual.DialectSiliconFlow()
		case "chat", "gemini", "4sapi", "chat-gemini":
			d = visual.DialectChatGemini()
		}
		return &visual.GenericImageBackend{BaseURL: base, APIKey: key, Model: model, Dialect: d}
	default:
		return &visual.OpenAIImageBackend{BaseURL: base, APIKey: key, Model: model}
	}
}

// runEdit 真实端点 img2img（垫图，走 /images/edits，与自拍同路径）：合成人设图→生成→强制水印→断言→落盘。
func runEdit(base, key, imgModel, prompt, backendName string, w, h int) bool {
	if base == "" || imgModel == "" {
		fmt.Println("[edit] 缺 -base/-imgmodel，跳过")
		return false
	}
	raw, err := synthPNG()
	if err != nil {
		fmt.Println("[edit] 合成垫图失败:", err)
		return false
	}
	backend := makeImageBackend(base, key, imgModel, backendName)
	svc, err := visual.NewService(backend, visual.NewWatermarker(), 1)
	if err != nil {
		fmt.Println("[edit] NewService 失败:", redact(err.Error(), key))
		return false
	}
	defer svc.Close()
	if prompt == "" {
		prompt = "把这张图变成示例蓝调、柔光的插画风格"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	t0 := time.Now()
	jobID, err := svc.SubmitRequest(ctx, visual.GenerateRequest{Prompt: prompt, ImageRef: raw, Width: w, Height: h}, "livecheck-edit")
	if err != nil {
		fmt.Println("[edit] 提交失败:", redact(err.Error(), key))
		return false
	}
	job, err := svc.Wait(ctx, jobID)
	if err != nil {
		fmt.Println("[edit] 等待失败:", redact(err.Error(), key))
		return false
	}
	if job.Status != visual.JobSucceeded || len(job.ItemIDs) == 0 {
		fmt.Println("[edit] ❌ 任务未成功:", job.Status, redact(job.Error, key))
		return false
	}
	it, ok := svc.Gallery().Get(job.ItemIDs[0])
	if !ok || len(it.Data) == 0 {
		fmt.Println("[edit] ❌ 画廊无图")
		return false
	}
	wm := visual.HasAIWatermark(it.Data)
	_ = os.MkdirAll("build", 0o755)
	out := "build/livecheck-edit.png"
	_ = os.WriteFile(out, it.Data, 0o644)
	fmt.Printf("[edit] 用时=%dms mime=%s bytes=%d 强制水印=%v → %s\n", time.Since(t0).Milliseconds(), it.MIME, len(it.Data), wm, out)
	if !wm {
		fmt.Println("[edit] ❌ 生成图未带 AI 水印（红线10）")
		return false
	}
	fmt.Println("[edit] ✅ 真机 img2img(垫图/自拍同路径)+强制水印 PASS")
	return true
}

// runImage 真实端点 1 次生图→强制水印→断言→落盘。
func runImage(base, key, imgModel, prompt, backendName string, w, h int) bool {
	if base == "" || imgModel == "" {
		fmt.Println("[image] 缺 -base/-imgmodel，跳过")
		return false
	}
	backend := makeImageBackend(base, key, imgModel, backendName)
	svc, err := visual.NewService(backend, visual.NewWatermarker(), 1)
	if err != nil {
		fmt.Println("[image] NewService 失败:", redact(err.Error(), key))
		return false
	}
	defer svc.Close()
	if prompt == "" {
		prompt = "一个安静的示例场景，柔和蓝光，插画风格"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	t0 := time.Now()
	jobID, err := svc.SubmitRequest(ctx, visual.GenerateRequest{Prompt: prompt, Width: w, Height: h}, "livecheck")
	if err != nil {
		fmt.Println("[image] 提交失败:", redact(err.Error(), key))
		return false
	}
	job, err := svc.Wait(ctx, jobID)
	if err != nil {
		fmt.Println("[image] 等待失败:", redact(err.Error(), key))
		return false
	}
	if job.Status != visual.JobSucceeded || len(job.ItemIDs) == 0 {
		fmt.Println("[image] ❌ 任务未成功:", job.Status, redact(job.Error, key))
		return false
	}
	it, ok := svc.Gallery().Get(job.ItemIDs[0])
	if !ok || len(it.Data) == 0 {
		fmt.Println("[image] ❌ 画廊无图")
		return false
	}
	wm := visual.HasAIWatermark(it.Data)
	_ = os.MkdirAll("build", 0o755)
	out := "build/livecheck-image.png"
	_ = os.WriteFile(out, it.Data, 0o644)
	fmt.Printf("[image] 用时=%dms mime=%s bytes=%d 强制水印=%v → %s\n", time.Since(t0).Milliseconds(), it.MIME, len(it.Data), wm, out)
	if !wm {
		fmt.Println("[image] ❌ 生成图未带 AI 水印（红线10）")
		return false
	}
	fmt.Println("[image] ✅ 真机生图+强制水印 PASS")
	return true
}

func main() {
	keyfile := flag.String("keyfile", "build/.testkey", "API key 文件（600，勿进 git）")
	base := flag.String("base", "", "端点 base_url，如 https://host/v1")
	model := flag.String("model", "", "对话模型")
	imgModel := flag.String("imgmodel", "", "生图模型（如 gpt-image-1 / dall-e-3）")
	mode := flag.String("mode", "selftest", "selftest|chat|image|all")
	run := flag.Bool("run", false, "真实调用（默认 dry：只打印计划，不打网络）")
	maxReq := flag.Int("maxreq", 2, "计费请求数硬上限")
	w := flag.Int("w", 768, "生图宽")
	h := flag.Int("h", 1024, "生图高")
	prompt := flag.String("prompt", "", "生图提示词（空则用默认示例场景）")
	backendName := flag.String("backend", "openai-image", "图像后端: openai-image | siliconflow | generic")
	dialect := flag.String("dialect", "", "generic 方言: openai | siliconflow | generic")
	flag.Parse()
	gBackend, gDialect = *backendName, *dialect

	key := readKey(*keyfile)
	if key == "" && *mode != "selftest" {
		fmt.Println("ERROR: key 文件为空/不存在:", *keyfile)
		os.Exit(2)
	}

	needReq := 0
	switch *mode {
	case "chat":
		needReq = 1
	case "image":
		needReq = 1
	case "selfie":
		needReq = 1
	case "all":
		needReq = 2
	}
	if *run && needReq > *maxReq {
		fmt.Printf("拒绝：%s 需 %d 次计费请求 > -maxreq %d。调大 -maxreq 再跑。\n", *mode, needReq, *maxReq)
		os.Exit(3)
	}

	fmt.Printf("[livecheck] mode=%s run=%v base=%q model=%q imgmodel=%q needReq=%d\n", *mode, *run, *base, *model, *imgModel, needReq)

	if !*run {
		fmt.Println("(dry 不联网) 计划：selftest=本地水印自检；models=列模型(免费)；chat=1 次对话；image=1 次生图+水印核。")
		if *mode == "selftest" || *mode == "all" {
			selftest()
		}
		return
	}

	ok := true
	switch *mode {
	case "selftest":
		ok = selftest()
	case "chat":
		ok = runChat(*base, key, *model)
	case "image":
		ok = runImage(*base, key, *imgModel, *prompt, *backendName, *w, *h)
	case "edit":
		ok = runEdit(*base, key, *imgModel, *prompt, *backendName, *w, *h)
	case "selfie":
		ok = runSelfie(*base, key, *imgModel)
	case "models":
		ok = runModels(*base, key)
	case "all":
		ok = selftest() && ok
		ok = runChat(*base, key, *model) && ok
		ok = runImage(*base, key, *imgModel, *prompt, *backendName, *w, *h) && ok
		ok = runEdit(*base, key, *imgModel, *prompt, *backendName, *w, *h) && ok
	default:
		fmt.Println("未知 mode:", *mode)
		os.Exit(2)
	}
	if ok {
		fmt.Println("LIVECHECK-GREEN")
	} else {
		fmt.Println("LIVECHECK-RED")
		os.Exit(1)
	}
}
