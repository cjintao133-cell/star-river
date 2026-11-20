package visual

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenAIImageSuccessB64(t *testing.T) {
	png := testPNG(t, 20, 20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Errorf("路径不对: %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("应带 Authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(png)}},
		})
	}))
	defer srv.Close()

	b := &OpenAIImageBackend{BaseURL: srv.URL + "/v1", APIKey: "sk-x", Model: "dall-e"}
	res, err := b.Generate(context.Background(), GenerateRequest{Prompt: "cat", Width: 1024, Height: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Images) != 1 || !bytesEqual(res.Images[0], png) {
		t.Fatalf("应返回 1 张原图")
	}
}

func TestOpenAIImageURLPath(t *testing.T) {
	png := testPNG(t, 16, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/images/generations":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{"url": "http://" + r.Host + "/img.png"}},
			})
		case "/img.png":
			_, _ = w.Write(png)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	b := &OpenAIImageBackend{BaseURL: srv.URL + "/v1"}
	res, err := b.Generate(context.Background(), GenerateRequest{Prompt: "cat"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Images) != 1 || !bytesEqual(res.Images[0], png) {
		t.Fatal("URL 取图失败")
	}
}

func TestOpenAIImageErrors(t *testing.T) {
	// HTTP 错误
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("err"))
	}))
	defer srv.Close()
	_, err := (&OpenAIImageBackend{BaseURL: srv.URL}).Generate(context.Background(), GenerateRequest{Prompt: "x"})
	mustKindVisual(t, err, "http")

	// error 字段
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "bad"}})
	}))
	defer srv2.Close()
	_, err = (&OpenAIImageBackend{BaseURL: srv2.URL}).Generate(context.Background(), GenerateRequest{Prompt: "x"})
	mustKindVisual(t, err, "rpc")

	// 空 data
	srv3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer srv3.Close()
	_, err = (&OpenAIImageBackend{BaseURL: srv3.URL}).Generate(context.Background(), GenerateRequest{Prompt: "x"})
	mustKindVisual(t, err, "protocol")

	// 空 prompt
	_, err = (&OpenAIImageBackend{BaseURL: "http://127.0.0.1:0"}).Generate(context.Background(), GenerateRequest{})
	mustKindVisual(t, err, "protocol")

	// img2img 现走 /images/edits（不再报 not supported）；base 不可达 → network
	_, err = (&OpenAIImageBackend{BaseURL: "http://127.0.0.1:0"}).Generate(context.Background(), GenerateRequest{Prompt: "x", ImageRef: []byte("r")})
	mustKindVisual(t, err, "network")
}

func TestComfyUINilBuilder(t *testing.T) {
	b := &ComfyUIBackend{BaseURL: "http://127.0.0.1:0"}
	_, err := b.Generate(context.Background(), GenerateRequest{Prompt: "x"})
	mustKindVisual(t, err, "protocol")
}

func TestComfyUISuccess(t *testing.T) {
	png := testPNG(t, 24, 24)
	var historyHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/prompt" && r.Method == "POST":
			_ = json.NewEncoder(w).Encode(map[string]any{"prompt_id": "p1"})
		case strings.HasPrefix(r.URL.Path, "/history/"):
			// 第一次空（模拟排队），第二次有结果 → 覆盖轮询分支
			if atomic.AddInt32(&historyHits, 1) == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"p1": map[string]any{"outputs": map[string]any{
					"9": map[string]any{"images": []map[string]string{{"filename": "a.png", "subfolder": "", "type": "output"}}},
				}},
			})
		case r.URL.Path == "/view":
			if r.URL.Query().Get("filename") != "a.png" {
				t.Errorf("view 参数不对: %s", r.URL.RawQuery)
			}
			_, _ = w.Write(png)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	b := &ComfyUIBackend{
		BaseURL: srv.URL, ClientID: "c1",
		WorkflowBuilder: func(req GenerateRequest) (json.RawMessage, error) {
			return json.RawMessage(`{"1":{"class_type":"KSampler"}}`), nil
		},
		PollInterval: 10 * time.Millisecond,
	}
	res, err := b.Generate(context.Background(), GenerateRequest{Prompt: "cat", Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Images) != 1 || !bytesEqual(res.Images[0], png) {
		t.Fatalf("ComfyUI 取图失败")
	}
}

func TestComfyUITimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/prompt":
			_ = json.NewEncoder(w).Encode(map[string]any{"prompt_id": "p1"})
		case strings.HasPrefix(r.URL.Path, "/history/"):
			_ = json.NewEncoder(w).Encode(map[string]any{}) // 永远空
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	b := &ComfyUIBackend{
		BaseURL: srv.URL,
		WorkflowBuilder: func(req GenerateRequest) (json.RawMessage, error) {
			return json.RawMessage(`{}`), nil
		},
		PollInterval: 10 * time.Millisecond, PollTimeout: 40 * time.Millisecond,
	}
	_, err := b.Generate(context.Background(), GenerateRequest{Prompt: "x"})
	mustKindVisual(t, err, "timeout")
}

func TestComfyUISubmitNoPromptID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "bad workflow"})
	}))
	defer srv.Close()
	b := &ComfyUIBackend{BaseURL: srv.URL, WorkflowBuilder: func(GenerateRequest) (json.RawMessage, error) { return json.RawMessage(`{}`), nil }}
	_, err := b.Generate(context.Background(), GenerateRequest{Prompt: "x"})
	mustKindVisual(t, err, "rpc")
}

// —— 辅助 ——

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func mustKindVisual(t *testing.T, err error, want string) {
	t.Helper()
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("期望 *APIError, got %v", err)
	}
	if ae.Kind != want {
		t.Fatalf("期望 Kind=%s, got %s (%v)", want, ae.Kind, err)
	}
}
