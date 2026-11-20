/* vector: int8 量化向量检索（T-M3-01，批准优化#2：v1=int8+暴力检索，接口留 HNSW 升级位）
 * 纯 C11，无第三方依赖。余弦相似度（int8 点积 + 预存模长），暴力 top-k。
 * aarch64 走 NEON 内核，其余架构标量。错误路径返回 0/NULL，不崩溃、不打印。
 * 句柄语义：create/deserialize 返回堆对象，必须 sr_vec_index_free 释放（Go 侧配 SetFinalizer+Close）。
 */
#ifndef SR_VECTOR_H
#define SR_VECTOR_H

#include <stddef.h>
#include <stdint.h>

#if defined(__GNUC__)
#define SR_API __attribute__((visibility("default")))
#else
#define SR_API
#endif

#ifdef __cplusplus
extern "C" {
#endif

typedef struct SrVecIndex SrVecIndex; /* 不透明句柄 */

/* 建索引。dim>0。失败返回 NULL。 */
SR_API SrVecIndex *sr_vec_index_create(int dim);

/* 释放索引（含内部向量表）。NULL 安全。 */
SR_API void sr_vec_index_free(SrVecIndex *idx);

/* 当前向量条数；NULL 返回 0。 */
SR_API int sr_vec_index_count(const SrVecIndex *idx);

/* 插入一条。id 由调用方保证唯一语义（本层不查重）。vec 长度须为 dim。
 * 成功 0；失败（NULL/维度不符/内存不足）-1。 */
SR_API int sr_vec_index_insert(SrVecIndex *idx, int64_t id, const int8_t *vec);

/* 检索 top-k（余弦降序）。out_ids/out_scores 长度至少 k。
 * 返回实际写入条数（0..k；空索引 0）。q 长度须为 dim。 */
SR_API int sr_vec_index_search(const SrVecIndex *idx, const int8_t *q, int k,
                               int64_t *out_ids, float *out_scores);

/* 序列化到新分配缓冲区（*out_buf 由内部 malloc，调用方用 sr_free 释放）。
 * 格式：magic "SRV1" | dim(i32le) | count(i32le) | 逐条{id(i64le), norm(f64le), data(dim i8)}。
 * 成功 0；失败 -1。 */
SR_API int sr_vec_index_serialize(const SrVecIndex *idx, uint8_t **out_buf, size_t *out_len);

/* 从序列化缓冲区重建索引。失败返回 NULL。 */
SR_API SrVecIndex *sr_vec_index_deserialize(const uint8_t *buf, size_t len);

/* 释放本库 malloc 的内存（serialize 输出等）。NULL 安全。 */
SR_API void sr_free(void *p);

#ifdef __cplusplus
}
#endif

#endif /* SR_VECTOR_H */
