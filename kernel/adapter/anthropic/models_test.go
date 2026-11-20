package anthropic

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestModelsOK(t *testing.T) {
	b := fixture(t, "models_ok.json")
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) { writeJSON(w, 200, b) })
	got, err := c.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	want := []ModelInfo{
		{ID: "claude-3-5-sonnet-20241022", DisplayName: "Claude 3.5 Sonnet", Type: "model", CreatedAt: "2024-10-22T00:00:00Z"},
		{ID: "claude-3-opus-20240229", DisplayName: "Claude 3 Opus", Type: "model", CreatedAt: "2024-02-29T00:00:00Z"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("models = %+v, want %+v", got, want)
	}
	if st.method(0) != http.MethodGet || st.path(0) != "/v1/models" {
		t.Errorf("%s %s", st.method(0), st.path(0))
	}
	if got := st.hdr(0).Get("x-api-key"); got != testKey {
		t.Errorf("x-api-key = %q", got)
	}
	if got := st.hdr(0).Get("anthropic-version"); got != "2023-06-01" {
		t.Errorf("anthropic-version = %q", got)
	}
}

func TestModelsEmptyData(t *testing.T) {
	for _, body := range []string{`{"type":"list"}`, `{"data":null}`} {
		c, _ := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) {
			writeJSON(w, 200, []byte(body))
		})
		got, err := c.Models(context.Background())
		if err != nil {
			t.Fatalf("Models(%s): %v", body, err)
		}
		if got == nil || len(got) != 0 {
			t.Errorf("Models(%s) = %+v, want 空切片非nil", body, got)
		}
	}
}

func TestModelsBadJSON(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) {
		writeJSON(w, 200, []byte(`<html>gateway</html>`))
	})
	_, err := c.Models(context.Background())
	if ae := asAPIError(t, err); ae.Kind != KindProtocol {
		t.Errorf("kind = %s", ae.Kind)
	}
}

func TestModelsTimeout(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) {
		time.Sleep(200 * time.Millisecond)
		writeJSON(w, 200, []byte(`{"data":[]}`))
	})
	c.HTTP.Timeout = 50 * time.Millisecond
	c.MaxRetries = 0
	_, err := c.Models(context.Background())
	if ae := asAPIError(t, err); ae.Kind != KindTimeout {
		t.Errorf("kind = %s, err=%v", ae.Kind, err)
	}
}

func TestModelsRetryP6(t *testing.T) {
	b := fixture(t, "models_ok.json")
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) {
		if n == 1 {
			writeJSON(w, 500, []byte(`{"type":"error","error":{"type":"api_error","message":"boom"}}`))
			return
		}
		writeJSON(w, 200, b)
	})
	if _, err := c.Models(context.Background()); err != nil {
		t.Fatalf("P6 retry: %v", err)
	}
	if st.count() != 2 {
		t.Errorf("requests = %d", st.count())
	}
	if w := st.sleepWaits(); !reflect.DeepEqual(w, []time.Duration{time.Millisecond}) {
		t.Errorf("waits = %v", w)
	}
}

func TestModelsAuthP4(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, n int, b []byte) {
		w.Header().Set("Request-Id", "req-fallback")
		w.WriteHeader(401)
		w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid key"}}`))
	})
	_, err := c.Models(context.Background())
	ae := asAPIError(t, err)
	if ae.Kind != KindAuth || ae.RequestID != "req-fallback" {
		t.Errorf("kind=%s rid=%q", ae.Kind, ae.RequestID)
	}
}
