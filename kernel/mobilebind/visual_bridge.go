package mobilebind

// 视觉桥（T-新增）：把 kernel/visual 管线暴露给 UI —— 相册/自拍/生图。
// 强制水印由 visual.Service 内部保证（无关闭 API），此处只做薄门面。
// 默认后端 = 直接画图模型（OpenAI 兼容：有垫图走 /images/edits，无垫图走 /images/generations），
// ComfyUI 仅作为可选后端（需工作流，App 侧暂不构造，报可读错误）。

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"star-river/kernel/visual"
)

var (
	visMu  sync.Mutex
	visSvc *visual.Service
)

type visualCfgInfo struct {
	BaseURL  string            `json:"base_url"`
	APIKey   string            `json:"api_key"`
	Model    string            `json:"model"`
	Backend  string            `json:"backend"`           // "openai-image"(默认) | "comfyui"
	EditPath string            `json:"edit_path"`         // 可选，覆盖默认 /images/edits
	Dialect  string            `json:"dialect,omitempty"` // generic 方言: openai|siliconflow|generic
	Header   map[string]string `json:"header,omitempty"`
}

// VisualInit 初始化视觉管线（配置图像模型）。返回 "OK"/"ERR:..."。
func VisualInit(cfgJSON string) string {
	var cfg visualCfgInfo
	if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
		return "ERR:bad config json: " + err.Error()
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return "ERR:base_url required"
	}
	backendName := strings.ToLower(strings.TrimSpace(cfg.Backend))
	if backendName == "" {
		backendName = "openai-image"
	}
	var backend visual.ImageBackend
	switch backendName {
	case "openai-image", "openai", "direct":
		hdr := http.Header{}
		for k, v := range cfg.Header {
			hdr.Set(k, v)
		}
		backend = &visual.OpenAIImageBackend{
			BaseURL:  cfg.BaseURL,
			APIKey:   cfg.APIKey,
			Model:    cfg.Model,
			EditPath: cfg.EditPath,
			Header:   hdr,
		}
	case "generic", "generic-image", "openai-compatible", "compatible":
		hdr := http.Header{}
		for k, v := range cfg.Header {
			hdr.Set(k, v)
		}
		d := visual.DialectGeneric()
		switch strings.ToLower(strings.TrimSpace(cfg.Dialect)) {
		case "openai":
			d = visual.DialectOpenAI()
		case "siliconflow", "sf":
			d = visual.DialectSiliconFlow()
		case "chat", "gemini", "4sapi", "chat-gemini":
			d = visual.DialectChatGemini()
		case "", "generic":
			d = visual.DialectGeneric()
		}
		if cfg.EditPath != "" {
			d.EditPath = cfg.EditPath
		}
		backend = &visual.GenericImageBackend{BaseURL: cfg.BaseURL, APIKey: cfg.APIKey, Model: cfg.Model, Header: hdr, Dialect: d}
	case "siliconflow", "siliconflow-image", "sf":
		hdr := http.Header{}
		for k, v := range cfg.Header {
			hdr.Set(k, v)
		}
		backend = &visual.SiliconFlowImageBackend{
			BaseURL: cfg.BaseURL,
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
			Header:  hdr,
		}
	case "comfyui":
		return "ERR:comfyui 需工作流(WorkflowBuilder)，App 侧暂不支持；请用直接画图模型(openai-image)"
	default:
		return "ERR:unknown backend: " + cfg.Backend
	}
	svc, err := visual.NewService(backend, visual.NewWatermarker(), 2)
	if err != nil {
		return "ERR:" + err.Error()
	}
	visMu.Lock()
	visSvc = svc
	visMu.Unlock()
	return "OK"
}

func visReady() (*visual.Service, bool) {
	visMu.Lock()
	defer visMu.Unlock()
	return visSvc, visSvc != nil
}

// VisualStatus 返回 {configured, backend}。
func VisualStatus() string {
	visMu.Lock()
	defer visMu.Unlock()
	configured := visSvc != nil
	be := ""
	if configured {
		be = visSvc.Backend()
	}
	b, _ := json.Marshal(map[string]any{"configured": configured, "backend": be})
	return string(b)
}

// VisualScenes 返回自拍场景池（key + 提示），供 UI 选择。
func VisualScenes() string {
	tpl := visual.DefaultSelfieTemplate()
	b, _ := json.Marshal(tpl.Scenes)
	return string(b)
}

// imageResult 回传 UI：已强制水印的图像（base64）。
type imageResult struct {
	ImageB64 string `json:"image_b64"`
	MIME     string `json:"mime"`
}

// waitAndFetch 等待任务终态并取第一张（已水印）图。
func waitAndFetch(ctx context.Context, svc *visual.Service, jobID string) (imageResult, error) {
	job, err := svc.Wait(ctx, jobID)
	if err != nil {
		return imageResult{}, err
	}
	if job.Status != visual.JobSucceeded || len(job.ItemIDs) == 0 {
		msg := job.Error
		if msg == "" {
			msg = string(job.Status)
		}
		return imageResult{}, &visErr{"job not succeeded: " + msg}
	}
	it, ok := svc.Gallery().Get(job.ItemIDs[0])
	if !ok || len(it.Data) == 0 {
		return imageResult{}, &visErr{"image not found in gallery"}
	}
	return imageResult{ImageB64: base64.StdEncoding.EncodeToString(it.Data), MIME: it.MIME}, nil
}

type visErr struct{ m string }

func (e *visErr) Error() string { return e.m }

// VisualGenerateImage 纯生图（无垫图）：prompt→图。返回 {image_b64,mime}/"ERR:..."。
func VisualGenerateImage(prompt string, width, height int) string {
	svc, ok := visReady()
	if !ok {
		return "ERR:visual not initialized"
	}
	if strings.TrimSpace(prompt) == "" {
		return "ERR:empty prompt"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	jobID, err := svc.SubmitRequest(ctx, visual.GenerateRequest{Prompt: prompt, Width: width, Height: height}, "manual")
	if err != nil {
		return "ERR:" + err.Error()
	}
	res, err := waitAndFetch(ctx, svc, jobID)
	if err != nil {
		return "ERR:" + err.Error()
	}
	b, _ := json.Marshal(res)
	return string(b)
}

// VisualSelfie 自拍：用角色人设图垫图 + 场景池，生成"她此刻的样子"（已水印）。
// characterID 必须已设人设图。返回 {image_b64,mime}/"ERR:..."。
func VisualSelfie(characterID, sceneKey string) string {
	svc, ok := visReady()
	if !ok {
		return "ERR:visual not initialized"
	}
	if !kernelReady() {
		return "ERR:kernel not initialized"
	}
	c, err := kbChars.Get(context.Background(), characterID)
	if err != nil {
		return "ERR:character not found: " + err.Error()
	}
	if len(c.PersonaImage) == 0 {
		return "ERR:该角色未设人设图，自拍需先在角色里上传人设图（人设图即垫图）"
	}
	pipe, err := visual.NewSelfiePipeline(svc, visual.DefaultSelfieTemplate(), time.Now().UnixNano())
	if err != nil {
		return "ERR:" + err.Error()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	jobID, err := pipe.Submit(ctx, c.PersonaImage, sceneKey)
	if err != nil {
		return "ERR:" + err.Error()
	}
	res, err := waitAndFetch(ctx, svc, jobID)
	if err != nil {
		return "ERR:" + err.Error()
	}
	b, _ := json.Marshal(res)
	return string(b)
}
