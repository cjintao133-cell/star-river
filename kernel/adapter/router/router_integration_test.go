package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cjintao133-cell/star-river/kernel/adapter/openai"
	"github.com/cjintao133-cell/star-river/kernel/bus"
	"github.com/cjintao133-cell/star-river/kernel/core/types"
)

// 集成：真 openai.Client 打 httptest。主端点 500、备端点 SSE 正常 → 路由切备用、
// 透传备用吐字、广播门铃、结果带备用应答模型。证明三协议适配器可被 Caller 串联做灾备。
func TestRouterIntegratesWithOpenAIClients(t *testing.T) {
	primarySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"error":{"message":"primary boom"}}`))
	}))
	defer primarySrv.Close()

	fallbackSSE := "data: {\"id\":\"x\",\"model\":\"backup-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"x\",\"model\":\"backup-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"
	fallbackSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Request-Id", "req-fb")
		w.Write([]byte(fallbackSSE))
	}))
	defer fallbackSrv.Close()

	clients := map[string]*openai.Client{}
	pc := openai.New(primarySrv.URL+"/v1", "sk-p")
	pc.MaxRetries = 0 // 主失败立即返回，不触发内部重试
	fc := openai.New(fallbackSrv.URL+"/v1", "sk-f")
	fc.MaxRetries = 0
	clients["primary"] = pc
	clients["fallback"] = fc

	caller := CallerFunc(func(ctx context.Context, ep Endpoint, emit func(types.StreamEvent)) (string, error) {
		cl := clients[ep.Name]
		req := openai.ChatRequest{
			Model:     ep.Model,
			MaxTokens: 64,
			Messages: []types.Message{
				{Role: types.RoleUser, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: "hi"}}},
			},
		}
		res, err := cl.Chat(ctx, req, emit)
		if err != nil {
			return "", err
		}
		return res.Model, nil
	})

	b := bus.New(64)
	sub, _ := b.Subscribe()
	defer b.Close()

	r, err := New([]Endpoint{
		{Name: "primary", BaseURL: primarySrv.URL + "/v1", APIKey: "sk-p", Model: "main-model", Tier: TierStandard},
		{Name: "fallback", BaseURL: fallbackSrv.URL + "/v1", APIKey: "sk-f", Model: "backup-model", Tier: TierCheap},
	}, caller, WithPublisher(b))
	if err != nil {
		t.Fatal(err)
	}

	var texts []string
	res, err := r.Chat(context.Background(), func(e types.StreamEvent) {
		if e.Kind == types.StreamDelta {
			texts = append(texts, e.Text)
		}
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if !res.Switched || res.Endpoint != "fallback" || res.Model != "backup-model" {
		t.Errorf("result = %+v", res)
	}
	if len(texts) != 1 || texts[0] != "hello" {
		t.Errorf("透传备用吐字 = %v", texts)
	}
	if events := drain(t, sub); len(events) != 1 || events[0].Topic != "adapter.route.failover" {
		t.Errorf("switch events = %+v", events)
	}
}
