package visual

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ComfyUIBackend ComfyUI 适配器：POST /prompt 提交工作流 → 轮询 /history/{id} → GET /view 取图。
//
// ComfyUI 用节点图工作流，语义（提示词/尺寸/种子）如何注入因工作流而异；
// 因此由调用方提供 WorkflowBuilder 把这些从 GenerateRequest 映射进自己的工作流模板，
// 本适配器只负责传输与轮询（与图结构解耦，便于替换工作流）。
type ComfyUIBackend struct {
	BaseURL         string
	ClientID        string
	HTTP            *http.Client
	Header          http.Header
	WorkflowBuilder func(req GenerateRequest) (json.RawMessage, error)
	PollInterval    time.Duration // 轮询间隔（<=0 用 500ms）
	PollTimeout     time.Duration // 总轮询超时（<=0 用 120s）
}

// Name 实现 ImageBackend。
func (b *ComfyUIBackend) Name() string { return "comfyui" }

// Generate 实现 ImageBackend。
func (b *ComfyUIBackend) Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
	if b.WorkflowBuilder == nil {
		return GenerateResult{}, &APIError{Kind: "protocol", Message: "comfyui: WorkflowBuilder not set"}
	}
	workflow, err := b.WorkflowBuilder(req)
	if err != nil {
		return GenerateResult{}, &APIError{Kind: "protocol", Message: "comfyui: build workflow: " + err.Error()}
	}
	hc := b.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: defaultImageHTTPTimeout}
	}
	base := strings.TrimRight(b.BaseURL, "/")

	// 1) 提交 prompt
	submitBody, _ := json.Marshal(map[string]any{"prompt": json.RawMessage(workflow), "client_id": b.ClientID})
	resp, err := b.do(ctx, hc, http.MethodPost, base+"/prompt", bytes.NewReader(submitBody))
	if err != nil {
		return GenerateResult{}, err
	}
	var submit struct {
		PromptID   string `json:"prompt_id"`
		Error      string `json:"error"`
		NodeErrors any    `json:"node_errors"`
	}
	if err := json.Unmarshal(resp, &submit); err != nil {
		return GenerateResult{}, &APIError{Kind: "protocol", Message: "comfyui: submit bad json: " + err.Error()}
	}
	if submit.PromptID == "" {
		return GenerateResult{}, &APIError{Kind: "rpc", Message: "comfyui: no prompt_id: " + submit.Error}
	}

	// 2) 轮询 history
	images, err := b.pollHistory(ctx, hc, base, submit.PromptID)
	if err != nil {
		return GenerateResult{}, err
	}

	// 3) 取图字节
	res := GenerateResult{Model: b.Name(), Info: map[string]any{"prompt_id": submit.PromptID}}
	for _, ref := range images {
		img, err := b.fetchView(ctx, hc, base, ref)
		if err != nil {
			return GenerateResult{}, err
		}
		res.Images = append(res.Images, img)
	}
	if len(res.Images) == 0 {
		return GenerateResult{}, &APIError{Kind: "protocol", Message: "comfyui: no output images"}
	}
	return res, nil
}

// imageRef ComfyUI /view 所需的图像引用。
type comfyImageRef struct {
	Filename  string `json:"filename"`
	Subfolder string `json:"subfolder"`
	Type      string `json:"type"`
}

// pollHistory 轮询 /history/{id} 直到 outputs 出现或超时。
func (b *ComfyUIBackend) pollHistory(ctx context.Context, hc *http.Client, base, promptID string) ([]comfyImageRef, error) {
	interval := b.PollInterval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	timeout := b.PollTimeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return nil, &APIError{Kind: "timeout", Message: "comfyui: history poll timeout"}
		}
		body, err := b.do(ctx, hc, http.MethodGet, base+"/history/"+url.PathEscape(promptID), nil)
		if err != nil {
			return nil, err
		}
		var hist map[string]struct {
			Outputs map[string]struct {
				Images []comfyImageRef `json:"images"`
			} `json:"outputs"`
			Status struct {
				Completed bool `json:"completed"`
			} `json:"status"`
		}
		if err := json.Unmarshal(body, &hist); err != nil {
			return nil, &APIError{Kind: "protocol", Message: "comfyui: history bad json: " + err.Error()}
		}
		if entry, ok := hist[promptID]; ok {
			var refs []comfyImageRef
			for _, node := range entry.Outputs {
				refs = append(refs, node.Images...)
			}
			if len(refs) > 0 {
				return refs, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, &APIError{Kind: "canceled", Message: "comfyui: poll canceled: " + ctx.Err().Error()}
		case <-time.After(interval):
		}
	}
}

// fetchView 通过 /view 拉取图像字节。
func (b *ComfyUIBackend) fetchView(ctx context.Context, hc *http.Client, base string, ref comfyImageRef) ([]byte, error) {
	q := url.Values{}
	q.Set("filename", ref.Filename)
	q.Set("subfolder", ref.Subfolder)
	q.Set("type", ref.Type)
	return b.do(ctx, hc, http.MethodGet, base+"/view?"+q.Encode(), nil)
}

// do 发请求并返回 2xx 响应体（错误归类）。
func (b *ComfyUIBackend) do(ctx context.Context, hc *http.Client, method, url string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, &APIError{Kind: "protocol", Message: err.Error()}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, vs := range b.Header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, &APIError{Kind: "network", Message: err.Error()}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageRespBytes))
	if err != nil {
		return nil, &APIError{Kind: "network", Message: err.Error()}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Kind: "http", Message: fmt.Sprintf("comfyui: status %d: %s", resp.StatusCode, truncate(string(data), 200))}
	}
	return data, nil
}
