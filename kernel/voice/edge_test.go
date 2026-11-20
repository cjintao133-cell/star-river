package voice

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTranscribeBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("{bad"))
	}))
	defer srv.Close()
	_, err := NewOpenAIClient(srv.URL, "k").Transcribe(context.Background(), AudioInput{Data: []byte("a")})
	if ae, ok := err.(*APIError); !ok || ae.Kind != KindProtocol {
		t.Errorf("坏 JSON 应 KindProtocol, got %v", err)
	}
}

func TestTranscribeNetworkError(t *testing.T) {
	_, err := NewOpenAIClient("http://127.0.0.1:1", "k").Transcribe(context.Background(), AudioInput{Data: []byte("a")})
	if ae, ok := err.(*APIError); !ok || (ae.Kind != KindNetwork && ae.Kind != KindTimeout) {
		t.Errorf("网络错应 network/timeout, got %v", err)
	}
}

func TestSynthesizeHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		_, _ = w.Write([]byte("rate"))
	}))
	defer srv.Close()
	_, err := NewOpenAIClient(srv.URL, "k").Synthesize(context.Background(), "hi", TtsOptions{})
	if ae, ok := err.(*APIError); !ok || ae.Kind != KindRateLimit {
		t.Errorf("429 应 KindRateLimit, got %v", err)
	}
}

func TestSynthesizeDefaultsAndSpeed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		if b["model"] != "tts-1" || b["response_format"] != "mp3" {
			t.Errorf("默认 model/format 不对: %v", b)
		}
		if b["speed"] != 1.5 {
			t.Errorf("speed 未透传: %v", b["speed"])
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("a"))
	}))
	defer srv.Close()
	if _, err := NewOpenAIClient(srv.URL, "k").Synthesize(context.Background(), "hi", TtsOptions{Speed: 1.5}); err != nil {
		t.Fatal(err)
	}
}

func TestClassifyStatus(t *testing.T) {
	if classifyStatus(403, "").Kind != KindAuth {
		t.Error("403 auth")
	}
	if classifyStatus(500, "x").Kind != KindHTTP {
		t.Error("500 http")
	}
	if (&APIError{Kind: KindHTTP, Status: 500, Message: "m"}).Error() == "" {
		t.Error("Error() 空")
	}
	if (&APIError{Kind: KindNetwork, Message: "m"}).Error() == "" {
		t.Error("Error() 无status 空")
	}
}

func TestDefaultsNewClient(t *testing.T) {
	c := NewOpenAIClient("http://h/v1/", "k")
	if c.BaseURL != "http://h/v1" {
		t.Errorf("尾斜杠应去除: %s", c.BaseURL)
	}
	if c.Name() != "openai-voice" {
		t.Errorf("Name 不对: %s", c.Name())
	}
}

// 编译期确认实现两接口
var (
	_ SttProvider  = (*OpenAIClient)(nil)
	_ TtsProvider  = (*OpenAIClient)(nil)
	_ Preprocessor = NoopPreprocessor{}
)
