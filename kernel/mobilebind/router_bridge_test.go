package mobilebind

// 灾备路由：主端点 500 → 自动切备用并成功；未配备用 → 直连。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"star-river/kernel/core/types"
)

func sseOK(model string) string {
	return "data: {\"id\":\"x\",\"model\":\"" + model + "\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"x\",\"model\":\"" + model + "\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"
}

func TestFailoverToBackup(t *testing.T) {
	SetChatProvider("openai")
	defer SetChatProvider("openai")
	defer SetBackupEndpoint("", "", "", "")

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"boom"}}`))
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(sseOK("backup-model")))
	}))
	defer good.Close()

	SetBackupEndpoint(good.URL, "k", "backup-model", "openai")
	if !strings.Contains(BackupStatus(), "\"configured\":true") {
		t.Fatalf("备用应已配置: %s", BackupStatus())
	}
	msgs := []types.Message{{Role: types.RoleUser, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: "hi"}}}}
	var sb strings.Builder
	model, err := sendWithFailover(context.Background(), bad.URL, "k", "m", msgs, nil, func(e types.StreamEvent) {
		if e.Kind == types.StreamDelta {
			sb.WriteString(e.Text)
		}
	})
	if err != nil {
		t.Fatalf("应切备用成功: %v", err)
	}
	if model != "backup-model" || sb.String() != "ok" {
		t.Fatalf("应由备用返回: model=%q text=%q", model, sb.String())
	}
}

func TestNoBackupDirectPath(t *testing.T) {
	SetChatProvider("openai")
	SetBackupEndpoint("", "", "", "")
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(sseOK("primary-model")))
	}))
	defer good.Close()
	msgs := []types.Message{{Role: types.RoleUser, Blocks: []types.ContentBlock{{Type: types.BlockText, Text: "hi"}}}}
	model, err := sendWithFailover(context.Background(), good.URL, "k", "primary-model", msgs, nil, func(types.StreamEvent) {})
	if err != nil || model != "primary-model" {
		t.Fatalf("无备用应直连成功: model=%q err=%v", model, err)
	}
}
