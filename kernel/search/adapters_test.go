package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"star-river/kernel/tool"
)

func TestSearXNGSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("路径不对: %s", r.URL.Path)
		}
		if r.URL.Query().Get("format") != "json" || r.URL.Query().Get("q") != "你好" {
			t.Errorf("参数不对: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "百度", "url": "https://baidu.com", "content": "搜索引擎", "thumbnail": "https://x/t.png", "publishedDate": "2026-01-01"},
				{"title": "无图", "url": "https://n.com", "content": "无缩略图"},
			},
		})
	}))
	defer srv.Close()
	p := &SearXNGProvider{BaseURL: srv.URL}
	rs, err := p.Search(context.Background(), Query{Text: "你好", Count: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 || rs[0].Title != "百度" || rs[0].Thumbnail == "" || rs[0].Source != "searxng" {
		t.Fatalf("结果不对: %+v", rs)
	}
	if rs[1].Thumbnail != "" {
		t.Errorf("第二条无图应空: %+v", rs[1])
	}
}

func TestSearXNGErrors(t *testing.T) {
	if _, err := (&SearXNGProvider{}).Search(context.Background(), Query{Text: "x"}); err == nil {
		t.Error("空 baseURL 应报错")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()
	_, err := (&SearXNGProvider{BaseURL: srv.URL}).Search(context.Background(), Query{Text: "x"})
	if ae, ok := err.(*APIError); !ok || ae.Kind != KindHTTP {
		t.Errorf("应 KindHTTP, got %v", err)
	}
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{bad"))
	}))
	defer srv2.Close()
	_, err = (&SearXNGProvider{BaseURL: srv2.URL}).Search(context.Background(), Query{Text: "x"})
	if ae, ok := err.(*APIError); !ok || ae.Kind != KindProtocol {
		t.Errorf("应 KindProtocol, got %v", err)
	}
}

func TestSearXNGHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()
	if err := (&SearXNGProvider{BaseURL: srv.URL}).Healthy(context.Background()); err != nil {
		t.Errorf("应健康: %v", err)
	}
	if err := (&SearXNGProvider{}).Healthy(context.Background()); err == nil {
		t.Error("空 baseURL 应不健康")
	}
}

func TestBraveSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/res/v1/web/search" {
			t.Errorf("路径不对: %s", r.URL.Path)
		}
		if r.Header.Get("X-Subscription-Token") != "key123" {
			t.Errorf("缺 token 头")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"web": map[string]any{"results": []map[string]any{
				{"title": "R1", "url": "https://r1.com", "description": "desc1",
					"thumbnail": map[string]any{"src": "https://t/r1.png"},
					"profile":   map[string]any{"img": "https://fav/r1.ico"}, "age": "2 days"},
			}},
		})
	}))
	defer srv.Close()
	p := &BraveProvider{APIKey: "key123", BaseURL: srv.URL}
	rs, err := p.Search(context.Background(), Query{Text: "q", Count: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || rs[0].Title != "R1" || rs[0].Thumbnail != "https://t/r1.png" || rs[0].Favicon != "https://fav/r1.ico" {
		t.Fatalf("结果不对: %+v", rs)
	}
}

func TestBraveErrors(t *testing.T) {
	if _, err := (&BraveProvider{}).Search(context.Background(), Query{Text: "x"}); err == nil {
		t.Error("空 key 应报错")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(429) }))
	defer srv.Close()
	_, err := (&BraveProvider{APIKey: "k", BaseURL: srv.URL}).Search(context.Background(), Query{Text: "x"})
	if ae, ok := err.(*APIError); !ok || ae.Kind != KindRateLimit {
		t.Errorf("429 应 KindRateLimit, got %v", err)
	}
}

// fakeCaller 模拟一个 MCP 搜索工具。
type fakeCaller struct {
	text    string
	isErr   bool
	err     error
	gotTool string
	gotArgs map[string]any
}

func (f *fakeCaller) CallTool(_ context.Context, name string, args map[string]any) (*tool.CallResult, error) {
	f.gotTool = name
	f.gotArgs = args
	if f.err != nil {
		return nil, f.err
	}
	return &tool.CallResult{Content: []tool.Content{{Type: "text", Text: f.text}}, IsError: f.isErr}, nil
}

func TestMCPProviderSearch(t *testing.T) {
	fc := &fakeCaller{text: `{"results":[{"title":"百度一下","url":"https://baidu.com","snippet":"搜索"},{"title":"带图","url":"https://x","content":"desc","img":"https://img/x.png"}]}`}
	p := &MCPProvider{ServerName: "baidu-mcp", ToolName: "search", Caller: fc}
	rs, err := p.Search(context.Background(), Query{Text: "天气", Count: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 || rs[0].Title != "百度一下" || rs[1].Thumbnail != "https://img/x.png" {
		t.Fatalf("MCP 结果不对: %+v", rs)
	}
	if fc.gotTool != "search" || fc.gotArgs["query"] != "天气" {
		t.Errorf("调用参数不对: tool=%s args=%v", fc.gotTool, fc.gotArgs)
	}
	if rs[0].Source != "mcp:baidu-mcp" {
		t.Errorf("来源标记不对: %s", rs[0].Source)
	}
}

func TestMCPProviderErrors(t *testing.T) {
	// 未配置
	if _, err := (&MCPProvider{}).Search(context.Background(), Query{Text: "x"}); err == nil {
		t.Error("未配置应报错")
	}
	// 工具 IsError
	fc := &fakeCaller{text: "failed", isErr: true}
	_, err := (&MCPProvider{ToolName: "s", Caller: fc}).Search(context.Background(), Query{Text: "x"})
	if err == nil {
		t.Error("工具错误应上抛")
	}
	// 传输错误
	fc2 := &fakeCaller{err: context.DeadlineExceeded}
	if _, err := (&MCPProvider{ToolName: "s", Caller: fc2}).Search(context.Background(), Query{Text: "x"}); err == nil {
		t.Error("传输错误应上抛")
	}
}

func TestDefaultParseMCPResults(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{`[{"title":"a","url":"u1"}]`, 1},
		{`{"results":[{"title":"a","url":"u1"},{"name":"b","link":"u2"}]}`, 2},
		{`{"items":[{"title":"x","url":"y"}]}`, 1},
		{`{"data":[{"title":"x","url":"y"}]}`, 1},
		{`{"web":{"results":[{"title":"x","url":"y"}]}}`, 1},
		{`说明文字 {"results":[{"title":"x","url":"y"}]} 结尾说明`, 1},
		{"", 0},
	}
	for i, c := range cases {
		rs, err := DefaultParseMCPResults(c.in)
		if err != nil {
			t.Errorf("case %d 解析失败: %v", i, err)
			continue
		}
		if len(rs) != c.want {
			t.Errorf("case %d 应 %d 条, got %d", i, c.want, len(rs))
		}
	}
	// 无法解析
	if _, err := DefaultParseMCPResults("no json here"); err == nil {
		t.Error("无 JSON 应报错")
	}
	if _, err := DefaultParseMCPResults(`{"foo":1}`); err == nil {
		t.Error("无结果数组应报错")
	}
}
