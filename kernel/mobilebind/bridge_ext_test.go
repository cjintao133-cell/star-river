package mobilebind

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebSearchSearxngBridge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{"title": "T", "url": "https://u", "content": "S", "thumbnail": "https://t"}},
		})
	}))
	defer srv.Close()
	out := WebSearch("searxng", srv.URL, "", "q", 5, "")
	if strings.HasPrefix(out, "ERR:") {
		t.Fatalf("应成功: %s", out)
	}
	var rs []searchResult
	if err := json.Unmarshal([]byte(out), &rs); err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || rs[0].Title != "T" || rs[0].Thumbnail != "https://t" {
		t.Errorf("结果不对: %+v", rs)
	}
}

func TestWebSearchErrors(t *testing.T) {
	if !strings.HasPrefix(WebSearch("searxng", "x", "", "  ", 5, ""), "ERR:") {
		t.Error("空 query 应 ERR")
	}
	if !strings.HasPrefix(WebSearch("bogus", "x", "", "q", 5, ""), "ERR:") {
		t.Error("未知 provider 应 ERR")
	}
}

func TestSttBridge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "你好", "language": "zh"})
	}))
	defer srv.Close()
	audio := base64.StdEncoding.EncodeToString([]byte("AUDIO"))
	out := SttTranscribe(srv.URL, "k", audio, "a.webm", "zh")
	var m map[string]string
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("应成功 JSON: %s", out)
	}
	if m["text"] != "你好" {
		t.Errorf("识别文本不对: %v", m)
	}
	// 坏 base64
	if !strings.HasPrefix(SttTranscribe(srv.URL, "k", "!!!notbase64", "a", ""), "ERR:") {
		t.Error("坏 base64 应 ERR")
	}
	// 空音频
	if !strings.HasPrefix(SttTranscribe(srv.URL, "k", "", "a", ""), "ERR:") {
		t.Error("空音频应 ERR")
	}
}

func TestTtsBridge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("AUDIOOUT"))
	}))
	defer srv.Close()
	out := TtsSynthesize(srv.URL, "k", "你好", "alloy", "mp3")
	var m map[string]string
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("应成功 JSON: %s", out)
	}
	if m["mime"] != "audio/mpeg" {
		t.Errorf("mime 不对: %v", m)
	}
	data, _ := base64.StdEncoding.DecodeString(m["audio_base64"])
	if string(data) != "AUDIOOUT" {
		t.Errorf("音频字节不对: %s", data)
	}
	// 空文本
	if !strings.HasPrefix(TtsSynthesize(srv.URL, "k", "  ", "", ""), "ERR:") {
		t.Error("空文本应 ERR")
	}
}

func TestModelCardsBridge(t *testing.T) {
	in := `[{"id":"gpt-4o"},{"id":"text-embedding-3-large"},{"id":"deepseek-r1"}]`
	out := ModelCards(in)
	var cards []map[string]any
	if err := json.Unmarshal([]byte(out), &cards); err != nil {
		t.Fatalf("应成功 JSON: %s", out)
	}
	if len(cards) != 3 {
		t.Fatalf("应 3 卡, got %d", len(cards))
	}
	// 坏 JSON
	if !strings.HasPrefix(ModelCards("{bad"), "ERR:") {
		t.Error("坏 JSON 应 ERR")
	}
}

func TestProbeModelBridge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/embeddings":
			w.WriteHeader(200)
		default:
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		}
	}))
	defer srv.Close()
	out := ProbeModel(srv.URL, "k", "m")
	var caps map[string]any
	if err := json.Unmarshal([]byte(out), &caps); err != nil {
		t.Fatalf("应成功 JSON: %s", out)
	}
	if caps["streaming"] != true || caps["model_id"] != "m" {
		t.Errorf("探测能力不对: %v", caps)
	}
	// 未配置
	if !strings.HasPrefix(ProbeModel("", "k", "m"), "ERR:") {
		t.Error("空 endpoint 应 ERR")
	}
}
