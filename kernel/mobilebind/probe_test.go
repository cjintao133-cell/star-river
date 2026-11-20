package mobilebind

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeEndpointSuccess(t *testing.T) {
	sse := "data: {\"id\":\"x\",\"model\":\"probe-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"x\",\"model\":\"probe-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(sse))
	}))
	defer srv.Close()
	var res ProbeResult
	if err := json.Unmarshal([]byte(ProbeEndpoint(srv.URL+"/v1", "k", "probe-model")), &res); err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Model != "probe-model" {
		t.Errorf("probe = %+v", res)
	}
}

func TestProbeEndpointAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Request-Id", "req-err")
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"message":"bad key sk-SECRET"}}`))
	}))
	defer srv.Close()
	var res ProbeResult
	json.Unmarshal([]byte(ProbeEndpoint(srv.URL+"/v1", "sk-SECRET", "m")), &res)
	if res.OK {
		t.Fatal("应失败")
	}
	if res.Category != "auth" {
		t.Errorf("category = %s, want auth", res.Category)
	}
	if res.RequestID != "req-err" {
		t.Errorf("requestId = %q", res.RequestID)
	}
	if contains(res.Error, "sk-SECRET") {
		t.Errorf("key 泄漏: %s", res.Error)
	}
}

func TestProbeEndpointEmptyConfig(t *testing.T) {
	var res ProbeResult
	json.Unmarshal([]byte(ProbeEndpoint("", "k", "")), &res)
	if res.OK || res.Category != "bad_request" {
		t.Errorf("空配置 = %+v", res)
	}
}
