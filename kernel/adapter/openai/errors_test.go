package openai

import (
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestRedactKeyAndTruncation(t *testing.T) {
	got := redact("prefix "+testKey+" suffix", testKey)
	if got != "prefix [REDACTED] suffix" {
		t.Errorf("redact key: %q", got)
	}
	if redact("no key here", "") != "no key here" {
		t.Error("redact empty key must not alter short text")
	}
	long := strings.Repeat("\u20ac", 1000) // 3000 字节，2048 截断点落在 3 字节符文中间
	out := redact(long, "")
	if len(out) > maxErrBody {
		t.Errorf("redact truncation: len=%d", len(out))
	}
	if !utf8.ValidString(out) {
		t.Error("redact must keep valid UTF-8 after truncation")
	}
}

func TestWireErrorCodeString(t *testing.T) {
	cases := []struct{ raw, want string }{
		{`"invalid_api_key"`, "invalid_api_key"},
		{`503`, "503"},
		{``, ""},
	}
	for _, c := range cases {
		w := wireError{Code: []byte(c.raw)}
		if got := w.CodeString(); got != c.want {
			t.Errorf("CodeString(%s) = %q, want %q", c.raw, got, c.want)
		}
	}
}

func TestKindForStatus(t *testing.T) {
	cases := []struct {
		status int
		want   string
	}{
		{401, KindAuth}, {403, KindAuth},
		{429, KindRateLimit},
		{500, KindServer}, {503, KindServer},
		{400, KindBadRequest}, {404, KindBadRequest}, {422, KindBadRequest},
	}
	for _, c := range cases {
		if got := kindForStatus(c.status); got != c.want {
			t.Errorf("kindForStatus(%d) = %s, want %s", c.status, got, c.want)
		}
	}
}

func TestStreamUnsupported(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   bool
	}{
		{400, `{"error":{"message":"This model does not support stream mode"}}`, true},
		{422, `stream is unsupported for this model`, true},
		{501, `streaming disabled`, true},
		{400, `stream field must be boolean`, false}, // 提到 stream 但非不支持
		{400, `model not support`, false},            // 不支持但没提 stream
		{401, `stream not supported`, false},         // 状态码不符
		{500, `stream unsupported`, false},
	}
	for _, c := range cases {
		if got := streamUnsupported(c.status, c.body); got != c.want {
			t.Errorf("streamUnsupported(%d, %q) = %v, want %v", c.status, c.body, got, c.want)
		}
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"2", 2 * time.Second},
		{"0", 0},
		{"-3", 0},
		{"", 0},
		{"junk", 0},
		{now.Add(90 * time.Second).Format(http.TimeFormat), 90 * time.Second},
		{now.Add(-time.Hour).Format(http.TimeFormat), 0},
	}
	for _, c := range cases {
		if got := parseRetryAfter(c.in, now); got != c.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestRequestID(t *testing.T) {
	h := http.Header{}
	h.Set("Request-Id", "fallback")
	h.Set("X-Request-Id", "primary")
	if got := requestID(h); got != "primary" {
		t.Errorf("requestID prefer X-Request-Id, got %q", got)
	}
	h.Del("X-Request-Id")
	if got := requestID(h); got != "fallback" {
		t.Errorf("requestID fallback, got %q", got)
	}
	if got := requestID(http.Header{}); got != "" {
		t.Errorf("requestID empty, got %q", got)
	}
}

func TestAPIErrorFromBodyP4(t *testing.T) {
	h := http.Header{}
	h.Set("X-Request-Id", "req-42")
	h.Set("Retry-After", "3")

	e := apiErrorFromBody(400, h, fixture(t, "err_stream_unsupported.json"), testKey)
	if e.Kind != KindStreamUnsupported || e.Code != "stream_not_supported" {
		t.Errorf("P3 classify: kind=%s code=%s", e.Kind, e.Code)
	}
	if e.RequestID != "req-42" || e.RetryAfter != 3*time.Second {
		t.Errorf("P4 retain: rid=%q retryAfter=%v", e.RequestID, e.RetryAfter)
	}

	e = apiErrorFromBody(503, h, fixture(t, "json_error200.json"), testKey)
	if e.Kind != KindServer || e.Code != "503" {
		t.Errorf("numeric code: kind=%s code=%s", e.Kind, e.Code)
	}

	body := []byte(`{"error":{"message":"key ` + testKey + ` rejected"}}`)
	e = apiErrorFromBody(401, h, body, testKey)
	if strings.Contains(e.Error(), testKey) {
		t.Error("P4/红线3: key leaked in error")
	}
	if !strings.Contains(e.Message, "[REDACTED]") {
		t.Errorf("P4 redaction: %q", e.Message)
	}
}
