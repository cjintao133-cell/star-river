package visual

// 通用「OpenAI 兼容」图像后端 + 方言 profile + 响应归一化 + 尺寸吸附。
//
// 背景：不同厂商/中转站的图像 API 不是同一方言——img2img 端点（/images/edits multipart vs
// /images/generations + image 字段 vs chat 型出图）、尺寸参数名（size / image_size / width,height）、
// 是否支持 response_format、返回结构（data[].b64_json / data[].url / images[].url / output[].result /
// choices[].message）各不相同。本文件把差异收敛为可配置的 ImageDialect，一家只需一个 preset，
// 不再为每个厂商写一个后端类。既有 OpenAIImageBackend / SiliconFlowImageBackend 保留为已验证快路径。
//
// 复用同包辅助：APIError / truncate / maxImageRespBytes / defaultImageHTTPTimeout / toDataURL / fetchURLBytes / netx。

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/cjintao133-cell/star-river/kernel/netx"
)

// Img2ImgMode img2img 走法。
type Img2ImgMode string

const (
	Img2ImgEdits       Img2ImgMode = "edits"       // multipart POST {base}/images/edits（官方 OpenAI）
	Img2ImgGenerations Img2ImgMode = "generations" // JSON {base}/images/generations + image 字段（硅基/千问等）
	Img2ImgChat        Img2ImgMode = "chat"        // chat completions 带 image 输入（gpt-4o 类出图）
)

// ImageDialect 一家服务商的图像方言。
type ImageDialect struct {
	Name               string
	Img2ImgMode        Img2ImgMode
	EditPath           string   // 默认 /images/edits
	GenPath            string   // 默认 /images/generations
	ChatPath           string   // 默认 /chat/completions
	SizeFields         []string // 尺寸字段名集合：size / image_size / wh(width+height)
	SendResponseFormat bool
	ResponseFormat     string // 默认 b64_json
}

// DialectOpenAI 官方 OpenAI 兼容（edits multipart + size + response_format）。
func DialectOpenAI() ImageDialect {
	return ImageDialect{
		Name: "openai", Img2ImgMode: Img2ImgEdits,
		EditPath: "/images/edits", GenPath: "/images/generations", ChatPath: "/chat/completions",
		SizeFields: []string{"size"}, SendResponseFormat: true, ResponseFormat: "b64_json",
	}
}

// DialectSiliconFlow 硅基流动（generations + image 字段；image_size/size 双写；不回 response_format）。
func DialectSiliconFlow() ImageDialect {
	return ImageDialect{
		Name: "siliconflow", Img2ImgMode: Img2ImgGenerations,
		EditPath: "/images/edits", GenPath: "/images/generations", ChatPath: "/chat/completions",
		SizeFields: []string{"image_size", "size"}, SendResponseFormat: false,
	}
}

// DialectGeneric 未知中转站的保守默认（generations + image 字段 + size）。
func DialectGeneric() ImageDialect {
	return ImageDialect{
		Name: "generic", Img2ImgMode: Img2ImgGenerations,
		EditPath: "/images/edits", GenPath: "/images/generations", ChatPath: "/chat/completions",
		SizeFields: []string{"size"}, SendResponseFormat: true, ResponseFormat: "b64_json",
	}
}

// GenericImageBackend 一个后端吃下全部 OpenAI 兼容方言（按 Dialect 分派）。
type GenericImageBackend struct {
	BaseURL string
	APIKey  string
	Model   string
	HTTP    *http.Client
	Header  http.Header
	Dialect ImageDialect
}

// Name 实现 ImageBackend。
func (b *GenericImageBackend) Name() string { return "generic-image:" + b.dialect().Name }

func (b *GenericImageBackend) dialect() ImageDialect {
	d := b.Dialect
	if d.GenPath == "" {
		d.GenPath = "/images/generations"
	}
	if d.EditPath == "" {
		d.EditPath = "/images/edits"
	}
	if d.ChatPath == "" {
		d.ChatPath = "/chat/completions"
	}
	if len(d.SizeFields) == 0 {
		d.SizeFields = []string{"size"}
	}
	if d.ResponseFormat == "" {
		d.ResponseFormat = "b64_json"
	}
	if d.Img2ImgMode == "" {
		d.Img2ImgMode = Img2ImgGenerations
	}
	return d
}

func (b *GenericImageBackend) client() *http.Client {
	if b.HTTP != nil {
		return b.HTTP
	}
	return netx.Client(defaultImageHTTPTimeout) // 禁跟随重定向（凭据 origin 绑定）
}

// Generate 实现 ImageBackend。
func (b *GenericImageBackend) Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return GenerateResult{}, &APIError{Kind: "protocol", Message: "generic-image: empty prompt"}
	}
	model := req.Model
	if model == "" {
		model = b.Model
	}
	d := b.dialect()
	// chat 型（Gemini 图像模型等）：文生图与图生图统一走 /chat/completions，图片内联返回。
	if d.Img2ImgMode == Img2ImgChat {
		return b.genChat(ctx, d, model, req)
	}
	if len(req.ImageRef) > 0 {
		if d.Img2ImgMode == Img2ImgEdits {
			return b.genEdits(ctx, d, model, req)
		}
		return b.genGenerations(ctx, d, model, req, true)
	}
	return b.genGenerations(ctx, d, model, req, false)
}

// applySize 按方言写尺寸字段。
func applySize(body map[string]any, fields []string, w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	for _, f := range fields {
		switch f {
		case "size", "image_size":
			body[f] = fmt.Sprintf("%dx%d", w, h)
		case "wh":
			body["width"] = w
			body["height"] = h
		}
	}
}

// genGenerations 走 /images/generations（JSON）。withImage=true 时把垫图作 image 字段（data URL）。
func (b *GenericImageBackend) genGenerations(ctx context.Context, d ImageDialect, model string, req GenerateRequest, withImage bool) (GenerateResult, error) {
	body := map[string]any{"model": model, "prompt": req.Prompt, "n": 1}
	size, snapped := snapImageSize(model, req.Width, req.Height)
	_ = snapped
	applySize(body, d.SizeFields, req.Width, req.Height)
	if size != "" && (len(d.SizeFields) > 0) {
		// 若发生吸附，用吸附后的字符串覆盖所有 "size"/"image_size" 字段。
		if size != fmt.Sprintf("%dx%d", req.Width, req.Height) {
			for _, f := range d.SizeFields {
				if f == "size" || f == "image_size" {
					body[f] = size
				}
			}
		}
	}
	if d.SendResponseFormat {
		body["response_format"] = d.ResponseFormat
	}
	if req.Seed != 0 {
		body["seed"] = req.Seed
	}
	if withImage {
		body["image"] = toDataURL(req.ImageRef)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return GenerateResult{}, &APIError{Kind: "protocol", Message: err.Error()}
	}
	respBytes, err := b.doJSON(ctx, strings.TrimRight(b.BaseURL, "/")+d.GenPath, raw)
	if err != nil {
		return GenerateResult{}, err
	}
	return b.finish(ctx, model, respBytes)
}

// genEdits 走 /images/edits（multipart 文件上传）。
func (b *GenericImageBackend) genEdits(ctx context.Context, d ImageDialect, model string, req GenerateRequest) (GenerateResult, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fname := "persona.png"
	if strings.Contains(http.DetectContentType(req.ImageRef), "jpeg") {
		fname = "persona.jpg"
	}
	fw, err := mw.CreateFormFile("image", fname)
	if err != nil {
		return GenerateResult{}, &APIError{Kind: "protocol", Message: "multipart: " + err.Error()}
	}
	if _, err := fw.Write(req.ImageRef); err != nil {
		return GenerateResult{}, &APIError{Kind: "protocol", Message: "multipart write: " + err.Error()}
	}
	_ = mw.WriteField("prompt", req.Prompt)
	if model != "" {
		_ = mw.WriteField("model", model)
	}
	_ = mw.WriteField("n", "1")
	if d.SendResponseFormat {
		_ = mw.WriteField("response_format", d.ResponseFormat)
	}
	if size, _ := snapImageSize(model, req.Width, req.Height); size != "" {
		_ = mw.WriteField("size", size)
	}
	if err := mw.Close(); err != nil {
		return GenerateResult{}, &APIError{Kind: "protocol", Message: "multipart close: " + err.Error()}
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(b.BaseURL, "/")+d.EditPath, &buf)
	if err != nil {
		return GenerateResult{}, &APIError{Kind: "protocol", Message: err.Error()}
	}
	hreq.Header.Set("Content-Type", mw.FormDataContentType())
	b.setHeaders(hreq)
	resp, err := b.client().Do(hreq)
	if err != nil {
		return GenerateResult{}, &APIError{Kind: "network", Message: err.Error()}
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxImageRespBytes))
	if err != nil {
		return GenerateResult{}, &APIError{Kind: "network", Message: err.Error()}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return GenerateResult{}, &APIError{Kind: "http", Message: fmt.Sprintf("status %d: %s", resp.StatusCode, truncate(string(bodyBytes), 200))}
	}
	return b.finish(ctx, model, bodyBytes)
}

// genChat 走 /chat/completions，把提示词+垫图作为 user 消息（image_url data URL）发出。
func (b *GenericImageBackend) genChat(ctx context.Context, d ImageDialect, model string, req GenerateRequest) (GenerateResult, error) {
	var content any
	if len(req.ImageRef) > 0 {
		parts := []map[string]any{
			{"type": "text", "text": req.Prompt},
			{"type": "image_url", "image_url": map[string]any{"url": toDataURL(req.ImageRef)}},
		}
		content = parts
	} else {
		content = req.Prompt
	}
	body := map[string]any{
		"model":    model,
		"messages": []map[string]any{{"role": "user", "content": content}},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return GenerateResult{}, &APIError{Kind: "protocol", Message: err.Error()}
	}
	respBytes, err := b.doJSON(ctx, strings.TrimRight(b.BaseURL, "/")+d.ChatPath, raw)
	if err != nil {
		return GenerateResult{}, err
	}
	return b.finish(ctx, model, respBytes)
}

func (b *GenericImageBackend) setHeaders(hreq *http.Request) {
	if b.APIKey != "" {
		hreq.Header.Set("Authorization", "Bearer "+b.APIKey)
	}
	for k, vs := range b.Header {
		for _, v := range vs {
			hreq.Header.Add(k, v)
		}
	}
}

func (b *GenericImageBackend) doJSON(ctx context.Context, url string, raw []byte) ([]byte, error) {
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, &APIError{Kind: "protocol", Message: err.Error()}
	}
	hreq.Header.Set("Content-Type", "application/json")
	b.setHeaders(hreq)
	resp, err := b.client().Do(hreq)
	if err != nil {
		return nil, &APIError{Kind: "network", Message: err.Error()}
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxImageRespBytes))
	if err != nil {
		return nil, &APIError{Kind: "network", Message: err.Error()}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Kind: "http", Message: fmt.Sprintf("status %d: %s", resp.StatusCode, truncate(string(bodyBytes), 200))}
	}
	return bodyBytes, nil
}

func (b *GenericImageBackend) finish(ctx context.Context, model string, body []byte) (GenerateResult, error) {
	imgs, err := normalizeImageResponse(ctx, b.client(), body)
	if err != nil {
		return GenerateResult{}, err
	}
	return GenerateResult{Model: model, Images: imgs, Info: map[string]any{}}, nil
}

// normalizeImageResponse 把各种返回结构归一化为图像字节切片。支持：
//
//	data[].b64_json / data[].url / images[].url / output[].result(b64, responses API) /
//	choices[].message.images[] / choices[].message.content[](type=image_url) / data 为 url 字符串。
func normalizeImageResponse(ctx context.Context, dl *http.Client, body []byte) ([][]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, &APIError{Kind: "protocol", Message: "bad json: " + err.Error()}
	}
	// 错误对象优先。
	if e, ok := m["error"]; ok && len(e) > 0 && string(e) != "null" {
		var er struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(e, &er) == nil && er.Message != "" {
			return nil, &APIError{Kind: "rpc", Message: er.Message}
		}
	}
	var images [][]byte

	// 单条：解析 b64_json / b64 / result(b64) / url。
	appendItem := func(it json.RawMessage) error {
		var o struct {
			B64JSON string `json:"b64_json"`
			B64     string `json:"b64"`
			Result  string `json:"result"`
			URL     string `json:"url"`
		}
		if json.Unmarshal(it, &o) != nil {
			return nil // 非图像 item（如 responses 的 message），忽略
		}
		for _, b64 := range []string{o.B64JSON, o.B64, o.Result} {
			if b64 != "" {
				if d, err := base64.StdEncoding.DecodeString(b64); err == nil {
					images = append(images, d)
				}
				return nil
			}
		}
		if o.URL != "" {
			if d, err := fetchURLBytes(ctx, dl, o.URL); err == nil {
				images = append(images, d)
			} else {
				return err
			}
		}
		return nil
	}

	// data / images / output 数组。
	for _, key := range []string{"data", "images", "output"} {
		if raw, ok := m[key]; ok {
			var arr []json.RawMessage
			if json.Unmarshal(raw, &arr) == nil {
				for _, it := range arr {
					if err := appendItem(it); err != nil {
						return nil, err
					}
				}
			} else {
				// data 可能是 url 字符串
				var s string
				if json.Unmarshal(raw, &s) == nil && strings.HasPrefix(s, "http") {
					if d, err := fetchURLBytes(ctx, dl, s); err == nil {
						images = append(images, d)
					}
				}
			}
		}
	}

	// choices[].message：{content, images[]}
	if raw, ok := m["choices"]; ok {
		var chs []struct {
			Message struct {
				Content json.RawMessage   `json:"content"`
				Images  []json.RawMessage `json:"images"`
			} `json:"message"`
		}
		if json.Unmarshal(raw, &chs) == nil {
			for _, ch := range chs {
				for _, im := range ch.Message.Images {
					if err := appendItem(im); err != nil {
						return nil, err
					}
				}
				// content 为字符串时：可能是 markdown ![](data:image/...;base64,xxx)
				var cstr string
				if json.Unmarshal(ch.Message.Content, &cstr) == nil && cstr != "" {
					images = append(images, extractDataURLs(cstr)...)
				}
				// content 为数组时找 image_url
				var parts []struct {
					Type     string `json:"type"`
					ImageURL struct {
						URL string `json:"url"`
					} `json:"image_url"`
				}
				if json.Unmarshal(ch.Message.Content, &parts) == nil {
					for _, p := range parts {
						if p.Type == "image_url" && p.ImageURL.URL != "" {
							if d, err := fetchURLBytes(ctx, dl, p.ImageURL.URL); err == nil {
								images = append(images, d)
							}
						}
					}
				}
			}
		}
	}

	if len(images) == 0 {
		return nil, &APIError{Kind: "protocol", Message: "no image in response"}
	}
	return images, nil
}

// snapImageSize 把 (w,h) 吸附到该模型支持的尺寸档，返回 "WxH"。未知模型原样返回。
// snapped=false 表示未改动。已知固定档：DALL·E 3 / gpt-image-1 / DALL·E 2。
func snapImageSize(model string, w, h int) (string, bool) {
	if w <= 0 || h <= 0 {
		return "", false
	}
	m := strings.ToLower(model)
	type opt struct{ w, h int }
	var opts []opt
	switch {
	case strings.Contains(m, "dall-e-3"), strings.Contains(m, "dalle-3"):
		opts = []opt{{1024, 1024}, {1792, 1024}, {1024, 1792}}
	case strings.Contains(m, "gpt-image"):
		opts = []opt{{1024, 1024}, {1536, 1024}, {1024, 1536}}
	case strings.Contains(m, "dall-e-2"), strings.Contains(m, "dalle-2"):
		opts = []opt{{256, 256}, {512, 512}, {1024, 1024}}
	default:
		return fmt.Sprintf("%dx%d", w, h), false // 未知模型不吸附
	}
	best := opts[0]
	bestDiff := 1 << 60
	for _, o := range opts {
		d := abs(o.w-w) + abs(o.h-h)
		if d < bestDiff {
			bestDiff, best = d, o
		}
	}
	return fmt.Sprintf("%dx%d", best.w, best.h), (best.w != w || best.h != h)
}

// extractDataURLs 从任意字符串（含 markdown ![](data:image/...;base64,xxx)）中抽取并解码内联图像。
func extractDataURLs(s string) [][]byte {
	var out [][]byte
	for {
		i := strings.Index(s, "data:image/")
		if i < 0 {
			break
		}
		rest := s[i:]
		comma := strings.Index(rest, ";base64,")
		if comma < 0 {
			s = rest[len("data:image/"):]
			continue
		}
		start := comma + len(";base64,")
		j := start
		for j < len(rest) && isB64Byte(rest[j]) {
			j++
		}
		if j > start {
			if d, err := base64.StdEncoding.DecodeString(rest[start:j]); err == nil && len(d) > 0 {
				out = append(out, d)
			}
		}
		s = rest[j:]
	}
	return out
}

func isB64Byte(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '/' || c == '='
}

// DialectChatGemini 对话式出图方言（Gemini 图像模型 / Nano Banana 类）：统一走 chat/completions，
// 图片以 markdown data-url 内联在回复正文。4sapi / 星链API 实测即此方言。
func DialectChatGemini() ImageDialect {
	return ImageDialect{
		Name: "chat-gemini", Img2ImgMode: Img2ImgChat,
		EditPath: "/images/edits", GenPath: "/images/generations", ChatPath: "/chat/completions",
		SizeFields: nil, SendResponseFormat: false,
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
