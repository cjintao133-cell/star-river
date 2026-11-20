package secret

import "bytes"

// Scrub 从 b 中抹掉所有 secret 出现（红线 3 双保险）。
// 用于错误体/日志落盘前的最后一道清洗；空 secret 原样返回。
func Scrub(b []byte, secret string) []byte {
	if secret == "" || len(b) == 0 {
		return b
	}
	return bytes.ReplaceAll(b, []byte(secret), []byte("[REDACTED]"))
}

// ScrubString 字符串版抹除。用于日志行、错误串。
func ScrubString(s, secret string) string {
	if secret == "" || s == "" {
		return s
	}
	return string(bytes.ReplaceAll([]byte(s), []byte(secret), []byte("[REDACTED]")))
}

// SecretRef 持久化层应保存的引用形式：只是 id 字符串，不是 secret 本体。
// store 落库时字段类型用它，从类型层面杜绝把 secret 写进会话/配置表。
type SecretRef string

// ID 返回引用 id。
func (r SecretRef) ID() string { return string(r) }
