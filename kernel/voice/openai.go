package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"strings"
	"time"
)

// defaultTimeout 默认 HTTP 超时（语音文件较大，放宽）。
const defaultTimeout = 120 * time.Second

// maxRespBytes 响应体上限。
const maxRespBytes = 32 << 20

// OpenAIClient OpenAI 兼容语音客户端（STT /audio/transcriptions，TTS /audio/speech）。
type OpenAIClient struct {
	BaseURL string
	HTTP    *http.Client
	apiKey  string
}

// NewOpenAIClient 建客户端。baseURL 形如 https://host/v1。
func NewOpenAIClient(baseURL, apiKey string) *OpenAIClient {
	return &OpenAIClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: defaultTimeout},
		apiKey:  apiKey,
	}
}

// Name 实现 SttProvider / TtsProvider。
func (c *OpenAIClient) Name() string { return "openai-voice" }

// Transcribe 实现 SttProvider：multipart 上传音频到 /audio/transcriptions。
func (c *OpenAIClient) Transcribe(ctx context.Context, in AudioInput) (Transcript, error) {
	if len(in.Data) == 0 {
		return Transcript{}, ErrEmptyAudio
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		return Transcript{}, &APIError{Kind: KindProtocol, Message: "empty base url"}
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fn := in.Filename
	if fn == "" {
		fn = "audio.webm"
	}
	fw, err := mw.CreateFormFile("file", fn)
	if err != nil {
		return Transcript{}, &APIError{Kind: KindProtocol, Message: err.Error()}
	}
	if _, err := fw.Write(in.Data); err != nil {
		return Transcript{}, &APIError{Kind: KindProtocol, Message: err.Error()}
	}
	_ = mw.WriteField("model", "whisper-1")
	_ = mw.WriteField("response_format", "json")
	if in.Language != "" {
		_ = mw.WriteField("language", in.Language)
	}
	if err := mw.Close(); err != nil {
		return Transcript{}, &APIError{Kind: KindProtocol, Message: err.Error()}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/audio/transcriptions", &buf)
	if err != nil {
		return Transcript{}, &APIError{Kind: KindProtocol, Message: err.Error()}
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Transcript{}, transportErr(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Transcript{}, classifyStatus(resp.StatusCode, string(body))
	}
	var w struct {
		Text     string `json:"text"`
		Language string `json:"language,omitempty"`
	}
	if err := json.Unmarshal(body, &w); err != nil {
		return Transcript{}, &APIError{Kind: KindProtocol, Message: "transcription not JSON: " + err.Error()}
	}
	return Transcript{Text: w.Text, Language: w.Language}, nil
}

// Synthesize 实现 TtsProvider：POST /audio/speech，返回音频字节。
func (c *OpenAIClient) Synthesize(ctx context.Context, text string, opt TtsOptions) (AudioOutput, error) {
	if strings.TrimSpace(text) == "" {
		return AudioOutput{}, &APIError{Kind: KindEmpty, Message: "empty text"}
	}
	model := opt.Model
	if model == "" {
		model = "tts-1"
	}
	voice := opt.Voice
	if voice == "" {
		voice = "alloy"
	}
	format := opt.Format
	if format == "" {
		format = "mp3"
	}
	body := map[string]any{"model": model, "input": text, "voice": voice, "response_format": format}
	if opt.Speed > 0 {
		body["speed"] = opt.Speed
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/audio/speech", bytes.NewReader(raw))
	if err != nil {
		return AudioOutput{}, &APIError{Kind: KindProtocol, Message: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return AudioOutput{}, transportErr(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AudioOutput{}, classifyStatus(resp.StatusCode, string(data))
	}
	mt := resp.Header.Get("Content-Type")
	if mt == "" {
		mt = mimeForFormat(format)
	}
	return AudioOutput{Data: data, MIMEType: mt}, nil
}

// mimeForFormat 格式→MIME。
func mimeForFormat(f string) string {
	switch f {
	case "mp3":
		return "audio/mpeg"
	case "opus":
		return "audio/ogg"
	case "aac":
		return "audio/aac"
	case "flac":
		return "audio/flac"
	case "wav":
		return "audio/wav"
	case "pcm":
		return "audio/L16"
	default:
		return "application/octet-stream"
	}
}

// transportErr 传输错误归类。
func transportErr(err error) *APIError {
	kind := KindNetwork
	var ne net.Error
	switch {
	case errors.Is(err, context.Canceled):
		kind = KindNetwork
	case errors.Is(err, context.DeadlineExceeded):
		kind = KindTimeout
	case errors.As(err, &ne) && ne.Timeout():
		kind = KindTimeout
	}
	return &APIError{Kind: kind, Message: err.Error()}
}

// classifyStatus HTTP 状态归类。
func classifyStatus(status int, body string) *APIError {
	var k ErrorKind
	switch {
	case status == 401 || status == 403:
		k = KindAuth
	case status == 429:
		k = KindRateLimit
	default:
		k = KindHTTP
	}
	if len(body) > 200 {
		body = body[:200]
	}
	return &APIError{Kind: k, Status: status, Message: body}
}
