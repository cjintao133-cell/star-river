// 桌面 c-shared 内核导出（T-M5-03 全功能接入）：把 mobilebind 的会话/角色/单次聊天
// 暴露给 JNI 层，供桌面三栏 UI 使用。桥层零逻辑，全部转调 mobilebind（与 Android 同源）。
//
// 内存契约同 main.go：返回 C.CString（malloc），JNI 层负责 free；入参由调用方持有，
// C.GoString 拷贝，不释放入参。
//
// 字符串编码：C.CString 产出 UTF-8 字节 + NUL。JNI 层用 jbyteArray 承载（非 NewStringUTF），
// 避免 Modified UTF-8 对 emoji/补充平面的破坏（见 docs/m0-conclusions.md T-M0-05 红线）。
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"strings"

	"star-river/kernel/mobilebind"
)

// SrInitKernel 打开本地数据库（cipherKeyBase64 为 32 字节字段加密钥 base64）。返回 "OK"/"ERR:..."。
//
//export SrInitKernel
func SrInitKernel(dbPath, cipherKeyBase64 *C.char) *C.char {
	return C.CString(mobilebind.InitKernel(C.GoString(dbPath), C.GoString(cipherKeyBase64)))
}

// SrListSessions 列出会话（JSON 数组）。
//
//export SrListSessions
func SrListSessions() *C.char {
	return C.CString(mobilebind.ListSessions())
}

// SrCreateSession 新建会话，返回 JSON {"session_id":...}。
//
//export SrCreateSession
func SrCreateSession(characterID, title, model *C.char) *C.char {
	return C.CString(mobilebind.CreateSession(C.GoString(characterID), C.GoString(title), C.GoString(model)))
}

// SrListMessages 列出某会话消息（JSON 数组，时间正序）。
//
//export SrListMessages
func SrListMessages(sessionID *C.char, limit C.int) *C.char {
	return C.CString(mobilebind.ListMessages(C.GoString(sessionID), int(limit)))
}

// SrAppendMessage 追加消息。返回 "OK"/"ERR:..."。
//
//export SrAppendMessage
func SrAppendMessage(sessionID, role, text *C.char) *C.char {
	return C.CString(mobilebind.AppendMessage(C.GoString(sessionID), C.GoString(role), C.GoString(text)))
}

// SrDeleteSession 删除会话。返回 "OK"/"ERR:..."。
//
//export SrDeleteSession
func SrDeleteSession(sessionID *C.char) *C.char {
	return C.CString(mobilebind.DeleteSession(C.GoString(sessionID)))
}

// SrListCharacters 列出角色（JSON 数组）。
//
//export SrListCharacters
func SrListCharacters() *C.char {
	return C.CString(mobilebind.ListCharacters())
}

// SrCreateCharacter 新建角色（入参 JSON）。返回 "OK"/"ERR:..."。
//
//export SrCreateCharacter
func SrCreateCharacter(jsonStr *C.char) *C.char {
	return C.CString(mobilebind.CreateCharacter(C.GoString(jsonStr)))
}

// SrDeleteCharacter 删除角色。返回 "OK"/"ERR:..."。
//
//export SrDeleteCharacter
func SrDeleteCharacter(id *C.char) *C.char {
	return C.CString(mobilebind.DeleteCharacter(C.GoString(id)))
}

// accumSink 累积流式增量，供单次同步聊天使用。
type accumSink struct {
	sb  strings.Builder
	err string
}

func (a *accumSink) OnDelta(t string)   { a.sb.WriteString(t) }
func (a *accumSink) OnDone(m string)    {}
func (a *accumSink) OnError(msg string) { a.err = msg }

// SrChatOnce 同步执行一次对话并返回完整文本（内部走流式累积）。失败返回 "ERR:..."。
// 便捷：桌面无回调需求时直接取整段；需流式请改用回调桥。
//
//export SrChatOnce
func SrChatOnce(baseURL, apiKey, model, system, user *C.char) *C.char {
	s := &accumSink{}
	mobilebind.ChatStream(C.GoString(baseURL), C.GoString(apiKey), C.GoString(model), C.GoString(system), C.GoString(user), s)
	if s.err != "" {
		return C.CString("ERR:" + s.err)
	}
	return C.CString(s.sb.String())
}
