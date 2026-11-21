package openai

import (
	"context"
	"net/http"
	"testing"
)

func TestEmbedOK(t *testing.T) {
	c, st := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) {
		writeJSON(w, 200, []byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	})
	got, err := c.Embed(context.Background(), "m", "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(got) != 3 || got[0] != 0.1 {
		t.Fatalf("vec=%v", got)
	}
	if st.method(0) != http.MethodPost || st.path(0) != "/v1/embeddings" {
		t.Errorf("%s %s", st.method(0), st.path(0))
	}
	if st.hdr(0).Get("Authorization") != "Bearer "+testKey {
		t.Error("Authorization header wrong")
	}
}

func TestEmbedErrorObject(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) {
		writeJSON(w, 200, []byte(`{"error":{"message":"quota exceeded"}}`))
	})
	_, err := c.Embed(context.Background(), "m", "x")
	if ae := asAPIError(t, err); ae.Kind != KindProtocol {
		t.Errorf("kind=%s", ae.Kind)
	}
}

func TestEmbedBadJSON(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) {
		writeJSON(w, 200, []byte(`<html>gateway</html>`))
	})
	_, err := c.Embed(context.Background(), "m", "x")
	if ae := asAPIError(t, err); ae.Kind != KindProtocol {
		t.Errorf("kind=%s", ae.Kind)
	}
}

func TestEmbedEmptyData(t *testing.T) {
	for _, body := range []string{`{"data":[]}`, `{"data":[{"embedding":[]}]}`} {
		b := body
		c, _ := newTestClient(t, func(w http.ResponseWriter, n int, rb []byte) {
			writeJSON(w, 200, []byte(b))
		})
		_, err := c.Embed(context.Background(), "m", "x")
		if ae := asAPIError(t, err); ae.Kind != KindProtocol {
			t.Errorf("%s kind=%s", body, ae.Kind)
		}
	}
}

func TestEmbedHTTPError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, n int, body []byte) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	})
	_, err := c.Embed(context.Background(), "m", "x")
	ae := asAPIError(t, err)
	if ae.Kind != KindAuth {
		t.Errorf("kind=%s want auth", ae.Kind)
	}
}
