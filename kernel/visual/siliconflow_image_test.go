package visual

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 文本生图：返回 {data:[{b64_json}]}，应解析出 1 张图。
func TestSiliconFlowTextToImageData(t *testing.T) {
	pngBytes := testPNG(t, 2, 2)
	b64 := base64.StdEncoding.EncodeToString(pngBytes)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Errorf("path=%s", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["image"] != nil {
			t.Errorf("文本生图不应带 image 字段")
		}
		if body["image_size"] != "64x64" {
			t.Errorf("image_size=%v", body["image_size"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + b64 + `"}]}`))
	}))
	defer srv.Close()

	b := &SiliconFlowImageBackend{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "Kwai-Kolors/Kolors"}
	res, err := b.Generate(context.Background(), GenerateRequest{Prompt: "cat", Width: 64, Height: 64})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(res.Images) != 1 || !bytes.Equal(res.Images[0], pngBytes) {
		t.Fatalf("images=%d", len(res.Images))
	}
}

// 图生图：请求 body 必须带 image data URL；返回 {images:[{url}]} 走 url 下载。
func TestSiliconFlowImageToImageURL(t *testing.T) {
	pngBytes := testPNG(t, 2, 2)
	var gotImage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/images/generations":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if s, ok := body["image"].(string); ok {
				gotImage = s
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"images":[{"url":"` + srvSchemeHost(r) + `/img/1.png"}]}`))
		case "/img/1.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := &SiliconFlowImageBackend{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "Qwen/Qwen-Image-Edit"}
	res, err := b.Generate(context.Background(), GenerateRequest{Prompt: "edit", ImageRef: pngBytes, Width: 64, Height: 64})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.HasPrefix(gotImage, "data:image/") || !strings.Contains(gotImage, "base64,") {
		t.Errorf("image 字段非 data URL: %.40q", gotImage)
	}
	if len(res.Images) != 1 || !bytes.Equal(res.Images[0], pngBytes) {
		t.Fatalf("images=%d", len(res.Images))
	}
}

// 错误状态：400 应返回 error。
func TestSiliconFlowHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer srv.Close()
	b := &SiliconFlowImageBackend{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m"}
	if _, err := b.Generate(context.Background(), GenerateRequest{Prompt: "x"}); err == nil {
		t.Fatal("应报错")
	}
}

// srvSchemeHost 从请求还原服务端 scheme://host。
func srvSchemeHost(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
