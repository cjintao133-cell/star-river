package mobilebind

// 灾备路由（router）：可配置一个备用端点；主端点失败（未吐字前）自动切备用。
// 未配置备用时走原直连路径，行为零变化。用 kernel/adapter/router 实现。

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"star-river/kernel/adapter/anthropic"
	"star-river/kernel/adapter/gemini"
	"star-river/kernel/adapter/openai"
	"star-river/kernel/adapter/router"
	"star-river/kernel/core/types"
)

var (
	brMu           sync.RWMutex
	backupBase     string
	backupKey      string
	backupModel    string
	backupProvider string
)

// SetBackupEndpoint 设置备用端点（provider: openai/anthropic/gemini）。全空=清除备用。
func SetBackupEndpoint(base, key, model, provider string) string {
	brMu.Lock()
	defer brMu.Unlock()
	backupBase, backupKey, backupModel = strings.TrimSpace(base), key, strings.TrimSpace(model)
	backupProvider = normalizeProvider(provider)
	return "OK"
}

// BackupStatus 返回 {configured, provider, model}。
func BackupStatus() string {
	brMu.RLock()
	defer brMu.RUnlock()
	ok := backupBase != "" && backupModel != ""
	b, _ := json.Marshal(map[string]any{"configured": ok, "provider": backupProvider, "model": backupModel})
	return string(b)
}

func normalizeProvider(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "anthropic", "claude":
		return "anthropic"
	case "gemini", "google":
		return "gemini"
	default:
		return "openai"
	}
}

// dispatch 用指定协议适配器发起一次流式对话（caller 底层实现）。
func dispatch(provider, baseURL, apiKey, model string, msgs []types.Message, defs []openai.ToolDef, emit func(types.StreamEvent)) (string, error) {
	switch normalizeProvider(provider) {
	case "anthropic":
		c := anthropic.New(baseURL, apiKey)
		tds := make([]anthropic.ToolDef, 0, len(defs))
		for _, d := range defs {
			tds = append(tds, anthropic.ToolDef{Name: d.Name, Description: d.Description, Parameters: d.Parameters})
		}
		res, err := c.Chat(context.Background(), anthropic.ChatRequest{Model: model, Messages: msgs, MaxTokens: 1024, Tools: tds}, emit)
		if err != nil {
			return "", err
		}
		return res.Model, nil
	case "gemini":
		c := gemini.New(baseURL, apiKey)
		tds := make([]gemini.ToolDef, 0, len(defs))
		for _, d := range defs {
			tds = append(tds, gemini.ToolDef{Name: d.Name, Description: d.Description, Parameters: d.Parameters})
		}
		res, err := c.Chat(context.Background(), gemini.ChatRequest{Model: model, Messages: msgs, MaxTokens: 1024, Tools: tds}, emit)
		if err != nil {
			return "", err
		}
		return res.Model, nil
	default:
		c := openai.NewWithHeaders(baseURL, apiKey, currentHeaders())
		res, err := c.Chat(context.Background(), openai.ChatRequest{Model: model, Messages: msgs, MaxTokens: 1024, Tools: defs}, emit)
		if err != nil {
			return "", err
		}
		return res.Model, nil
	}
}

// sendWithFailover 主备路由发送：有备用则用 router（失败切换），否则直连主端点。
func sendWithFailover(ctx context.Context, baseURL, apiKey, model string, msgs []types.Message, defs []openai.ToolDef, emit func(types.StreamEvent)) (string, error) {
	primary := ChatProvider()
	brMu.RLock()
	bBase, bKey, bModel, bProv := backupBase, backupKey, backupModel, backupProvider
	brMu.RUnlock()

	if bBase == "" || bModel == "" {
		return dispatch(primary, baseURL, apiKey, model, msgs, defs, emit)
	}
	eps := []router.Endpoint{
		{Name: primary, BaseURL: baseURL, APIKey: apiKey, Model: model, Tier: router.TierUnknown},
		{Name: bProv, BaseURL: bBase, APIKey: bKey, Model: bModel, Tier: router.TierUnknown},
	}
	caller := router.CallerFunc(func(c context.Context, ep router.Endpoint, emit func(types.StreamEvent)) (string, error) {
		return dispatch(normalizeProvider(ep.Name), ep.BaseURL, ep.APIKey, ep.Model, msgs, defs, emit)
	})
	r, err := router.New(eps, caller, router.WithAllowHigherTier(true), router.WithPublisher(busPublisher()))
	if err != nil {
		return "", err
	}
	res, err := r.Chat(ctx, emit)
	if err != nil {
		return "", err
	}
	return res.Model, nil
}
