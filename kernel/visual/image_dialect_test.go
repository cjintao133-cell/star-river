package visual

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 归一化覆盖各家返回结构。
func TestNormalizeShapes(t *testing.T) {
	png := testPNG(t, 2, 2)
	b64 := base64.StdEncoding.EncodeToString(png)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	defer srv.Close()
	host := srv.URL

	cases := map[string]string{
		"data_b64":    `{"data":[{"b64_json":"` + b64 + `"}]}`,
		"data_url":    `{"data":[{"url":"` + host + `/a.png"}]}`,
		"images_url":  `{"images":[{"url":"` + host + `/a.png"}]}`,
		"output_res":  `{"output":[{"type":"image_generation_call","result":"` + b64 + `"}]}`,
		"data_strurl": `{"data":"` + host + `/a.png"}`,
		"chat_imgs":   `{"choices":[{"message":{"images":[{"url":"` + host + `/a.png"}]}}]}`,
		"chat_parts":  `{"choices":[{"message":{"content":[{"type":"image_url","image_url":{"url":"` + host + `/a.png"}}]}}]}`,
	}
	for name, body := range cases {
		imgs, err := normalizeImageResponse(context.Background(), netxClient(), []byte(body))
		if err != nil {
			t.Errorf("%s: err=%v", name, err)
			continue
		}
		if len(imgs) != 1 || !bytes.Equal(imgs[0], png) {
			t.Errorf("%s: imgs=%d mismatch", name, len(imgs))
		}
	}
}

func TestNormalizeError(t *testing.T) {
	if _, err := normalizeImageResponse(context.Background(), netxClient(), []byte(`{"error":{"message":"boom"}}`)); err == nil {
		t.Fatal("应报错")
	}
	if _, err := normalizeImageResponse(context.Background(), netxClient(), []byte(`not json`)); err == nil {
		t.Fatal("坏 json 应报错")
	}
	if _, err := normalizeImageResponse(context.Background(), netxClient(), []byte(`{"foo":1}`)); err == nil {
		t.Fatal("无图应报错")
	}
}

// generations + image 字段方言（硅基型）：断言 body 带 image data URL、image_size+size、无 response_format。
func TestDialectGenerationsImage(t *testing.T) {
	png := testPNG(t, 2, 2)
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/i.png" {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
			return
		}
		if r.URL.Path != "/v1/images/generations" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"images":[{"url":"` + srvSchemeHost(r) + `/i.png"}]}`))
	}))
	defer srv.Close()
	b := &GenericImageBackend{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "Qwen/Qwen-Image-Edit", Dialect: DialectSiliconFlow()}
	res, err := b.Generate(context.Background(), GenerateRequest{Prompt: "edit", ImageRef: png, Width: 768, Height: 1024})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(res.Images) != 1 {
		t.Fatalf("images=%d", len(res.Images))
	}
	if s, _ := gotBody["image"].(string); !strings.HasPrefix(s, "data:image/") {
		t.Errorf("image 非 data URL: %.30q", gotBody["image"])
	}
	if gotBody["image_size"] == nil || gotBody["size"] == nil {
		t.Errorf("缺 image_size/size: %v", gotBody)
	}
	if _, ok := gotBody["response_format"]; ok {
		t.Errorf("硅基方言不应发 response_format")
	}
}

// edits multipart 方言（官方型）：断言 multipart 含 image 文件 + prompt + model。
func TestDialectEditsMultipart(t *testing.T) {
	png := testPNG(t, 2, 2)
	b64 := base64.StdEncoding.EncodeToString(png)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Errorf("multipart: %v", err)
			return
		}
		if _, _, err := r.FormFile("image"); err != nil {
			t.Errorf("缺 image 文件")
		}
		if r.FormValue("prompt") != "hi" || r.FormValue("model") != "gpt-image-1" {
			t.Errorf("prompt/model=%q/%q", r.FormValue("prompt"), r.FormValue("model"))
		}
		if r.FormValue("response_format") != "b64_json" {
			t.Errorf("应发 response_format")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + b64 + `"}]}`))
	}))
	defer srv.Close()
	b := &GenericImageBackend{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "gpt-image-1", Dialect: DialectOpenAI()}
	res, err := b.Generate(context.Background(), GenerateRequest{Prompt: "hi", ImageRef: png, Width: 1024, Height: 1024})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(res.Images) != 1 {
		t.Fatalf("images=%d", len(res.Images))
	}
}

// chat 型出图：断言 chat body 含 image_url；返回 choices.images 走 url。
func TestDialectChat(t *testing.T) {
	png := testPNG(t, 2, 2)
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/i.png" {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"images":[{"url":"` + srvSchemeHost(r) + `/i.png"}]}}]}`))
	}))
	defer srv.Close()
	d := DialectGeneric()
	d.Img2ImgMode = Img2ImgChat
	b := &GenericImageBackend{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "gpt-4o-image", Dialect: d}
	res, err := b.Generate(context.Background(), GenerateRequest{Prompt: "draw", ImageRef: png})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(res.Images) != 1 {
		t.Fatalf("images=%d", len(res.Images))
	}
	// 断言 content 数组里有 image_url
	raw, _ := json.Marshal(gotBody)
	if !strings.Contains(string(raw), "image_url") {
		t.Errorf("chat body 缺 image_url")
	}
}

func TestSnapImageSize(t *testing.T) {
	cases := []struct {
		model   string
		w, h    int
		wantSet map[string]bool
	}{
		{"dall-e-3", 768, 1024, map[string]bool{"1024x1792": true, "1024x1024": true}},
		{"gpt-image-1", 768, 1024, map[string]bool{"1024x1536": true, "1024x1024": true}},
		{"Kwai-Kolors/Kolors", 768, 1024, map[string]bool{"768x1024": true}},
	}
	for _, c := range cases {
		got, _ := snapImageSize(c.model, c.w, c.h)
		if !c.wantSet[got] {
			t.Errorf("%s %dx%d → %s 不在期望集 %v", c.model, c.w, c.h, got, c.wantSet)
		}
	}
}

func TestGenericHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer srv.Close()
	b := &GenericImageBackend{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m", Dialect: DialectGeneric()}
	if _, err := b.Generate(context.Background(), GenerateRequest{Prompt: "x"}); err == nil {
		t.Fatal("应报错")
	}
}

// netxClient 测试用无重定向客户端。
func netxClient() *http.Client { return &http.Client{} }

var _ = multipart.NewWriter // 保留 import（multipart 仅在 server 端 ParseMultipartForm 用）
