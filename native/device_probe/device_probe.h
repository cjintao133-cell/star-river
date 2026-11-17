/* device_probe: CPU 特性检测 + 可用内存读取（T-M0-06）
 * 纯 C11，无第三方依赖。错误路径一律返回 0，不崩溃、不打印。
 */
#ifndef SR_DEVICE_PROBE_H
#define SR_DEVICE_PROBE_H

#include <stdint.h>

#if defined(__GNUC__)
#define SR_API __attribute__((visibility("default")))
#else
#define SR_API
#endif

#ifdef __cplusplus
extern "C" {
#endif

/* NEON/ASIMD：1=支持 0=不支持。linux/android aarch64 经 AT_HWCAP 检测；其余架构恒 0。 */
SR_API int sr_cpu_has_neon(void);

/* AVX2：1=支持 0=不支持。x86 经 CPUID（__builtin_cpu_supports）检测；其余架构恒 0。 */
SR_API int sr_cpu_has_avx2(void);

/* 可用物理内存字节数（近似值）。失败返回 0。
 * linux/android: sysinfo.freeram（不含可回收缓存，偏保守）；windows: ullAvailPhys。 */
SR_API uint64_t sr_mem_available_bytes(void);

#ifdef __cplusplus
}
#endif

#endif /* SR_DEVICE_PROBE_H */
