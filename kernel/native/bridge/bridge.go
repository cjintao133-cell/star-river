// Package bridge 是 device_probe C 静态库的 cgo 薄封装（T-M0-07）。
// 指针纪律：仅传值调用（无指针入参、无句柄、无堆分配），C 不持有 Go 指针；
// 因无句柄，SetFinalizer 无适用对象——评审记录见 docs/m0-conclusions.md。
// 链接纪律：先 make -f native/Makefile all 产出 build/native/<goos>-<goarch>/libdevice_probe.a。
package bridge

/*
#cgo CFLAGS: -I${SRCDIR}/../../../native/device_probe
#cgo linux,amd64 LDFLAGS: ${SRCDIR}/../../../build/native/linux-amd64/libdevice_probe.a
#cgo android,arm64 LDFLAGS: ${SRCDIR}/../../../build/native/android-arm64/libdevice_probe.a
#cgo windows,amd64 LDFLAGS: ${SRCDIR}/../../../build/native/windows-amd64/libdevice_probe.a
#include "device_probe.h"
*/
import "C"

// CPUFeatures 返回 CPU 特性检测结果（NEON/ASIMD 与 AVX2）。
func CPUFeatures() (neon, avx2 bool) {
return C.sr_cpu_has_neon() != 0, C.sr_cpu_has_avx2() != 0
}

// AvailMemBytes 返回可用物理内存字节数（近似值）；底层失败时返回 0。
func AvailMemBytes() uint64 {
return uint64(C.sr_mem_available_bytes())
}
