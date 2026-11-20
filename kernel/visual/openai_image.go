package visual

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// defaultImageHTTPTimeout 默认 HTTP 超时。
const defaultImageHTTPTimeout = 120 * time.Second

// maxImageRespBytes 响应体读取上限（含 base64，放宽到 32MiB）。
const maxImageRespBytes = 32 << 20

// OpenAIImageBackend OpenAI 兼容图像 API 适配器（POST {BaseURL}/images/generations）。
type OpenAIImageBackend struct {
	BaseURL string       // 形如 https://host/v1
	APIKey  string       // 仅经 Authorization 头
	Model   string       // 默认模型（可被 req.Model 覆盖）
	HTTP    *http.Client // 可空

	// 额外请求头（多用于第三方 relay）
	Header http.Header
}

// Name 实现 ImageBackend。
func (b *OpenAIImageBackend) Name() string { return "openai-image" }

type openAIImageReq struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n"`
	Size           string `json:"size,omitempty"`
	ResponseFormat string `json:"response_format"`
	Seed           *int64 `json:"seed,omitempty"`
}

type openAIImageResp struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
		URL     string `json:"url"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Generate 实现 ImageBackend。垫图（img2img）OpenAI 兼容 /images/generations 不支持，
// 若 req.ImageRef 非空则明确报错（不静默忽略）。
func (b *OpenAIImageBackend) Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return GenerateResult{}, &APIError{Kind: "protocol", Message: "openai-image: empty prompt"}
	}
	if len(req.ImageRef) > 0 {
		return GenerateResult{}, &APIError{Kind: "protocol", Message: "openai-image: image reference (img2img) not supported"}
	}
	model := req.Model
	if model == "" {
		model = b.Model
	}
	body := openAIImageReq{
		Model:          model,
		Prompt:         req.Prompt,
		N:              1,
		ResponseFormat: "b64_json",
	}
	if req.Width > 0 && req.Height > 0 {
		body.Size = fmt.Sprintf("%dx%d", req.Width, req.Height)
	}
	if req.Seed != 0 {
		seed := req.Seed
		body.Seed = &seed
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return GenerateResult{}, &APIError{Kind: "protocol", Message: err.Error()}
	}
	hc := b.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: defaultImageHTTPTimeout}
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
	var out openAIImageResp
	if err := json.Unmarshal(bodyBytes, &out); err != nil {
		return GenerateResult{}, &APIError{Kind: "protocol", Message: "bad json: " + err.Error()}
	}
	if out.Error != nil {
		return GenerateResult{}, &APIError{Kind: "rpc", Message: out.Error.Message}
	}
	if len(out.Data) == 0 {
		return GenerateResult{}, &APIError{Kind: "protocol", Message: "no image data"}
	}
	res := GenerateResult{Model: model, Info: map[string]any{}}
	for _, d := range out.Data {
		if d.B64JSON != "" {
			img, err := base64.StdEncoding.DecodeString(d.B64JSON)
			if err != nil {
				return GenerateResult{}, &APIError{Kind: "protocol", Message: "bad b64: " + err.Error()}
			}
			res.Images = append(res.Images, img)
			continue
		}
		if d.URL != "" {
			img, err := b.fetchImage(ctx, hc, d.URL)
			if err != nil {
				return GenerateResult{}, err
			}
			res.Images = append(res.Images, img)
		}
	}
	if len(res.Images) == 0 {
		return GenerateResult{}, &APIError{Kind: "protocol", Message: "empty images"}
	}
	return res, nil
}

// fetchImage 下载 url 返回的图像字节。
func (b *OpenAIImageBackend) fetchImage(ctx context.Context, hc *http.Client, url string) ([]byte, error) {
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

// APIError visual 包统一错误（Kind 复用 tool 语义字符串）。
type APIError struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// Error 实现 error。
func (e *APIError) Error() string { return "visual: " + e.Kind + ": " + e.Message }

// truncate 截断字符串（脱敏展示用）。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Errors 便于外部用 errors.As。
var _ = errors.As
