package visual

import (
	"errors"
	"fmt"
	"strings"
)

// ErrRecipe 配方非法。
var ErrRecipe = errors.New("visual: invalid recipe")

// Slot 输入槽（配方声明用户/模型需填充的语义槽位）。
type Slot struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
	Default  string `json:"default,omitempty"`
}

// ParamType 可调参数类型。
type ParamType string

const (
	ParamInt    ParamType = "int"
	ParamFloat  ParamType = "float"
	ParamEnum   ParamType = "enum"
	ParamString ParamType = "string"
)

// Param 可调参数（尺寸/种子/风格等）。
type Param struct {
	Key        string    `json:"key"`
	Label      string    `json:"label"`
	Type       ParamType `json:"type"`
	Min        float64   `json:"min,omitempty"`
	Max        float64   `json:"max,omitempty"`
	Default    float64   `json:"default,omitempty"`
	DefaultStr string    `json:"default_str,omitempty"`
	Options    []string  `json:"options,omitempty"` // enum 用
}

// ImageRefPreset 垫图强度预设（img2img）。
type ImageRefPreset struct {
	Key      string  `json:"key"`
	Label    string  `json:"label"`
	Strength float64 `json:"strength"` // 0..1
}

// Recipe 视觉配方（数据驱动，JSON 可序列化，可整表替换）。
type Recipe struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	PromptTemplate  string           `json:"prompt_template"` // 用 {{key}} 引用 slot
	NegativePrompt  string           `json:"negative_prompt,omitempty"`
	Slots           []Slot           `json:"slots"`
	Params          []Param          `json:"params"`
	ImageRefPresets []ImageRefPreset `json:"image_ref_presets,omitempty"`
	DefaultModel    string           `json:"default_model,omitempty"`
	DefaultWidth    int              `json:"default_width,omitempty"`
	DefaultHeight   int              `json:"default_height,omitempty"`
}

// Validate 校验配方结构。
func (r Recipe) Validate() error {
	if r.ID == "" || r.Name == "" || r.PromptTemplate == "" {
		return fmt.Errorf("%w: id/name/prompt_template required", ErrRecipe)
	}
	seen := map[string]bool{}
	for _, s := range r.Slots {
		if s.Key == "" {
			return fmt.Errorf("%w: empty slot key", ErrRecipe)
		}
		if seen["s:"+s.Key] {
			return fmt.Errorf("%w: duplicate slot %q", ErrRecipe, s.Key)
		}
		seen["s:"+s.Key] = true
	}
	for _, p := range r.Params {
		if p.Key == "" {
			return fmt.Errorf("%w: empty param key", ErrRecipe)
		}
		if seen["p:"+p.Key] {
			return fmt.Errorf("%w: duplicate param %q", ErrRecipe, p.Key)
		}
		seen["p:"+p.Key] = true
		switch p.Type {
		case ParamInt, ParamFloat:
			if p.Max != 0 && p.Min > p.Max {
				return fmt.Errorf("%w: param %q min>max", ErrRecipe, p.Key)
			}
		case ParamEnum:
			if len(p.Options) == 0 {
				return fmt.Errorf("%w: enum param %q needs options", ErrRecipe, p.Key)
			}
		case ParamString:
		default:
			return fmt.Errorf("%w: param %q unknown type %q", ErrRecipe, p.Key, p.Type)
		}
	}
	for _, ps := range r.ImageRefPresets {
		if ps.Key == "" || ps.Strength < 0 || ps.Strength > 1 {
			return fmt.Errorf("%w: bad image_ref_preset %q (strength 0..1)", ErrRecipe, ps.Key)
		}
	}
	return nil
}

// BuildRequest 用输入填充配方，产出 GenerateRequest。
// inputs：slot 值 + 可选 param 覆盖（int/float 以数字字符串、enum/string 以文本）。
// 必填 slot 缺失（且无默认）报错。
func (r Recipe) BuildRequest(inputs map[string]string) (GenerateRequest, error) {
	if err := r.Validate(); err != nil {
		return GenerateRequest{}, err
	}
	// 组装 slot 值（inputs 覆盖默认）。
	vals := make(map[string]string, len(r.Slots))
	for _, s := range r.Slots {
		v := s.Default
		if iv, ok := inputs[s.Key]; ok {
			v = iv
		}
		if s.Required && strings.TrimSpace(v) == "" {
			return GenerateRequest{}, fmt.Errorf("%w: required slot %q empty", ErrRecipe, s.Key)
		}
		vals[s.Key] = v
	}
	// 渲染 prompt 模板。
	prompt := renderTemplate(r.PromptTemplate, vals)

	req := GenerateRequest{
		Prompt:         prompt,
		NegativePrompt: r.NegativePrompt,
		Model:          r.DefaultModel,
		Width:          r.DefaultWidth,
		Height:         r.DefaultHeight,
		Extra:          map[string]any{},
	}
	// 应用参数（size/seed/model/strength 等约定 key）。
	for _, p := range r.Params {
		iv, ok := inputs[p.Key]
		if !ok {
			continue
		}
		iv = strings.TrimSpace(iv)
		if iv == "" {
			continue
		}
		switch p.Key {
		case "width":
			if n, err := parseInt(iv); err == nil {
				req.Width = n
			}
		case "height":
			if n, err := parseInt(iv); err == nil {
				req.Height = n
			}
		case "seed":
			if n, err := parseInt64(iv); err == nil {
				req.Seed = n
			}
		case "model":
			req.Model = iv
		case "strength":
			if f, err := parseFloat(iv); err == nil {
				req.Strength = f
			}
		default:
			req.Extra[p.Key] = iv
		}
	}
	return req, nil
}

// DefaultImageRefStrength 按预设 key 取垫图强度；未知返回 0 与 false。
func (r Recipe) DefaultImageRefStrength(key string) (float64, bool) {
	for _, ps := range r.ImageRefPresets {
		if ps.Key == key {
			return ps.Strength, true
		}
	}
	return 0, false
}

// renderTemplate 把 {{key}} 替换为值；未提供的 token 保留原样。
func renderTemplate(tpl string, vals map[string]string) string {
	out := tpl
	for k, v := range vals {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}
