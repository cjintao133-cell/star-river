// Package main 是内核 c-shared 桌面出口（T-M0-05 spike）。
// 产物：linux .so / windows .dll + 头文件；JNI 桥见 jnibridge.c（REVIEW-NEEDED 挂起，不改）
// 与 jnibridge_probe.c（T-M0-07 新增）。
// 桥接纪律同 mobilebind：薄门面，签名稳定；逻辑复用 kernel/mobilebind。
// 注意：#cgo 的 JDK 头路径为唯一构建机实测路径（docs/env-linux.md），M5 参数化。
package main

/*
#cgo linux CFLAGS: -I/usr/lib/jvm/java-17-openjdk-amd64/include -I/usr/lib/jvm/java-17-openjdk-amd64/include/linux
#cgo windows CFLAGS: -I/usr/lib/jvm/java-17-openjdk-amd64/include -I${SRCDIR}/wininc
#include <stdlib.h>
*/
import "C"

import "github.com/cjintao133-cell/star-river/kernel/mobilebind"

// 内存契约：SrVersion/SrEcho/SrDeviceProfile 返回 C.CString（malloc），调用方负责 free；
// 入参 char* 由调用方持有，Go 侧 C.GoString 拷贝，不释放入参。

//export SrVersion
func SrVersion() *C.char {
	return C.CString(mobilebind.Version())
}

//export SrEcho
func SrEcho(s *C.char) *C.char {
	return C.CString(mobilebind.Echo(C.GoString(s)))
}

//export SrDeviceProfile
func SrDeviceProfile() *C.char {
	return C.CString(mobilebind.DeviceProfile())
}

func main() {}
