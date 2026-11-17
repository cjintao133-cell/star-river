#include "device_probe.h"

#if defined(_WIN32)
#include <windows.h>
#elif defined(__linux__)
#include <sys/sysinfo.h>
#if defined(__aarch64__)
#include <sys/auxv.h>
#include <asm/hwcap.h>
#ifndef HWCAP_ASIMD
#define HWCAP_ASIMD (1 << 1)
#endif
#endif
#endif

SR_API int sr_cpu_has_neon(void)
{
#if defined(__linux__) && defined(__aarch64__)
    return (getauxval(AT_HWCAP) & HWCAP_ASIMD) ? 1 : 0;
#else
    return 0;
#endif
}

SR_API int sr_cpu_has_avx2(void)
{
#if defined(__x86_64__) || defined(__i386__)
    return __builtin_cpu_supports("avx2") ? 1 : 0;
#else
    return 0;
#endif
}

SR_API uint64_t sr_mem_available_bytes(void)
{
#if defined(_WIN32)
    MEMORYSTATUSEX st;
    st.dwLength = sizeof(st);
    if (!GlobalMemoryStatusEx(&st)) {
        return 0;
    }
    return (uint64_t)st.ullAvailPhys;
#elif defined(__linux__)
    struct sysinfo si;
    if (sysinfo(&si) != 0) {
        return 0;
    }
    return (uint64_t)si.freeram * (uint64_t)si.mem_unit;
#else
    return 0;
#endif
}
