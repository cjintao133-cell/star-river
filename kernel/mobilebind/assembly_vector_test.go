package mobilebind

// 语义记忆：mock /embeddings → 记忆写入向量索引 → 召回走向量路径（status.vectors>0 且能召回）。

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func mockEmbeddings() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		// 简单按关键词给不同向量（dim=4）
		vec := []float32{0, 0, 0, 1}
		switch {
		case strings.Contains(req.Input, "猫"), strings.Contains(req.Input, "喜欢"):
			vec = []float32{1, 0, 0, 0}
		case strings.Contains(req.Input, "狗"):
			vec = []float32{0, 1, 0, 0}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"embedding": vec}}})
	}))
}

func TestAssemblyVectorSemantic(t *testing.T) {
	InitKernel(filepath.Join(t.TempDir(), "vec.db"), base64.StdEncoding.EncodeToString(make([]byte, 32)))
	srv := mockEmbeddings()
	defer srv.Close()
	if r := SetEmbeddingConfig(srv.URL, "k", "text-embedding-mock"); r != "OK" {
		t.Fatalf("set embedding: %s", r)
	}
	if !strings.Contains(EmbeddingStatus(), "\"enabled\":true") {
		t.Fatalf("应启用 embedding: %s", EmbeddingStatus())
	}

	observeTurn("我特别喜欢猫", "好呀")
	observeTurn("我养了一只狗", "哈")

	if st := EmbeddingStatus(); !strings.Contains(st, "\"vectors\":2") {
		t.Fatalf("应有 2 条向量: %s", st)
	}
	// 召回"猫"（非逐字关键词也能命中同向量记忆）
	hits := MemoryHitsJSON(10)
	if !strings.Contains(hits, "猫") {
		t.Fatalf("应召回与猫相关的记忆: %s", hits)
	}
}

func TestAssemblyVectorDisabledFallback(t *testing.T) {
	InitKernel(filepath.Join(t.TempDir(), "vec2.db"), base64.StdEncoding.EncodeToString(make([]byte, 32)))
	SetEmbeddingConfig("", "", "") // 关闭
	observeTurn("关键：生日五月一日", "记住了")
	if !strings.Contains(EmbeddingStatus(), "\"enabled\":false") {
		t.Fatalf("应关闭")
	}
	// 关键词召回仍工作
	if !strings.Contains(MemoryHitsJSON(10), "生日") {
		t.Fatalf("关闭向量后关键词召回应工作")
	}
}
