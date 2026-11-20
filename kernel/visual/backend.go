package visual

import "context"

// GenerateRequest 一次图像生成请求（适配器无关的中性结构）。
type GenerateRequest struct {
	Prompt         string
	NegativePrompt string
	Model          string
	Width          int
	Height         int
	Seed           int64
	ImageRef       []byte  // 可选垫图（img2img / 参考图），已解码字节
	Strength       float64 // 垫图强度 0..1（ImageRef 存在时）
	Extra          map[string]any
}

// GenerateResult 生成结果（原始图像字节，未加水印；水印由管线强制叠加）。
type GenerateResult struct {
	Images [][]byte
	Model  string
	Info   map[string]any
}

// ImageBackend 图像生成适配器接口（ComfyUI / OpenAI 兼容 等）。
type ImageBackend interface {
	// Name 适配器名（日志/画廊元数据用）。
	Name() string
	// Generate 执行一次生成，返回原始图像字节（1..n 张）。
	Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error)
}
