package mobilebind

// 应用请求头 + 设备策略测试。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestHeaderDefaultAndCustom(t *testing.T) {
	SetRequestHeaders("") // 清空→仅默认标识头
	var gotClient, gotTag, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClient = r.Header.Get("X-Star-River-Client")
		gotTag = r.Header.Get("X-Star-River-Tag")
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	// 默认：应带 X-Star-River-Client
	s := &recSink{}
	ChatStream(srv.URL+"/v1", "sk-x", "m", "", "hi", s)
	if !strings.HasPrefix(gotClient, "star-river-kernel/") {
		t.Fatalf("默认应用头缺失: %q", gotClient)
	}
	if gotAuth != "Bearer sk-x" {
		t.Fatalf("Authorization 应正常: %q", gotAuth)
	}

	// 自定义 tag + 尝试覆盖 Authorization（应被拒绝）
	SetRequestHeaders(`{"X-Star-River-Tag":"用户","Authorization":"Bearer HACK"}`)
	ChatStream(srv.URL+"/v1", "sk-y", "m", "", "hi", s)
	if gotTag != "用户" {
		t.Fatalf("自定义头应生效: %q", gotTag)
	}
	if gotAuth != "Bearer sk-y" {
		t.Fatalf("Authorization 不应被覆盖: %q", gotAuth)
	}
	SetRequestHeaders("")
}

func TestSetRequestHeadersBadJSON(t *testing.T) {
	if r := SetRequestHeaders("{bad"); !strings.HasPrefix(r, "ERR:") {
		t.Fatalf("坏 JSON 应报错: %q", r)
	}
	SetRequestHeaders("")
}

func TestDeviceStrategyJSON(t *testing.T) {
	got := DeviceStrategy()
	if !strings.Contains(got, "\"tier\":") || !strings.Contains(got, "\"policy\":") ||
		!strings.Contains(got, "\"max_history\":") || !strings.Contains(got, "\"animation\":") {
		t.Fatalf("设备策略 JSON 字段缺失: %s", got)
	}
}
