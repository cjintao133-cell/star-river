package visual

// 图生图（垫图自拍）路由测试：ImageRef 非空 → /images/edits(multipart)；否则 /images/generations(JSON)。

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 最小 1x1 PNG（DetectContentType 识别为 image/png）。
var tinyPNG = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
	0x89, 0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
	0x42, 0x60, 0x82,
}

func TestOpenAIImageEditsWhenImageRef(t *testing.T) {
	var gotPath, gotCT, gotPrompt, gotImagePart string
	outB64 := base64.StdEncoding.EncodeToString(tinyPNG)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		if strings.Contains(gotCT, "multipart/form-data") {
			_ = r.ParseMultipartForm(1 << 20)
			gotPrompt = r.FormValue("prompt")
			if f, _, err := r.FormFile("image"); err == nil {
				b, _ := io.ReadAll(f)
				gotImagePart = string(b)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"b64_json": outB64}}})
	}))
	defer srv.Close()

	b := &OpenAIImageBackend{BaseURL: srv.URL + "/v1", APIKey: "sk-x"}
	res, err := b.Generate(context.Background(), GenerateRequest{
		Prompt: "示例角色自拍，窗边", ImageRef: tinyPNG, Model: "img-model", Width: 512, Height: 512,
	})
	if err != nil {
		t.Fatalf("Generate err: %v", err)
	}
	if !strings.HasSuffix(gotPath, "/images/edits") {
		t.Fatalf("有垫图应走 /images/edits，实际 %s", gotPath)
	}
	if !strings.Contains(gotCT, "multipart/form-data") {
		t.Fatalf("edits 应为 multipart，实际 %s", gotCT)
	}
	if gotPrompt != "示例角色自拍，窗边" {
		t.Fatalf("prompt 未传: %q", gotPrompt)
	}
	if gotImagePart != string(tinyPNG) {
		t.Fatalf("人设图字节未原样上传")
	}
	if len(res.Images) != 1 || string(res.Images[0]) != string(tinyPNG) {
		t.Fatalf("返回图像不符")
	}
}

func TestOpenAIImageGenerationsWhenNoImageRef(t *testing.T) {
	var gotPath string
	outB64 := base64.StdEncoding.EncodeToString(tinyPNG)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"b64_json": outB64}}})
	}))
	defer srv.Close()

	b := &OpenAIImageBackend{BaseURL: srv.URL + "/v1"}
	res, err := b.Generate(context.Background(), GenerateRequest{Prompt: "纯文生图"})
	if err != nil {
		t.Fatalf("Generate err: %v", err)
	}
	if !strings.HasSuffix(gotPath, "/images/generations") {
		t.Fatalf("无垫图应走 /images/generations，实际 %s", gotPath)
	}
	if len(res.Images) != 1 {
		t.Fatalf("返回图像不符")
	}
}
