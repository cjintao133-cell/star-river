#include <stdio.h>
#include <assert.h>
#include "../device_probe.h"
int main(void)
{
    int n = sr_cpu_has_neon();
    int a = sr_cpu_has_avx2();
    uint64_t m = sr_mem_available_bytes();
    assert(n == 0 || n == 1);
    assert(a == 0 || a == 1);
    assert(m > 0); /* Linux 构建机必有可用内存；失败路径断言属 T-M0-08 */
    printf("neon=%d avx2=%d avail_bytes=%llu\n", n, a, (unsigned long long)m);
    puts("SMOKE-OK");
    return 0;
}
