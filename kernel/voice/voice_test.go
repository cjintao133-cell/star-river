package voice

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranscribe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/transcriptions" {
			t.Errorf("路径不对: %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("应为 multipart: %s", r.Header.Get("Content-Type"))
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("缺 Authorization")
		}
		// 读一下 multipart 确认带 file 字段
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "audio.webm") {
			t.Errorf("multipart 缺 file: %s", string(body))
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "你好世界", "language": "zh"})
	}))
	defer srv.Close()
	c := NewOpenAIClient(srv.URL, "k")
	tr, err := c.Transcribe(context.Background(), AudioInput{Data: []byte("AUDIODATA"), Filename: "audio.webm", Language: "zh"})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Text != "你好世界" || tr.Language != "zh" {
		t.Errorf("识别结果不对: %+v", tr)
	}
}

func TestTranscribeErrors(t *testing.T) {
	c := NewOpenAIClient("http://x", "k")
	if _, err := c.Transcribe(context.Background(), AudioInput{}); err != ErrEmptyAudio {
		t.Errorf("空音频应 ErrEmptyAudio, got %v", err)
	}
	// 401
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	defer srv.Close()
	_, err := NewOpenAIClient(srv.URL, "k").Transcribe(context.Background(), AudioInput{Data: []byte("a")})
	if ae, ok := err.(*APIError); !ok || ae.Kind != KindAuth {
		t.Errorf("401 应 KindAuth, got %v", err)
	}
}

func TestSynthesize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/speech" {
			t.Errorf("路径不对: %s", r.URL.Path)
		}
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		if b["input"] != "你好" || b["voice"] != "alloy" {
			t.Errorf("请求体不对: %v", b)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("AUDIOBYTES"))
	}))
	defer srv.Close()
	c := NewOpenAIClient(srv.URL, "k")
	out, err := c.Synthesize(context.Background(), "你好", TtsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if string(out.Data) != "AUDIOBYTES" || out.MIMEType != "audio/mpeg" {
		t.Errorf("合成结果不对: %+v", out)
	}
}

func TestSynthesizeEmptyText(t *testing.T) {
	c := NewOpenAIClient("http://x", "k")
	if _, err := c.Synthesize(context.Background(), "  ", TtsOptions{}); err == nil {
		t.Error("空文本应报错")
	}
}

func TestNoopPreprocessor(t *testing.T) {
	in := AudioInput{Data: []byte("x"), MIMEType: "audio/wav"}
	out, err := NoopPreprocessor{}.Process(context.Background(), in)
	if err != nil || string(out.Data) != "x" {
		t.Errorf("直通不对: %+v %v", out, err)
	}
}

func TestMimeForFormat(t *testing.T) {
	cases := map[string]string{"mp3": "audio/mpeg", "wav": "audio/wav", "opus": "audio/ogg", "weird": "application/octet-stream"}
	for f, want := range cases {
		if got := mimeForFormat(f); got != want {
			t.Errorf("mimeForFormat(%q)=%q want %q", f, got, want)
		}
	}
}
