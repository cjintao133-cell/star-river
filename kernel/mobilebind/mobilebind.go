// Package mobilebind 是 Go 内核对 Android AAR 的正式出口（gomobile bind 目标包）。
// 桥接层纪律：只做薄门面，函数签名保持稳定；内核内部逻辑不得写在这里。
// 类型限制：gomobile bind 仅支持 string/int/float/bool/[]byte 等有限类型，
// 新增导出函数前先核对 gomobile 类型表（docs/m0-conclusions.md）。
package mobilebind

import (
"fmt"

"star-river/kernel/native/bridge"
)

// kernelVersion 是内核版本标识。发布链任务（T-M7-06）改为构建期注入前，此处为唯一来源。
const kernelVersion = "0.0.1-m0"

// Version 返回内核版本字符串，供 UI 侧验证 AAR 链路连通。
func Version() string {
return "star-river-kernel/" + kernelVersion
}

// Echo 原样返回入参，供 UI 侧验证字符串双向过桥无损。
func Echo(s string) string {
return s
}

// DeviceProfile 返回设备探测 JSON（T-M0-07）：版本 + CPU 特性 + 可用内存。
// 手工拼接固定四字段，全为受控值，无注入面；不引入序列化依赖。
func DeviceProfile() string {
neon, avx2 := bridge.CPUFeatures()
return fmt.Sprintf(`{"version":%q,"neon":%t,"avx2":%t,"avail_mem_bytes":%d}`,
Version(), neon, avx2, bridge.AvailMemBytes())
}
