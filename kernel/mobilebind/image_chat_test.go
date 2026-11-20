package mobilebind

// 多模态：ChatStreamImage 带图请求，验证请求体里图片块(image_url data-uri)确实发出。

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestChatStreamImageSendsImageBlock(t *testing.T) {
	var gotBody string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock(); gotBody = string(b); mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"id\":\"x\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"看到了\"},\"finish_reason\":null}]}\n\n"))
		w.Write([]byte("data: {\"id\":\"x\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	// 1x1 PNG base64
	imgB64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	s := &recSink{}
	ChatStreamImage(srv.URL+"/v1", "k", "m", "人设", "这是什么", imgB64, "image/png", s)
	if s.err != "" {
		t.Fatalf("err: %s", s.err)
	}
	mu.Lock(); body := gotBody; mu.Unlock()
	if !strings.Contains(body, "image_url") && !strings.Contains(body, "data:image/png;base64,") {
		t.Fatalf("请求体应含图片块: %s", body[:min(len(body),400)])
	}
	if !strings.Contains(body, "这是什么") {
		t.Fatalf("文本未带上: %s", body[:min(len(body),400)])
	}
}

func min(a, b int) int { if a < b { return a }; return b }
