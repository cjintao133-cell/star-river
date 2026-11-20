#include "vector.h"

#include <math.h>
#include <stdlib.h>
#include <string.h>

#if defined(__ARM_NEON)
#include <arm_neon.h>
#endif

/* 仅支持 64 位 size_t（设备目标 arm64/amd64）。32 位容量计算会 size_t 回绕，
 * 编译期直接拒绝，避免把"靠构建惯例排除 32 位"留成隐患（astra 终扫条件性 P0）。 */
#if !defined(SIZE_MAX) || SIZE_MAX < 0xFFFFFFFFFFFFFFFFULL
#error "vector.c: requires 64-bit size_t (arm64/amd64 only)"
#endif

/* int8 点积用 int32 累加：全范围乘积 128*128=16384，安全维度上限 2^31/16384 = 131071。 */
#define SR_VEC_MAX_DIM 131071

/* ---- 点积内核（int8 × int8 → int32 累加） ---- */

#if !defined(__ARM_NEON)
static int32_t dot_int8_scalar(const int8_t *a, const int8_t *b, int n) {
    int32_t s = 0;
    for (int i = 0; i < n; i++) {
        s += (int32_t)a[i] * (int32_t)b[i];
    }
    return s;
}
#endif

#if defined(__ARM_NEON)
/* 可移植 NEON：int8→int16 拓宽, 16×int16 相乘, vpadalq 累加进 int32x4。 */
static int32_t dot_int8_neon(const int8_t *a, const int8_t *b, int n) {
    int32x4_t acc = vdupq_n_s32(0);
    int i = 0;
    for (; i + 16 <= n; i += 16) {
        int8x16_t va = vld1q_s8(a + i);
        int8x16_t vb = vld1q_s8(b + i);
        int16x8_t lo = vmulq_s16(vmovl_s8(vget_low_s8(va)), vmovl_s8(vget_low_s8(vb)));
        int16x8_t hi = vmulq_s16(vmovl_s8(vget_high_s8(va)), vmovl_s8(vget_high_s8(vb)));
        acc = vpadalq_s16(acc, lo);
        acc = vpadalq_s16(acc, hi);
    }
    int32_t s = vaddvq_s32(acc);
    for (; i < n; i++) {
        s += (int32_t)a[i] * (int32_t)b[i];
    }
    return s;
}
#endif

static int32_t dot_int8(const int8_t *a, const int8_t *b, int n) {
#if defined(__ARM_NEON)
    return dot_int8_neon(a, b, n);
#else
    return dot_int8_scalar(a, b, n);
#endif
}

/* ---- 索引结构 ---- */

struct SrVecIndex {
    int dim;
    int count;
    int cap;
    int64_t *ids;
    int8_t *data;    /* count*dim 连续 int8 */
    double *norms;   /* 每条向量模长（预计算，余弦用） */
};

static double vec_norm(const int8_t *v, int dim) {
    int64_t sq = 0;
    for (int i = 0; i < dim; i++) {
        int64_t x = v[i];
        sq += x * x;
    }
    return sqrt((double)sq);
}

static int ensure_cap(SrVecIndex *idx, int need) {
    if (need <= idx->cap) {
        return 0;
    }
    int ncap = idx->cap ? idx->cap * 2 : 64;
    while (ncap < need) {
        ncap *= 2;
    }
    int64_t *nids = (int64_t *)realloc(idx->ids, (size_t)ncap * sizeof(int64_t));
    if (!nids) return -1;
    idx->ids = nids;
    int8_t *ndata = (int8_t *)realloc(idx->data, (size_t)ncap * (size_t)idx->dim);
    if (!ndata) return -1;
    idx->data = ndata;
    double *nnorms = (double *)realloc(idx->norms, (size_t)ncap * sizeof(double));
    if (!nnorms) return -1;
    idx->norms = nnorms;
    idx->cap = ncap;
    return 0;
}

SrVecIndex *sr_vec_index_create(int dim) {
    if (dim <= 0 || dim > SR_VEC_MAX_DIM) return NULL;
    SrVecIndex *idx = (SrVecIndex *)calloc(1, sizeof(SrVecIndex));
    if (!idx) return NULL;
    idx->dim = dim;
    return idx;
}

void sr_vec_index_free(SrVecIndex *idx) {
    if (!idx) return;
    free(idx->ids);
    free(idx->data);
    free(idx->norms);
    free(idx);
}

int sr_vec_index_count(const SrVecIndex *idx) {
    return idx ? idx->count : 0;
}

int sr_vec_index_insert(SrVecIndex *idx, int64_t id, const int8_t *vec) {
    if (!idx || !vec) return -1;
    if (ensure_cap(idx, idx->count + 1) != 0) return -1;
    idx->ids[idx->count] = id;
    memcpy(idx->data + (size_t)idx->count * (size_t)idx->dim, vec, (size_t)idx->dim);
    idx->norms[idx->count] = vec_norm(vec, idx->dim);
    idx->count++;
    return 0;
}

/* top-k：维护大小 k 的降序结果（插入排序，k 通常很小）。 */
int sr_vec_index_search(const SrVecIndex *idx, const int8_t *q, int k,
                        int64_t *out_ids, float *out_scores) {
    if (!idx || !q || !out_ids || !out_scores || k <= 0) return 0;
    if (idx->count == 0) return 0;
    double qn = vec_norm(q, idx->dim);
    if (qn <= 0.0) qn = 1e-12;
    int filled = 0;
    for (int i = 0; i < idx->count; i++) {
        int32_t d = dot_int8(q, idx->data + (size_t)i * (size_t)idx->dim, idx->dim);
        double vn = idx->norms[i];
        if (vn <= 0.0) vn = 1e-12;
        float score = (float)((double)d / (qn * vn));
        int64_t id = idx->ids[i];
        /* 插入到降序位置。 */
        int pos = filled < k ? filled : k; /* 满则尝试替换最后 */
        /* 找插入点 */
        int j = (filled < k) ? filled : k - 1;
        if (filled == k && score <= out_scores[k - 1]) {
            continue;
        }
        if (filled < k) filled++;
        while (j > 0 && out_scores[j - 1] < score) {
            out_scores[j] = out_scores[j - 1];
            out_ids[j] = out_ids[j - 1];
            j--;
        }
        out_scores[j] = score;
        out_ids[j] = id;
        (void)pos;
    }
    return filled;
}

/* ---- 序列化 ---- */

static void put_i32(uint8_t *p, int32_t v) {
    uint32_t u = (uint32_t)v;
    p[0] = (uint8_t)(u & 0xff);
    p[1] = (uint8_t)((u >> 8) & 0xff);
    p[2] = (uint8_t)((u >> 16) & 0xff);
    p[3] = (uint8_t)((u >> 24) & 0xff);
}
static void put_i64(uint8_t *p, int64_t v) {
    uint64_t u = (uint64_t)v;
    for (int i = 0; i < 8; i++) p[i] = (uint8_t)((u >> (8 * i)) & 0xff);
}
static void put_f64(uint8_t *p, double v) {
    uint64_t u;
    memcpy(&u, &v, 8);
    for (int i = 0; i < 8; i++) p[i] = (uint8_t)((u >> (8 * i)) & 0xff);
}
static int32_t get_i32(const uint8_t *p) {
    uint32_t u = (uint32_t)p[0] | ((uint32_t)p[1] << 8) | ((uint32_t)p[2] << 16) | ((uint32_t)p[3] << 24);
    return (int32_t)u;
}
static int64_t get_i64(const uint8_t *p) {
    uint64_t u = 0;
    for (int i = 0; i < 8; i++) u |= ((uint64_t)p[i]) << (8 * i);
    return (int64_t)u;
}
static double get_f64(const uint8_t *p) {
    uint64_t u = 0;
    for (int i = 0; i < 8; i++) u |= ((uint64_t)p[i]) << (8 * i);
    double v;
    memcpy(&v, &u, 8);
    return v;
}

int sr_vec_index_serialize(const SrVecIndex *idx, uint8_t **out_buf, size_t *out_len) {
    if (!idx || !out_buf || !out_len) return -1;
    size_t per = 8 + 8 + (size_t)idx->dim; /* id + norm + data */
    size_t total = 4 + 4 + 4 + (size_t)idx->count * per; /* magic+dim+count */
    uint8_t *buf = (uint8_t *)malloc(total);
    if (!buf) return -1;
    memcpy(buf, "SRV1", 4);
    put_i32(buf + 4, idx->dim);
    put_i32(buf + 8, idx->count);
    size_t off = 12;
    for (int i = 0; i < idx->count; i++) {
        put_i64(buf + off, idx->ids[i]);
        off += 8;
        put_f64(buf + off, idx->norms[i]);
        off += 8;
        memcpy(buf + off, idx->data + (size_t)i * (size_t)idx->dim, (size_t)idx->dim);
        off += (size_t)idx->dim;
    }
    *out_buf = buf;
    *out_len = total;
    return 0;
}

SrVecIndex *sr_vec_index_deserialize(const uint8_t *buf, size_t len) {
    if (!buf || len < 12) return NULL;
    if (memcmp(buf, "SRV1", 4) != 0) return NULL;
    int dim = get_i32(buf + 4);
    int count = get_i32(buf + 8);
    if (dim <= 0 || dim > SR_VEC_MAX_DIM || count < 0) return NULL;
    size_t per = 8 + 8 + (size_t)dim;
    if (len < 12 + (size_t)count * per) return NULL;
    SrVecIndex *idx = sr_vec_index_create(dim);
    if (!idx) return NULL;
    if (count > 0 && ensure_cap(idx, count) != 0) {
        sr_vec_index_free(idx);
        return NULL;
    }
    size_t off = 12;
    for (int i = 0; i < count; i++) {
        idx->ids[i] = get_i64(buf + off);
        off += 8;
        idx->norms[i] = get_f64(buf + off);
        off += 8;
        memcpy(idx->data + (size_t)i * (size_t)dim, buf + off, (size_t)dim);
        off += (size_t)dim;
    }
    idx->count = count;
    return idx;
}

void sr_free(void *p) {
    free(p);
}
