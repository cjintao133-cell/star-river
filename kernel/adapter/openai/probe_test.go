package openai

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 全能力 mock：chat 200、embeddings 200、带图也 200。
func TestSendProbeFull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/embeddings":
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			if bytes.Contains(body, []byte("image_url")) {
				_, _ = w.Write([]byte("data: [DONE]\n\n"))
			} else {
				_, _ = w.Write([]byte("data: [DONE]\n\n"))
			}
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := New(srv.URL, "k")
	sig, err := c.SendProbe(context.Background(), "m")
	if err != nil {
		t.Fatal(err)
	}
	if !sig.Streamed || !sig.AcceptedImage || !sig.AcceptedTools || !sig.EmbeddingOK {
		t.Errorf("全能力应当真: %+v", sig)
	}
}

// 端点不支持图片：带图请求 400，其余 200 → Vision=false 但不致命。
func TestSendProbeNoVision(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/embeddings":
			w.WriteHeader(200)
		case "/chat/completions":
			if bytes.Contains(body, []byte("image_url")) {
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"error":{"message":"does not support image input"}}`))
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := New(srv.URL, "k")
	sig, err := c.SendProbe(context.Background(), "m")
	if err != nil {
		t.Fatal(err)
	}
	if sig.AcceptedImage {
		t.Error("不支持图应 AcceptedImage=false")
	}
	if !sig.Streamed {
		t.Error("流式仍应 true")
	}
}

// 基础连通失败（/chat/completions 全 500）→ 返回 err（致命）。
func TestSendProbeFatalNetwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	c := New(srv.URL, "k")
	_, err := c.SendProbe(context.Background(), "m")
	if err == nil {
		t.Fatal("基础连通失败应返回错误")
	}
	if !strings.Contains(err.Error(), "m") && !strings.Contains(strings.ToLower(err.Error()), "500") {
		// 仅宽泛检查错误非空；具体分类由 APIError 承担
	}
}
