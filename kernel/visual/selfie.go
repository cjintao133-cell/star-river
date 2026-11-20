package visual

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
)

// ErrNoCharacterImage 无人设图，自拍功能不可用（视觉置灰）。
var ErrNoCharacterImage = errors.New("visual: character image required to unlock selfie")

// SelfieDisabledError 自拍功能被禁用（附带解锁提示，供 UI 展示）。
type SelfieDisabledError struct {
	Hint string
}

// Error 实现 error。
func (e *SelfieDisabledError) Error() string { return "visual: selfie disabled: " + e.Hint }

// Unwrap 支持 errors.Is(err, ErrNoCharacterImage)。
func (e *SelfieDisabledError) Unwrap() error { return ErrNoCharacterImage }

// SelfieScene 自拍场景池条目。
type SelfieScene struct {
	Key    string `json:"key"`
	Prompt string `json:"prompt"`
}

// SelfieTemplate 自拍模板（数据驱动）：第一人称构图 + 高强度垫图 + 场景随机。
type SelfieTemplate struct {
	ID             string        `json:"id"`
	BasePrompt     string        `json:"base_prompt"` // 含 {{scene}} 占位
	NegativePrompt string        `json:"negative_prompt"`
	Scenes         []SelfieScene `json:"scenes"`
	Strength       float64       `json:"strength"` // 垫图强度（自拍需高强度，默认 0.85）
	Width          int           `json:"default_width"`
	Height         int           `json:"default_height"`
	Model          string        `json:"default_model"`
}

// Validate 校验模板。
func (t SelfieTemplate) Validate() error {
	if t.ID == "" || strings.TrimSpace(t.BasePrompt) == "" {
		return fmt.Errorf("%w: selfie template id/base_prompt required", ErrRecipe)
	}
	if len(t.Scenes) == 0 {
		return fmt.Errorf("%w: selfie template needs scenes", ErrRecipe)
	}
	seen := map[string]bool{}
	for _, s := range t.Scenes {
		if s.Key == "" || s.Prompt == "" {
			return fmt.Errorf("%w: selfie scene key/prompt required", ErrRecipe)
		}
		if seen[s.Key] {
			return fmt.Errorf("%w: duplicate scene %q", ErrRecipe, s.Key)
		}
		seen[s.Key] = true
	}
	if t.Strength < 0 || t.Strength > 1 {
		return fmt.Errorf("%w: strength 0..1", ErrRecipe)
	}
	return nil
}

// DefaultSelfieTemplate 默认自拍模板（第一人称构图 + 场景池）。
func DefaultSelfieTemplate() SelfieTemplate {
	return SelfieTemplate{
		ID:             "selfie_default",
		BasePrompt:     "第一人称视角的手机自拍，画面以本人入镜为主，{{scene}}，表情自然生动，自拍构图，柔和光线",
		NegativePrompt: "lowres, bad anatomy, extra fingers, multiple people, blurry, deformed face",
		Scenes: []SelfieScene{
			{Key: "bedroom", Prompt: "在自己卧室里，清晨"},
			{Key: "cafe", Prompt: "在咖啡馆靠窗位置"},
			{Key: "seaside", Prompt: "在海边，夕阳光"},
			{Key: "library", Prompt: "在安静的图书馆"},
			{Key: "rainy", Prompt: "雨天隔着玻璃窗"},
			{Key: "night_market", Prompt: "热闹的夜市街头"},
		},
		Strength: 0.85,
		Width:    768, Height: 1024,
	}
}

// SelfiePipeline 自拍管线：人设图垫图 + 模板 + 场景随机 → 视觉管线异步生成。
// 无人设图 → 视觉功能置灰（返回 *SelfieDisabledError 带解锁提示），不提交任务。
type SelfiePipeline struct {
	svc *Service
	tpl SelfieTemplate
	mu  sync.Mutex
	rng *rand.Rand
}

// NewSelfiePipeline 建自拍管线。seed 用于场景随机（便于测试确定性）。
func NewSelfiePipeline(svc *Service, tpl SelfieTemplate, seed int64) (*SelfiePipeline, error) {
	if svc == nil {
		return nil, ErrNoBackend
	}
	if err := tpl.Validate(); err != nil {
		return nil, err
	}
	return &SelfiePipeline{svc: svc, tpl: tpl, rng: rand.New(rand.NewSource(seed))}, nil
}

// Available 报告自拍功能是否可用（人设图是否存在）。返回 (enabled, hint)。
// 无图时 hint 为解锁提示（供 UI 展示"视觉功能已置灰"）。
func (p *SelfiePipeline) Available(hasCharacterImage bool) (bool, string) {
	if !hasCharacterImage {
		return false, "角色还没有人设图，自拍/视觉功能已置灰；先导入或生成一张人设图即可解锁。"
	}
	return true, ""
}

// pickScene 随机选一个场景（并发安全）。
func (p *SelfiePipeline) pickScene() SelfieScene {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tpl.Scenes[p.rng.Intn(len(p.tpl.Scenes))]
}

// sceneByKey 按 key 取场景。
func (p *SelfiePipeline) sceneByKey(key string) (SelfieScene, bool) {
	for _, s := range p.tpl.Scenes {
		if s.Key == key {
			return s, true
		}
	}
	return SelfieScene{}, false
}

// Submit 发起一次自拍：characterImage 为人设图（垫图，必需）；sceneKey 为空则场景随机。
// 无 characterImage → 返回 *SelfieDisabledError（视觉置灰）。返回 jobID。
func (p *SelfiePipeline) Submit(ctx context.Context, characterImage []byte, sceneKey string) (string, error) {
	if len(characterImage) == 0 {
		_, hint := p.Available(false)
		return "", &SelfieDisabledError{Hint: hint}
	}
	var scene SelfieScene
	if sceneKey == "" {
		scene = p.pickScene()
	} else {
		s, ok := p.sceneByKey(sceneKey)
		if !ok {
			return "", fmt.Errorf("%w: unknown scene %q", ErrRecipe, sceneKey)
		}
		scene = s
	}
	prompt := strings.ReplaceAll(p.tpl.BasePrompt, "{{scene}}", scene.Prompt)
	req := GenerateRequest{
		Prompt:         prompt,
		NegativePrompt: p.tpl.NegativePrompt,
		Model:          p.tpl.Model,
		Width:          p.tpl.Width,
		Height:         p.tpl.Height,
		ImageRef:       characterImage, // 人设图垫图
		Strength:       p.tpl.Strength,
		Extra:          map[string]any{"selfie_scene": scene.Key},
	}
	return p.svc.SubmitRequest(ctx, req, p.tpl.ID)
}

// SubmitTextOnly 纯文生图自拍（降级用）：模型不支持垫图(img2img)时，用场景+外貌文字描述生成，
// 不传 ImageRef（无角色一致性），其余同上。appearance 为空则仅用场景。degraded 标记进 Extra。
func (p *SelfiePipeline) SubmitTextOnly(ctx context.Context, sceneKey, appearance string) (string, error) {
	var scene SelfieScene
	if sceneKey == "" {
		scene = p.pickScene()
	} else {
		s, ok := p.sceneByKey(sceneKey)
		if !ok {
			return "", fmt.Errorf("%w: unknown scene %q", ErrRecipe, sceneKey)
		}
		scene = s
	}
	prompt := strings.ReplaceAll(p.tpl.BasePrompt, "{{scene}}", scene.Prompt)
	if a := strings.TrimSpace(appearance); a != "" {
		prompt += "，人物外貌参考：" + a
	}
	req := GenerateRequest{
		Prompt:         prompt,
		NegativePrompt: p.tpl.NegativePrompt,
		Model:          p.tpl.Model,
		Width:          p.tpl.Width,
		Height:         p.tpl.Height, // 不设 ImageRef → 文生图
		Extra:          map[string]any{"selfie_scene": scene.Key, "degraded": true},
	}
	return p.svc.SubmitRequest(ctx, req, p.tpl.ID)
}

// Template 暴露模板（只读）。
func (p *SelfiePipeline) Template() SelfieTemplate { return p.tpl }
