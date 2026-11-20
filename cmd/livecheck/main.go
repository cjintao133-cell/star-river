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
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"time"

	"star-river/kernel/adapter/openai"
	"star-river/kernel/core/types"
	"star-river/kernel/visual"
)

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

// runImage 真实端点 1 次生图→强制水印→断言→落盘。
func runImage(base, key, imgModel, prompt string, w, h int) bool {
	if base == "" || imgModel == "" {
		fmt.Println("[image] 缺 -base/-imgmodel，跳过")
		return false
	}
	backend := &visual.OpenAIImageBackend{BaseURL: base, APIKey: key, Model: imgModel}
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
	flag.Parse()

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
	case "all":
		needReq = 2
	}
	if *run && needReq > *maxReq {
		fmt.Printf("拒绝：%s 需 %d 次计费请求 > -maxreq %d。调大 -maxreq 再跑。\n", *mode, needReq, *maxReq)
		os.Exit(3)
	}

	fmt.Printf("[livecheck] mode=%s run=%v base=%q model=%q imgmodel=%q needReq=%d\n", *mode, *run, *base, *model, *imgModel, needReq)

	if !*run {
		fmt.Println("(dry 不联网) 计划：selftest=本地水印自检；chat=1 次对话；image=1 次生图+水印核。")
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
		ok = runImage(*base, key, *imgModel, *prompt, *w, *h)
	case "all":
		ok = selftest() && ok
		ok = runChat(*base, key, *model) && ok
		ok = runImage(*base, key, *imgModel, *prompt, *w, *h) && ok
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
