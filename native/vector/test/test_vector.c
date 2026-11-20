#include "../vector.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static int fails = 0;
#define CHECK(cond, msg) do { \
    if (!(cond)) { printf("[FAIL] %s\n", msg); fails++; } \
    else { printf("[OK] %s\n", msg); } \
} while (0)

int main(void) {
    const int dim = 4;
    SrVecIndex *idx = sr_vec_index_create(dim);
    CHECK(idx != NULL, "create");
    CHECK(sr_vec_index_count(idx) == 0, "empty count=0");

    int8_t v1[4] = {1, 0, 0, 0};
    int8_t v2[4] = {0, 1, 0, 0};
    int8_t v3[4] = {1, 1, 0, 0};
    CHECK(sr_vec_index_insert(idx, 10, v1) == 0, "insert v1");
    CHECK(sr_vec_index_insert(idx, 20, v2) == 0, "insert v2");
    CHECK(sr_vec_index_insert(idx, 30, v3) == 0, "insert v3");
    CHECK(sr_vec_index_count(idx) == 3, "count=3");

    int8_t q[4] = {1, 0, 0, 0};
    int64_t ids[3] = {0, 0, 0};
    float scores[3] = {0, 0, 0};
    int n = sr_vec_index_search(idx, q, 3, ids, scores);
    CHECK(n == 3, "search returns 3");
    CHECK(ids[0] == 10, "top1 = v1 (cos 1.0)");
    CHECK(ids[1] == 30, "top2 = v3 (cos ~0.707)");
    CHECK(ids[2] == 20, "top3 = v2 (cos 0)");
    CHECK(scores[0] > scores[1] && scores[1] > scores[2], "scores descending");

    /* k 小于条数：只回前 k。 */
    int64_t ids1[1];
    float sc1[1];
    int n1 = sr_vec_index_search(idx, q, 1, ids1, sc1);
    CHECK(n1 == 1 && ids1[0] == 10, "top-k k=1");

    /* 序列化往返：重建后 count 与检索一致。 */
    uint8_t *buf = NULL;
    size_t len = 0;
    CHECK(sr_vec_index_serialize(idx, &buf, &len) == 0, "serialize");
    CHECK(len == (size_t)(12 + 3 * (8 + 8 + dim)), "serialize length");
    SrVecIndex *idx2 = sr_vec_index_deserialize(buf, len);
    CHECK(idx2 != NULL, "deserialize");
    CHECK(sr_vec_index_count(idx2) == 3, "deserialize count");
    int64_t ids2[3];
    float sc2[3];
    int n2 = sr_vec_index_search(idx2, q, 3, ids2, sc2);
    CHECK(n2 == 3 && ids2[0] == 10 && ids2[1] == 30 && ids2[2] == 20, "deserialize search matches");

    /* 坏的序列化数据：magic 错 / 太短 → NULL。 */
    CHECK(sr_vec_index_deserialize((const uint8_t *)"XXXX", 4) == NULL, "deserialize bad magic");
    CHECK(sr_vec_index_deserialize(buf, 8) == NULL, "deserialize too short");

    sr_free(buf);
    sr_vec_index_free(idx2);

    /* 空索引检索 → 0。 */
    SrVecIndex *empty = sr_vec_index_create(dim);
    int64_t eid[2];
    float esc[2];
    CHECK(sr_vec_index_search(empty, q, 2, eid, esc) == 0, "empty search=0");
    sr_vec_index_free(empty);

    /* NULL 安全。 */
    sr_vec_index_free(NULL);
    sr_free(NULL);
    CHECK(sr_vec_index_count(NULL) == 0, "count(NULL)=0");
    CHECK(sr_vec_index_insert(NULL, 1, v1) == -1, "insert(NULL)=-1");
    CHECK(sr_vec_index_create(0) == NULL, "create dim=0 NULL");

    sr_vec_index_free(idx);

    if (fails == 0) { printf("VEC-TESTS-GREEN\n"); return 0; }
    printf("VEC-TESTS-RED (%d fails)\n", fails);
    return 1;
}
