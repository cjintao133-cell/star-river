package mobilebind

// 模型能力卡桥（T-新增）：把模型能力「详标」暴露给 UI。薄门面，JSON 进出。

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/cjintao133-cell/star-river/kernel/adapter/discovery"
	"github.com/cjintao133-cell/star-river/kernel/adapter/openai"
	"github.com/cjintao133-cell/star-river/kernel/adapter/probe"
)

// modelMetaDTO UI 传来的模型元数据（来自 /models 富字段）。
type modelMetaDTO struct {
	ID               string   `json:"id"`
	Provider         string   `json:"provider,omitempty"`
	ContextLength    int      `json:"context_length,omitempty"`
	InputModalities  []string `json:"input_modalities,omitempty"`
	OutputModalities []string `json:"output_modalities,omitempty"`
}

// ModelCards 输入模型元数据 JSON 数组，返回能力卡 JSON 数组（分类+能力位+来源+上下文/模态）。
// 失败返回 "ERR:<信息>"。
func ModelCards(modelsJSON string) string {
	var dtos []modelMetaDTO
	if err := json.Unmarshal([]byte(modelsJSON), &dtos); err != nil {
		return "ERR:bad models json: " + err.Error()
	}
	metas := make([]discovery.ModelMeta, 0, len(dtos))
	for _, d := range dtos {
		metas = append(metas, discovery.ModelMeta{
			ID:               d.ID,
			Provider:         d.Provider,
			ContextLength:    d.ContextLength,
			InputModalities:  d.InputModalities,
			OutputModalities: d.OutputModalities,
		})
	}
	cards := discovery.BuildCards(metas, nil)
	b, err := json.Marshal(cards)
	if err != nil {
		return "ERR:" + err.Error()
	}
	return string(b)
}

// ProbeModel 对某模型做一次实测探测（OpenAI 兼容协议），返回探测到的能力位 JSON；
// 失败返回 "ERR:<脱敏>"。有真实成本，UI 应仅在用户选中模型时调用。
func ProbeModel(baseURL, apiKey, model string) string {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(model) == "" {
		return "ERR:endpoint/model 未配置"
	}
	cl := openai.New(baseURL, apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	sig, err := cl.SendProbe(ctx, model)
	if err != nil {
		return "ERR:" + redactKey(err.Error(), apiKey)
	}
	caps := probe.Caps(model, sig)
	b, err := json.Marshal(caps)
	if err != nil {
		return "ERR:" + redactKey(err.Error(), apiKey)
	}
	return string(b)
}
