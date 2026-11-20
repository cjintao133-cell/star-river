#define _POSIX_C_SOURCE 199309L
/* 桌面基准（sanity）：N 条 dim 维 int8 向量，测暴力检索单次耗时。
 * 注意：桌面 x86 走标量内核，且非目标低端机——本基准只证明"能规模、不炸内存"，
 * 真实低端机 P99<80ms 基准 DEFERRED 至真机（T-M7 或设备实测）。 */
#include "../vector.h"

#include <stdio.h>
#include <stdlib.h>
#include <time.h>

static double now_ms(void) {
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    return (double)ts.tv_sec * 1000.0 + (double)ts.tv_nsec / 1e6;
}

int main(int argc, char **argv) {
    int N = (argc > 1) ? atoi(argv[1]) : 100000;
    int dim = (argc > 2) ? atoi(argv[2]) : 128;
    int k = 10;
    SrVecIndex *idx = sr_vec_index_create(dim);
    if (!idx) { printf("create failed\n"); return 1; }
    int8_t *v = (int8_t *)malloc((size_t)dim);
    srand(42);
    for (int i = 0; i < N; i++) {
        for (int d = 0; d < dim; d++) v[d] = (int8_t)((rand() % 255) - 127);
        /* 跳过全零（模长 0 无意义） */
        v[0] = (int8_t)(v[0] ? v[0] : 1);
        if (sr_vec_index_insert(idx, i, v) != 0) { printf("insert %d failed\n", i); return 1; }
    }
    free(v);
    int8_t q[512];
    if (dim > 512) { printf("dim too large for bench stack\n"); return 1; }
    for (int d = 0; d < dim; d++) q[d] = (int8_t)((rand() % 255) - 127);
    q[0] = q[0] ? q[0] : 1;

    int64_t ids[16];
    float sc[16];
    int reps = 50;
    /* 预热 */
    sr_vec_index_search(idx, q, k, ids, sc);
    double t0 = now_ms();
    for (int r = 0; r < reps; r++) {
        sr_vec_index_search(idx, q, k, ids, sc);
    }
    double t1 = now_ms();
    printf("N=%d dim=%d k=%d avg=%.3f ms/query (desktop scalar, %.1fs total)\n",
           N, dim, k, (t1 - t0) / reps, (t1 - t0) / 1000.0);
    printf("top1 id=%lld score=%.4f\n", (long long)ids[0], sc[0]);
    sr_vec_index_free(idx);
    return 0;
}
