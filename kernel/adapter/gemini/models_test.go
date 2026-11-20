package gemini

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
	if len(got) != 2 {
		t.Fatalf("models = %d, want 2", len(got))
	}
	if got[0].Name != "models/gemini-1.5-pro" || got[0].ID() != "gemini-1.5-pro" || !got[0].SupportsGenerateContent() {
		t.Errorf("models[0] = %+v", got[0])
	}
	if got[1].Name != "models/text-embedding-004" || got[1].SupportsGenerateContent() {
		t.Errorf("models[1] = %+v", got[1])
	}
	if st.method(0) != http.MethodGet || st.path(0) != "/v1beta/models" {
		t.Errorf("%s %s", st.method(0), st.path(0))
	}
	if got := st.hdr(0).Get("x-goog-api-key"); got != testKey {
		t.Errorf("x-goog-api-key = %q", got)
	}
}

func TestChatModelsFilter(t *testing.T) {
	b := fixture(t, "models_ok.json")
	c, _ := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) { writeJSON(w, 200, b) })
	got, err := c.ChatModels(context.Background())
	if err != nil {
		t.Fatalf("ChatModels: %v", err)
	}
	want := []ModelInfo{{Name: "models/gemini-1.5-pro", DisplayName: "Gemini 1.5 Pro", Description: "chat model", SupportedGenerationMethods: []string{"generateContent", "countTokens"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("chatModels = %+v, want %+v", got, want)
	}
}

func TestModelsEmptyData(t *testing.T) {
	for _, body := range []string{`{"nextPageToken":""}`, `{"models":null}`} {
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
		writeJSON(w, 200, []byte(`{"models":[]}`))
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
			writeJSON(w, 500, []byte(`{"error":{"code":500,"status":"INTERNAL","message":"boom"}}`))
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
		w.Header().Set("X-Goog-Request-Id", "req-fallback")
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"code":401,"status":"UNAUTHENTICATED","message":"invalid key"}}`))
	})
	_, err := c.Models(context.Background())
	ae := asAPIError(t, err)
	if ae.Kind != KindAuth || ae.RequestID != "req-fallback" {
		t.Errorf("kind=%s rid=%q", ae.Kind, ae.RequestID)
	}
}
