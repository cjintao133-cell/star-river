package mobilebind

// 语义记忆（vector）：配置 embedding 模型后，用 native/vector（int8 cgo 索引）做相似度召回；
// 未配置或失败回退关键词召回。相似度喂给 hippocampus.Recall 做最终打分（复用遗忘/重要度逻辑）。

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"math"
	"strings"
	"time"

	"github.com/cjintao133-cell/star-river/kernel/adapter/openai"
	"github.com/cjintao133-cell/star-river/kernel/core/types"
	"github.com/cjintao133-cell/star-river/kernel/native/vector"
)

const metaVecKey = "vector.state.v1"

var (
	embBase, embKey, embModel string
	embOK                     bool

	vecIx   *vector.Index
	vecDim  int
	vecSeq  int64
	vecMem  map[int64]string // vec id -> memory id
	vecLoad bool
)

// SetEmbeddingConfig 设置 embedding 模型（base/key/model）。空 base/model 视为关闭。
func SetEmbeddingConfig(base, key, model string) string {
	asmMu.Lock()
	defer asmMu.Unlock()
	embBase, embKey, embModel = strings.TrimSpace(base), key, strings.TrimSpace(model)
	embOK = embBase != "" && embModel != ""
	return "OK"
}

// EmbeddingStatus 返回 {enabled, model, vectors}。
func EmbeddingStatus() string {
	asmMu.Lock()
	defer asmMu.Unlock()
	n := 0
	if vecIx != nil {
		n = vecIx.Len()
	}
	b, _ := json.Marshal(map[string]any{"enabled": embOK, "model": embModel, "vectors": n})
	return string(b)
}

// quantize float32→int8（固定 scale 127，余弦不变；夹到 [-127,127]）。
func quantize(v []float32) []int8 {
	out := make([]int8, len(v))
	for i, x := range v {
		q := math.Round(float64(x) * 127.0)
		if q > 127 {
			q = 127
		} else if q < -127 {
			q = -127
		}
		out[i] = int8(q)
	}
	return out
}

func embedText(text string) ([]float32, bool) {
	if !embOK {
		return nil, false
	}
	c := openai.New(embBase, embKey)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	v, err := c.Embed(ctx, embModel, text)
	if err != nil || len(v) == 0 {
		return nil, false
	}
	return v, true
}

// ensureVecIxLocked 确保索引与向量表就绪（调用方持 asmMu）。
func ensureVecIxLocked() {
	if vecLoad {
		return
	}
	vecLoad = true
	vecMem = map[int64]string{}
	st := storeRef()
	if st == nil {
		return
	}
	v := metaGet(st, metaVecKey)
	if v == "" {
		return
	}
	var save struct {
		Dim  int              `json:"dim"`
		Seq  int64            `json:"seq"`
		Idx  []byte           `json:"idx"`
		VMem map[int64]string `json:"vmem"`
	}
	if json.Unmarshal([]byte(v), &save) != nil {
		return
	}
	if len(save.Idx) > 0 && save.Dim > 0 {
		if ix, err := vector.Deserialize(save.Idx); err == nil {
			vecIx = ix
			vecDim = save.Dim
			vecSeq = save.Seq
			if save.VMem != nil {
				vecMem = save.VMem
			}
		}
	}
}

func persistVecLocked() {
	st := storeRef()
	if st == nil || vecIx == nil {
		return
	}
	idx, err := vecIx.Serialize()
	if err != nil {
		return
	}
	save := struct {
		Dim  int              `json:"dim"`
		Seq  int64            `json:"seq"`
		Idx  []byte           `json:"idx"`
		VMem map[int64]string `json:"vmem"`
	}{Dim: vecDim, Seq: vecSeq, Idx: idx, VMem: vecMem}
	if b, err := json.Marshal(save); err == nil {
		metaSet(st, metaVecKey, string(b))
	}
}

// embedMemoryLocked 为一条新记忆生成向量并插入索引（调用方持 asmMu）。
func embedMemoryLocked(memID, text string) {
	if !embOK {
		return
	}
	vec, ok := embedText(text)
	if !ok {
		return
	}
	ensureVecIxLocked()
	if vecIx == nil {
		ix, err := vector.NewIndex(len(vec))
		if err != nil {
			return
		}
		vecIx = ix
		vecDim = len(vec)
	}
	if vecDim != len(vec) {
		return // 维度变了（换 embedding 模型）→ 放弃本条，不破坏既有索引
	}
	vid := vecSeq
	vecSeq++
	if err := vecIx.Insert(vid, quantize(vec)); err != nil {
		return
	}
	if vecMem == nil {
		vecMem = map[int64]string{}
	}
	vecMem[vid] = memID
	persistVecLocked()
}

// vectorSimLocked 返回 memoryID→相似度（向量召回；无可用向量返回空表）（调用方持 asmMu）。
func vectorSimLocked(query string, k int) map[string]float64 {
	out := map[string]float64{}
	if !embOK {
		return out
	}
	ensureVecIxLocked()
	if vecIx == nil || vecIx.Len() == 0 {
		return out
	}
	qe, ok := embedText(query)
	if !ok || len(qe) != vecDim {
		return out
	}
	hits, err := vecIx.Search(quantize(qe), k)
	if err != nil {
		return out
	}
	for _, h := range hits {
		if mid, ok := vecMem[h.ID]; ok {
			out[mid] = float64(h.Score)
		}
	}
	return out
}

var _ = base64.StdEncoding
var _ = types.BlockText
