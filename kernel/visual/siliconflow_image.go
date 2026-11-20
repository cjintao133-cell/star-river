package visual

// SiliconFlowImageBackend 适配硅基流动（SiliconFlow）图像 API。
// 与 OpenAI 的差异（实测）：
//   - 无 /images/edits（404）；图生图复用 /images/generations，用 JSON 的 "image" 字段传
//     data URL（base64），不走 multipart 文件上传。
//   - 返回体可能是 OpenAI 风格 {data:[{b64_json|url}]}，也可能是 {images:[{url}]}，两种都解析。
// 文本生图/图生图共用本后端的 Generate（req.ImageRef 非空即图生图）。

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"star-river/kernel/netx"
)

// SiliconFlowImageBackend 硅基流动图像后端。
type SiliconFlowImageBackend struct {
	BaseURL string       // 形如 https://api.siliconflow.cn/v1
	APIKey  string       // 仅经 Authorization 头
	Model   string       // 默认模型（可被 req.Model 覆盖），如 Kwai-Kolors/Kolors / Qwen/Qwen-Image-Edit
	HTTP    *http.Client // 可空（默认禁重定向 netx.Client）
	Header  http.Header  // 额外头（relay 用）
}

// Name 实现 ImageBackend。
func (b *SiliconFlowImageBackend) Name() string { return "siliconflow-image" }

type sfImageResp struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
		URL     string `json:"url"`
	} `json:"data"`
	Images []struct {
		URL string `json:"url"`
	} `json:"images"`
	Error *struct {
		Message string `json:"message"`
		Code    any    `json:"code"`
	} `json:"error"`
	Message string `json:"message"`
}

// Generate 实现 ImageBackend：req.ImageRef 非空走图生图（image 字段），否则文生图。
func (b *SiliconFlowImageBackend) Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return GenerateResult{}, &APIError{Kind: "protocol", Message: "siliconflow-image: empty prompt"}
	}
	model := req.Model
	if model == "" {
		model = b.Model
	}
	body := map[string]any{
		"model":      model,
		"prompt":     req.Prompt,
		"batch_size": 1,
	}
	if req.Width > 0 && req.Height > 0 {
		size := fmt.Sprintf("%dx%d", req.Width, req.Height)
		body["image_size"] = size // 硅基字段
		body["size"] = size       // OpenAI 兼容别名（多数服务端忽略未知键）
	}
	if req.Seed != 0 {
		body["seed"] = req.Seed
	}
	if len(req.ImageRef) > 0 {
		body["image"] = toDataURL(req.ImageRef)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return GenerateResult{}, &APIError{Kind: "protocol", Message: err.Error()}
	}

	hc := b.HTTP
	if hc == nil {
		hc = netx.Client(defaultImageHTTPTimeout) // 禁跟随重定向（凭据 origin 绑定）
	}
	url := strings.TrimRight(b.BaseURL, "/") + "/images/generations"
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return GenerateResult{}, &APIError{Kind: "protocol", Message: err.Error()}
	}
	hreq.Header.Set("Content-Type", "application/json")
	if b.APIKey != "" {
		hreq.Header.Set("Authorization", "Bearer "+b.APIKey)
	}
	for k, vs := range b.Header {
		for _, v := range vs {
			hreq.Header.Add(k, v)
		}
	}
	resp, err := hc.Do(hreq)
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

	var out sfImageResp
	if err := json.Unmarshal(bodyBytes, &out); err != nil {
		return GenerateResult{}, &APIError{Kind: "protocol", Message: "bad json: " + err.Error()}
	}
	if out.Error != nil {
		return GenerateResult{}, &APIError{Kind: "rpc", Message: out.Error.Message}
	}
	res := GenerateResult{Model: model, Info: map[string]any{}}
	dl := &http.Client{Timeout: defaultImageHTTPTimeout} // url 下载无凭据，允许跳转
	for _, d := range out.Data {
		if d.B64JSON != "" {
			img, err := base64.StdEncoding.DecodeString(d.B64JSON)
			if err != nil {
				return GenerateResult{}, &APIError{Kind: "protocol", Message: "bad b64: " + err.Error()}
			}
			res.Images = append(res.Images, img)
		} else if d.URL != "" {
			img, err := fetchURLBytes(ctx, dl, d.URL)
			if err != nil {
				return GenerateResult{}, err
			}
			res.Images = append(res.Images, img)
		}
	}
	for _, im := range out.Images {
		if im.URL != "" {
			img, err := fetchURLBytes(ctx, dl, im.URL)
			if err != nil {
				return GenerateResult{}, err
			}
			res.Images = append(res.Images, img)
		}
	}
	if len(res.Images) == 0 {
		msg := out.Message
		if msg == "" {
			msg = "no image data"
		}
		return GenerateResult{}, &APIError{Kind: "protocol", Message: msg}
	}
	return res, nil
}

// toDataURL 把图像字节编码成 data URL（http.DetectContentType 判 MIME）。
func toDataURL(data []byte) string {
	mime := http.DetectContentType(data)
	if mime == "" || !strings.HasPrefix(mime, "image/") {
		mime = "image/png"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// fetchURLBytes 下载 url 字节（无凭据 GET，允许跳转）。
func fetchURLBytes(ctx context.Context, hc *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, &APIError{Kind: "protocol", Message: err.Error()}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &APIError{Kind: "network", Message: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Kind: "http", Message: fmt.Sprintf("image url status %d", resp.StatusCode)}
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxImageRespBytes))
}
