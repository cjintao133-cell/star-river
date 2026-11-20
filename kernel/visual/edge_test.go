package visual

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNamesAndListers(t *testing.T) {
	if (&OpenAIImageBackend{}).Name() != "openai-image" {
		t.Error("openai Name 不对")
	}
	if (&ComfyUIBackend{}).Name() != "comfyui" {
		t.Error("comfyui Name 不对")
	}
	svc, _ := NewService(&fakeBackend{}, NewWatermarker(), 1)
	if svc.Backend() != "fake" {
		t.Errorf("Service.Backend 不对: %s", svc.Backend())
	}
	if len(svc.ListJobs()) != 0 {
		t.Error("初始无任务")
	}
}

func TestTruncate(t *testing.T) {
	if truncate("abc", 5) != "abc" {
		t.Error("短串原样")
	}
	if got := truncate(strings.Repeat("x", 300), 200); len(got) != 200 {
		t.Errorf("应截断到 200, got %d", len(got))
	}
}

func TestOpenAIBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("{not json"))
	}))
	defer srv.Close()
	_, err := (&OpenAIImageBackend{BaseURL: srv.URL}).Generate(context.Background(), GenerateRequest{Prompt: "x"})
	mustKindVisual(t, err, "protocol")
}

func TestOpenAIFetchImageError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/images/generations" {
			_, _ = w.Write([]byte(`{"data":[{"url":"http://` + r.Host + `/img.png"}]}`))
			return
		}
		w.WriteHeader(500) // /img.png 失败
	}))
	defer srv.Close()
	_, err := (&OpenAIImageBackend{BaseURL: srv.URL + "/v1"}).Generate(context.Background(), GenerateRequest{Prompt: "x"})
	mustKindVisual(t, err, "http")
}

func fixedWorkflow(GenerateRequest) (json.RawMessage, error) { return json.RawMessage(`{}`), nil }

func TestComfyUINetworkError(t *testing.T) {
	// 指向一个关闭的端口 → network 错误
	b := &ComfyUIBackend{BaseURL: "http://127.0.0.1:1", WorkflowBuilder: fixedWorkflow}
	_, err := b.Generate(context.Background(), GenerateRequest{Prompt: "x"})
	mustKindVisual(t, err, "network")
}

func TestComfyUIBadSubmitJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{bad"))
	}))
	defer srv.Close()
	b := &ComfyUIBackend{BaseURL: srv.URL, WorkflowBuilder: fixedWorkflow}
	_, err := b.Generate(context.Background(), GenerateRequest{Prompt: "x"})
	mustKindVisual(t, err, "protocol")
}

func TestComfyUIPollCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/prompt":
			_, _ = w.Write([]byte(`{"prompt_id":"p1"}`))
		default: // history 永远空
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	b := &ComfyUIBackend{
		BaseURL: srv.URL, WorkflowBuilder: fixedWorkflow,
		PollInterval: 20 * time.Millisecond, PollTimeout: 10 * time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	_, err := b.Generate(ctx, GenerateRequest{Prompt: "x"})
	mustKindVisual(t, err, "canceled")
}

func TestRecipeEnumParamAndExtra(t *testing.T) {
	r := Recipe{
		ID: "r", Name: "n", PromptTemplate: "t",
		Params: []Param{
			{Key: "style", Type: ParamEnum, Options: []string{"a", "b"}},
			{Key: "cfg", Type: ParamFloat, Min: 1, Max: 20},
		},
	}
	req, err := r.BuildRequest(map[string]string{"style": "a", "cfg": "7.5"})
	if err != nil {
		t.Fatal(err)
	}
	if req.Extra["style"] != "a" || req.Extra["cfg"] != "7.5" {
		t.Errorf("非约定 key 应进 Extra: %+v", req.Extra)
	}
}
