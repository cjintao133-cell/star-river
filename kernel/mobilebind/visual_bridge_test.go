package mobilebind

// 视觉桥测试：mock 图像服务 → init → 生图（验证水印已加、返回有效 PNG）；自拍缺人设图报错。

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func makePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 4), 120, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestVisualBridgeGenerateAndWatermark(t *testing.T) {
	raw := makePNG(t, 64, 64)
	outB64 := base64.StdEncoding.EncodeToString(raw)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 纯文生图走 /images/generations
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"b64_json": outB64}}})
	}))
	defer srv.Close()

	if r := VisualInit(`{"base_url":"` + srv.URL + `/v1","api_key":"k","model":"img"}`); r != "OK" {
		t.Fatalf("VisualInit: %s", r)
	}
	if !strings.Contains(VisualStatus(), "\"configured\":true") {
		t.Fatalf("status 应 configured: %s", VisualStatus())
	}
	res := VisualGenerateImage("一只示例角色", 64, 64)
	if strings.HasPrefix(res, "ERR:") {
		t.Fatalf("生图失败: %s", res)
	}
	var ir imageResult
	if err := json.Unmarshal([]byte(res), &ir); err != nil {
		t.Fatalf("bad result json: %s", res)
	}
	out, err := base64.StdEncoding.DecodeString(ir.ImageB64)
	if err != nil {
		t.Fatalf("bad b64: %v", err)
	}
	if len(out) == 0 || !bytes.HasPrefix(out, []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("返回非 PNG 或空")
	}
	// 水印生效：输出应与原图字节不同（强制加了水印）。
	if bytes.Equal(out, raw) {
		t.Fatalf("水印未生效：输出与原图完全相同")
	}
	if ir.MIME == "" {
		t.Fatalf("mime 缺失")
	}
}

func TestVisualSelfieRequiresPersona(t *testing.T) {
	raw := makePNG(t, 64, 64)
	outB64 := base64.StdEncoding.EncodeToString(raw)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"b64_json": outB64}}})
	}))
	defer srv.Close()
	VisualInit(`{"base_url":"` + srv.URL + `/v1"}`)

	if r := InitKernel(filepath.Join(t.TempDir(), "v.db"), base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))); r != "OK" {
		t.Fatalf("InitKernel: %s", r)
	}
	if r := CreateCharacter(`{"id":"c-v","name":"示例角色"}`); r != "OK" {
		t.Fatalf("CreateCharacter: %s", r)
	}
	got := VisualSelfie("c-v", "")
	if !strings.HasPrefix(got, "ERR:") || !strings.Contains(got, "人设图") {
		t.Fatalf("无人设图应报缺人设图: %q", got)
	}
}

func TestVisualNotInit(t *testing.T) {
	visMu.Lock(); visSvc = nil; visMu.Unlock()
	if got := VisualGenerateImage("x", 8, 8); !strings.Contains(got, "not initialized") {
		t.Fatalf("应未初始化: %q", got)
	}
}

func TestVisualScenes(t *testing.T) {
	got := VisualScenes()
	if !strings.Contains(got, "\"key\"") {
		t.Fatalf("场景应含 key: %s", got)
	}
}
