/* device_probe 正式单测（T-M0-08）。
 * 正常路径：直接链 device_probe.c。
 * 异常路径：Linux sysinfo 失败分支经 ld --wrap=sysinfo 故障注入（零新依赖），
 * 由 native/test-c.sh 单独编译运行；Android getauxval / Windows GlobalMemoryStatusEx
 * 失败分支 Linux 侧无法注入，DEFERRED 记录于 docs/m0-conclusions.md。
 */
#include <stdio.h>
#include <assert.h>
#include "../device_probe.h"

static int failures = 0;
#define CHECK(cond, name) do { \
    if (cond) { printf("[OK] %s\n", name); } \
    else { printf("[FAIL] %s\n", name); failures++; } \
} while (0)

#if defined(__linux__) && !defined(TEST_NO_WRAP)
#include <sys/sysinfo.h>
#include <errno.h>
extern int __real_sysinfo(struct sysinfo *info);
static int wrap_mode = 0; /* 0=透传 1=失败 2=mem_unit=0 */
int __wrap_sysinfo(struct sysinfo *info)
{
    if (wrap_mode == 1) { errno = EFAULT; return -1; }
    if (wrap_mode == 2) {
        int r = __real_sysinfo(info);
        if (r == 0) info->mem_unit = 0;
        return r;
    }
    return __real_sysinfo(info);
}
#endif

int main(void)
{
    /* 正常路径 */
    int n = sr_cpu_has_neon();
    int a = sr_cpu_has_avx2();
    CHECK(n == 0 || n == 1, "normal-neon-domain");
    CHECK(a == 0 || a == 1, "normal-avx2-domain");
#if defined(__x86_64__) || defined(__i386__)
    CHECK(n == 0, "normal-neon-zero-on-x86");
#else
    CHECK(a == 0, "normal-avx2-zero-on-non-x86");
#endif
    uint64_t m1 = sr_mem_available_bytes();
    uint64_t m2 = sr_mem_available_bytes();
    CHECK(m1 > 0, "normal-mem-positive");
    CHECK(m1 < (1ULL << 42), "normal-mem-plausible");
    CHECK(m2 > 0, "normal-mem-repeatable");

#if defined(__linux__) && !defined(TEST_NO_WRAP)
    /* 异常路径：sysinfo 返 -1（EFAULT）→ 必须返 0，不崩溃 */
    wrap_mode = 1;
    CHECK(sr_mem_available_bytes() == 0, "fault-sysinfo-minus1-returns-zero");
    /* 边界：mem_unit=0 → 乘法结果 0，无 UB */
    wrap_mode = 2;
    CHECK(sr_mem_available_bytes() == 0, "edge-mem-unit-zero");
    /* 恢复透传，确认无状态残留 */
    wrap_mode = 0;
    CHECK(sr_mem_available_bytes() > 0, "recover-after-fault");
#endif

    if (failures == 0) { puts("C-TESTS-GREEN"); return 0; }
    printf("C-TESTS-RED (%d failures)\n", failures);
    return 1;
}
