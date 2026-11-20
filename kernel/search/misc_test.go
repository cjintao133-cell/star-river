package search

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProvidersNameCaps(t *testing.T) {
	if (&SearXNGProvider{}).Name() != "searxng" {
		t.Error("searxng name")
	}
	if c := (&SearXNGProvider{}).Caps(); !c.HasThumbnail {
		t.Error("searxng caps")
	}
	if (&BraveProvider{}).Name() != "brave" {
		t.Error("brave name")
	}
	if c := (&BraveProvider{}).Caps(); !c.HasThumbnail {
		t.Error("brave caps")
	}
	mp := &MCPProvider{ServerName: "x", ToolName: "y"}
	if mp.Name() != "mcp:x" {
		t.Error("mcp name(server)")
	}
	if (&MCPProvider{ToolName: "y"}).Name() != "mcp:y" {
		t.Error("mcp name(tool)")
	}
	_ = mp.Caps()
	if (&SearXNGProvider{BaseURL: "u"}).String() == "" {
		t.Error("searxng string")
	}
}

func TestBraveHealthy(t *testing.T) {
	if err := (&BraveProvider{}).Healthy(context.Background()); err == nil {
		t.Error("空 key 应不健康")
	}
	if err := (&BraveProvider{APIKey: "k"}).Healthy(context.Background()); err != nil {
		t.Error("有 key 应健康")
	}
}

func TestMCPHealthyAndName(t *testing.T) {
	if err := (&MCPProvider{}).Healthy(context.Background()); err == nil {
		t.Error("未配置应不健康")
	}
	if err := (&MCPProvider{ToolName: "s", Caller: &fakeCaller{}}).Healthy(context.Background()); err != nil {
		t.Error("配置齐全应健康")
	}
}

func TestTimeRangeMappings(t *testing.T) {
	cases := map[string]string{"pd": "day", "pw": "month", "pm": "month", "py": "year", "": ""}
	for in, want := range cases {
		if got := searxngTimeRange(in); got != want {
			t.Errorf("searxng(%q)=%q want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"pd": "pd", "py": "py", "x": ""} {
		if got := braveFreshness(in); got != want {
			t.Errorf("brave(%q)=%q want %q", in, got, want)
		}
	}
}

func TestTransportErrorClassify(t *testing.T) {
	if ae := transportErr("p", context.DeadlineExceeded); ae.Kind != KindTimeout {
		t.Errorf("deadline 应 timeout, got %s", ae.Kind)
	}
	if ae := transportErr("p", errors.New("connection refused")); ae.Kind != KindNetwork {
		t.Errorf("普通错误应 network, got %s", ae.Kind)
	}
	// 带 Timeout() bool 接口
	type to struct{}
	_ = to{}
}

func TestSearXNGNetworkError(t *testing.T) {
	// 指向关闭端口
	p := &SearXNGProvider{BaseURL: "http://127.0.0.1:1"}
	_, err := p.Search(context.Background(), Query{Text: "x"})
	if ae, ok := err.(*APIError); !ok || (ae.Kind != KindNetwork && ae.Kind != KindTimeout) {
		t.Errorf("网络错误分类不对: %v", err)
	}
}

func TestClassifyStatus(t *testing.T) {
	if classifyStatus(401, "").Kind != KindAuth {
		t.Error("401 auth")
	}
	if classifyStatus(429, "").Kind != KindRateLimit {
		t.Error("429 rate")
	}
	if classifyStatus(503, "").Kind != KindHTTP {
		t.Error("503 http")
	}
	if classifyStatus(400, "xx").Kind != KindHTTP {
		t.Error("400 http")
	}
}

func TestAPIErrorString(t *testing.T) {
	if (&APIError{Kind: KindHTTP, Status: 500, Message: "x"}).Error() == "" {
		t.Error("带status Error 为空")
	}
	if (&APIError{Kind: KindNetwork, Message: "y"}).Error() == "" {
		t.Error("无status Error 为空")
	}
}

func TestSetTTL(t *testing.T) {
	r := NewRouter(nil, 0)
	r.SetTTL(time.Minute)
	r.SetTTL(0) // 0 不生效
	// 间接验证：Add 后可搜
	r.Add(&stubProvider{name: "a", res: []Result{{Title: "x"}}}, nil)
	if _, err := r.Search(context.Background(), Query{Text: "q"}); err != nil {
		t.Fatal(err)
	}
}

func TestTokenBucketAvailable(t *testing.T) {
	b := NewTokenBucket(5, 5, nil)
	_ = b.Allow()
	if b.Available() >= 5 {
		t.Errorf("用掉后应少: %f", b.Available())
	}
}

// 覆盖 realtime timeout 接口分支
type timeoutErr struct{}

func (timeoutErr) Error() string { return "i/o timeout" }
func (timeoutErr) Timeout() bool { return true }

func TestTimeoutInterface(t *testing.T) {
	if !isTimeout(timeoutErr{}) {
		t.Error("Timeout()=true 应识别为超时")
	}
	if ae := transportErr("p", timeoutErr{}); ae.Kind != KindTimeout {
		t.Errorf("应 timeout, got %s", ae.Kind)
	}
}

// 一个总是 200 但没有 results 的 searxng（空结果）
func TestSearXNGEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()
	rs, err := (&SearXNGProvider{BaseURL: srv.URL}).Search(context.Background(), Query{Text: "x"})
	if err != nil || len(rs) != 0 {
		t.Errorf("空结果应无错空切片: %v %v", err, rs)
	}
}
