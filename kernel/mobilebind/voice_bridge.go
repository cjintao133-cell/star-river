package mobilebind

// 语音桥（T-新增）：STT/TTS 暴露给 UI。薄门面，base64 音频进出，错误前缀 "ERR:"。

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"star-river/kernel/voice"
)

// SttTranscribe 语音识别：audioBase64 为音频字节 base64。返回 JSON {"text","language"}；
// 失败返回 "ERR:<脱敏>"。
func SttTranscribe(baseURL, apiKey, audioBase64, filename, lang string) string {
	data, err := base64.StdEncoding.DecodeString(audioBase64)
	if err != nil {
		return "ERR:bad base64: " + redactKey(err.Error(), apiKey)
	}
	if len(data) == 0 {
		return "ERR:empty audio"
	}
	c := voice.NewOpenAIClient(baseURL, apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	tr, err := c.Transcribe(ctx, voice.AudioInput{Data: data, Filename: filename, Language: lang})
	if err != nil {
		return "ERR:" + redactKey(err.Error(), apiKey)
	}
	b, err := json.Marshal(map[string]string{"text": tr.Text, "language": tr.Language})
	if err != nil {
		return "ERR:" + redactKey(err.Error(), apiKey)
	}
	return string(b)
}

// TtsSynthesize 语音合成：返回 JSON {"audio_base64","mime"}；失败返回 "ERR:<脱敏>"。
func TtsSynthesize(baseURL, apiKey, text, ttsVoice, format string) string {
	c := voice.NewOpenAIClient(baseURL, apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	out, err := c.Synthesize(ctx, text, voice.TtsOptions{Voice: ttsVoice, Format: format})
	if err != nil {
		return "ERR:" + redactKey(err.Error(), apiKey)
	}
	b, err := json.Marshal(map[string]string{
		"audio_base64": base64.StdEncoding.EncodeToString(out.Data),
		"mime":         out.MIMEType,
	})
	if err != nil {
		return "ERR:" + redactKey(err.Error(), apiKey)
	}
	return string(b)
}
