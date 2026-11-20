package visual

// 模型图像能力表（能力探测 + 优雅降级用）。
// 目的：在执行前判断某模型「能不能出图」「支不支持垫图(img2img)」，
// 对不支持的模型提前置灰/降级，而不是等运行时 404/500 炸给用户。
// 说明：这是静态启发式（按模型名），覆盖已知主流；未知模型乐观放行，
// 真正出错时仍走既有错误分类（优雅 OnError），不会崩。

import "strings"

// ModelCap 模型的图像能力。
type ModelCap struct {
	CanImage   bool `json:"canImage"`   // 是否为画图模型
	CanImg2Img bool `json:"canImg2Img"` // 是否支持垫图/图生图
}

// 明显非画图（文本/语音/向量）模型标记。
var nonImageMarkers = []string{
	"tts", "text-to-speech", "whisper", "transcri", "asr",
	"embed", "embedding", "rerank", "bge-", "ocr", "moderation", "clip",
}

// 已知「能出图但不支持垫图」的模型标记（官方按模型区分能力）。
var noImg2ImgMarkers = []string{
	"dall-e-3", "dalle-3",
}

// ModelCapability 按模型名给出图像能力（启发式）。
func ModelCapability(model string) ModelCap {
	m := strings.ToLower(strings.TrimSpace(model))
	for _, k := range nonImageMarkers {
		if strings.Contains(m, k) {
			return ModelCap{CanImage: false, CanImg2Img: false}
		}
	}
	if m == "" {
		return ModelCap{CanImage: true, CanImg2Img: true} // 未知，乐观
	}
	for _, k := range noImg2ImgMarkers {
		if strings.Contains(m, k) {
			return ModelCap{CanImage: true, CanImg2Img: false}
		}
	}
	return ModelCap{CanImage: true, CanImg2Img: true}
}
