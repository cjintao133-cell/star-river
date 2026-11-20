package voice

import (
	"context"
	"errors"
	"fmt"
)

// 错误分类。
type ErrorKind string

const (
	KindNetwork   ErrorKind = "network"
	KindTimeout   ErrorKind = "timeout"
	KindAuth      ErrorKind = "auth"
	KindRateLimit ErrorKind = "rate_limit"
	KindHTTP      ErrorKind = "http"
	KindProtocol  ErrorKind = "protocol"
	KindEmpty     ErrorKind = "empty"
)

// ErrEmptyAudio 空音频输入。
var ErrEmptyAudio = errors.New("voice: empty audio")

// APIError 统一错误。
type APIError struct {
	Kind    ErrorKind `json:"kind"`
	Status  int       `json:"status,omitempty"`
	Message string    `json:"message"`
}

// Error 实现 error。
func (e *APIError) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("voice: %s (http %d): %s", e.Kind, e.Status, e.Message)
	}
	return fmt.Sprintf("voice: %s: %s", e.Kind, e.Message)
}

// AudioInput 一段待识别音频。
type AudioInput struct {
	Data     []byte
	Filename string // 上传文件名（带扩展名更稳，如 audio.webm）
	MIMEType string
	Language string // 可选：期望语言（ISO-639-1，如 zh）
}

// Transcript 识别结果。
type Transcript struct {
	Text     string
	Language string
}

// TtsOptions 合成选项。
type TtsOptions struct {
	Model  string
	Voice  string
	Format string  // 输出格式：mp3/opus/aac/flac/wav/pcm
	Speed  float64 // 语速倍率（0=默认）
}

// AudioOutput 合成出的音频。
type AudioOutput struct {
	Data     []byte
	MIMEType string
}

// SttProvider 语音识别后端。
type SttProvider interface {
	Name() string
	Transcribe(ctx context.Context, in AudioInput) (Transcript, error)
}

// TtsProvider 语音合成后端。
type TtsProvider interface {
	Name() string
	Synthesize(ctx context.Context, text string, opt TtsOptions) (AudioOutput, error)
}

// Preprocessor 音频前处理（重采样/降噪/声道）。默认 NoopPreprocessor。
type Preprocessor interface {
	Process(ctx context.Context, in AudioInput) (AudioInput, error)
}

// NoopPreprocessor 直通（不做任何处理）。
type NoopPreprocessor struct{}

// Process 实现 Preprocessor。
func (NoopPreprocessor) Process(_ context.Context, in AudioInput) (AudioInput, error) { return in, nil }
