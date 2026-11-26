package mobilebind

// 记忆的 SQLite 持久层 + 混合检索（向量 + FTS5 BM25 + RRF 融合）。
// 取代原先"塞进 app_meta 一个 JSON、硬限 500 条"的做法。

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// memDBInsert 写入/更新一条记忆到 memories 表 + FTS 索引（调用方持 asmMu）。
func memDBInsert(r memRecord) {
	st := storeRef()
	if st == nil {
		return
	}
	_, _ = st.Exec(context.Background(),
		`INSERT OR REPLACE INTO memories(id,kind,text,importance,emotion) VALUES(?,?,?,?,?)`,
		r.ID, r.Kind, r.Text, r.Imp, r.Emo)
	_, _ = st.Exec(context.Background(), `DELETE FROM memories_fts WHERE mem_id=?`, r.ID)
	_, _ = st.Exec(context.Background(), `INSERT INTO memories_fts(mem_id,text) VALUES(?,?)`, r.ID, r.Text)
}

// memDBDelete 删除一条记忆（含 FTS）。
func memDBDelete(id string) {
	st := storeRef()
	if st == nil {
		return
	}
	_, _ = st.Exec(context.Background(), `DELETE FROM memories WHERE id=?`, id)
	_, _ = st.Exec(context.Background(), `DELETE FROM memories_fts WHERE mem_id=?`, id)
}

// memDBCount 记忆总数。
func memDBCount() int {
	st := storeRef()
	if st == nil {
		return 0
	}
	var n int
	_ = st.DB().QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&n)
	return n
}

// memDBLoadAll 读出全部记忆（按创建时间正序），limit<=0 表示全部。
func memDBLoadAll(limit int) []memRecord {
	st := storeRef()
	if st == nil {
		return nil
	}
	q := `SELECT id,kind,text,importance,emotion FROM memories ORDER BY created_at ASC, id ASC`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := st.Query(context.Background(), q)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []memRecord
	for rows.Next() {
		var r memRecord
		if rows.Scan(&r.ID, &r.Kind, &r.Text, &r.Imp, &r.Emo) == nil {
			out = append(out, r)
		}
	}
	return out
}

// memFTSQuery 关键词召回：返回按 BM25 相关度排序的 mem_id 列表。
// 中文用 trigram，查询串>=3 字符才有效；用双引号包裹避免特殊字符爆错。
func memFTSQuery(q string, k int) []string {
	st := storeRef()
	if st == nil {
		return nil
	}
	q = strings.TrimSpace(q)
	if len([]rune(q)) < 3 {
		return nil
	}
	safe := "\"" + strings.ReplaceAll(q, "\"", " ") + "\""
	if k <= 0 {
		k = 30
	}
	rows, err := st.Query(context.Background(),
		`SELECT mem_id FROM memories_fts WHERE memories_fts MATCH ? ORDER BY bm25(memories_fts) LIMIT ?`, safe, k)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// rrfSim 用 RRF 把"向量排名"和"FTS 关键词排名"融合成一张 id->相似度 表。
// vecMap: id->向量相似度(已排序可选)；ftsIDs: 关键词召回的 id 有序列表。
func rrfSim(vecSorted []string, ftsIDs []string) map[string]float64 {
	const kk = 60.0
	m := map[string]float64{}
	for rank, id := range vecSorted {
		m[id] += 1.0 / (kk + float64(rank+1))
	}
	for rank, id := range ftsIDs {
		m[id] += 1.0 / (kk + float64(rank+1))
	}
	// 归一化到 [0,1]，便于与 hippocampus 的 recallScore 相乘
	var maxv float64
	for _, v := range m {
		if v > maxv {
			maxv = v
		}
	}
	if maxv > 0 {
		for id := range m {
			m[id] = m[id] / maxv
		}
	}
	return m
}

// vecSortedBySim 把 vectorSimLocked 的结果按分数降序转成 id 列表。
func vecSortedBySim(sims map[string]float64) []string {
	ids := make([]string, 0, len(sims))
	for id := range sims {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return sims[ids[i]] > sims[ids[j]] })
	return ids
}

// ---------------------------------------------------------------------------
// 知识库（RAG）：文档入库 -> 分块 -> FTS 索引；检索走 FTS + 向量(若已 embedding)。
// ---------------------------------------------------------------------------

// kbChunk 文本粗分块（按 ~500 字符 + 50 重叠，尽量在句读处切）。
func splitChunks(text string, size, overlap int) []string {
	if size <= 0 {
		size = 500
	}
	if overlap < 0 || overlap >= size {
		overlap = size / 5
	}
	r := []rune(strings.TrimSpace(text))
	if len(r) == 0 {
		return nil
	}
	var out []string
	for i := 0; i < len(r); i += size - overlap {
		end := i + size
		if end > len(r) {
			end = len(r)
		}
		out = append(out, string(r[i:end]))
		if end == len(r) {
			break
		}
	}
	return out
}

// kbIngest 入库一篇文档：写 kb_docs + 分块 + 分块 FTS。返回分块数。
func kbIngest(docID, title, source, text string) int {
	st := storeRef()
	if st == nil || strings.TrimSpace(text) == "" {
		return 0
	}
	now := nowMillis()
	_, _ = st.Exec(context.Background(), `INSERT OR REPLACE INTO kb_docs(id,title,source,created_at) VALUES(?,?,?,?)`, docID, title, source, now)
	// 清旧分块
	var old []string
	if rows, err := st.Query(context.Background(), `SELECT chunk_id FROM kb_chunks WHERE doc_id=?`, docID); err == nil {
		for rows.Next() {
			var c string
			if rows.Scan(&c) == nil {
				old = append(old, c)
			}
		}
		rows.Close()
	}
	for _, c := range old {
		_, _ = st.Exec(context.Background(), `DELETE FROM kb_chunks WHERE chunk_id=?`, c)
		_, _ = st.Exec(context.Background(), `DELETE FROM kb_chunks_fts WHERE chunk_id=?`, c)
	}
	chunks := splitChunks(text, 500, 100)
	for i, c := range chunks {
		cid := fmt.Sprintf("%s#%d", docID, i)
		_, _ = st.Exec(context.Background(), `INSERT OR REPLACE INTO kb_chunks(chunk_id,doc_id,ord,text) VALUES(?,?,?,?)`, cid, docID, i, c)
		_, _ = st.Exec(context.Background(), `INSERT INTO kb_chunks_fts(chunk_id,doc_id,text) VALUES(?,?,?)`, cid, docID, c)
	}
	return len(chunks)
}

// kbSearch 知识库关键词检索（FTS trigram）。返回 [{doc_id,chunk_id,text,score}]。
func kbSearch(q string, k int) []map[string]any {
	st := storeRef()
	if st == nil {
		return nil
	}
	q = strings.TrimSpace(q)
	if len([]rune(q)) < 3 {
		return nil
	}
	if k <= 0 {
		k = 6
	}
	safe := "\"" + strings.ReplaceAll(q, "\"", " ") + "\""
	rows, err := st.Query(context.Background(),
		`SELECT chunk_id,doc_id,text FROM kb_chunks_fts WHERE kb_chunks_fts MATCH ? ORDER BY bm25(kb_chunks_fts) LIMIT ?`, safe, k)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var cid, did, txt string
		if rows.Scan(&cid, &did, &txt) == nil {
			out = append(out, map[string]any{"chunk_id": cid, "doc_id": did, "text": txt})
		}
	}
	return out
}

// kbDocCount 知识库文档数。
func kbDocCount() int {
	st := storeRef()
	if st == nil {
		return 0
	}
	var n int
	_ = st.DB().QueryRow(`SELECT COUNT(*) FROM kb_docs`).Scan(&n)
	return n
}
