// Package voice 是语音能力（STT 语音识别 / TTS 语音合成）的适配层。
//
// 设计：协议无关的 SttProvider / TtsProvider 接口，多后端可插拔（OpenAI 兼容 Whisper/audio 端点；
// 后续可接 Groq、本地 whisper.cpp、云厂商等）。BYOK：端点/key 用户自带。
//
// 音频前处理（重采样/降噪/声道转换）经 Preprocessor 接口注入，默认 Noop（直通）；
// 真实现走 native 音频模块（当前 native 仅有 device_probe/vector，音频前处理为预留位）。
//
// 依赖方向（红线 2）：本包为内核底座，只 import 标准库；不 import 任何能力插件包。
package voice
