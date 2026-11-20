package mobilebind

// 应用装配（composition root）：把伴侣插件 hippocampus(记忆) / endocrine(情绪) 接进 App。
// 边界：只有入口层 mobilebind 可 import 插件（红线2 只约束开源区 core/adapter/tool/orchestrator…）。
// 职责：真实记忆编码/召回、情绪状态演化、把情绪+记忆注入聊天 system、为开发者三面板供真数据。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"star-river/kernel/endocrine"
	"star-river/kernel/hippocampus"
	"star-river/kernel/store"
	"star-river/kernel/tool"
)

const (
	metaMoodKey = "endocrine.snapshot.v1"
	metaMemKey  = "hippocampus.records.v1"
	memCap      = 500
)

var (
	asmMu     sync.Mutex
	asmMood   *endocrine.Engine
	asmMem    *hippocampus.Engine
	asmRecs   []memRecord
	asmLoaded bool
)

type memRecord struct {
	ID   string  `json:"id"`
	Text string  `json:"text"`
	Imp  float64 `json:"imp"`
	Emo  float64 `json:"emo"`
}

func metaGet(st *store.Store, key string) string {
	var v string
	if err := st.DB().QueryRow(`SELECT value FROM app_meta WHERE key=?`, key).Scan(&v); err != nil {
		return ""
	}
	return v
}

func metaSet(st *store.Store, key, val string) {
	if st == nil {
		return
	}
	_, _ = st.Exec(context.Background(), `INSERT OR REPLACE INTO app_meta(key,value) VALUES(?,?)`, key, val)
}

func storeRef() *store.Store {
	kbMu.Lock()
	defer kbMu.Unlock()
	return kbStore
}

// ensureAssembly 惰性建情绪/记忆引擎并从 store 恢复（调用方持 asmMu）。
func ensureAssembly() {
	if asmLoaded {
		return
	}
	asmLoaded = true
	asmMood = endocrine.New(endocrine.DefaultRegistry(), nil)
	asmMem = hippocampus.New(hippocampus.Options{})
	st := storeRef()
	if st == nil {
		return
	}
	if v := metaGet(st, metaMoodKey); v != "" {
		var snap endocrine.Snapshot
		if json.Unmarshal([]byte(v), &snap) == nil {
			asmMood.Restore(snap)
		}
	}
	if v := metaGet(st, metaMemKey); v != "" {
		var recs []memRecord
		if json.Unmarshal([]byte(v), &recs) == nil {
			asmRecs = recs
			for _, r := range recs {
				asmMem.Encode(r.ID, r.Text, r.Imp, r.Emo)
			}
		}
	}
}

func persistMoodLocked() {
	st := storeRef()
	if st == nil {
		return
	}
	if b, err := json.Marshal(asmMood.Snapshot()); err == nil {
		metaSet(st, metaMoodKey, string(b))
	}
}

func persistMemLocked() {
	st := storeRef()
	if st == nil {
		return
	}
	if b, err := json.Marshal(asmRecs); err == nil {
		metaSet(st, metaMemKey, string(b))
	}
}

// resetAssembly 供 InitKernel 重开库时清空单例。
func resetAssembly() {
	asmMu.Lock()
	asmMood, asmMem, asmRecs, asmLoaded = nil, nil, nil, false
	asmMu.Unlock()
}

// —— 供 UI / 面板 ——

// MoodStateJSON 真实情绪状态（默认注册表五维）。
func MoodStateJSON() string {
	asmMu.Lock()
	ensureAssembly()
	st := asmMood.State()
	asmMu.Unlock()
	b, _ := json.Marshal(st)
	return string(b)
}

// MemoryHitsJSON 记忆列表（供面板；按强度/时间返回最近若干条）。
func MemoryHitsJSON(limit int) string {
	asmMu.Lock()
	ensureAssembly()
	if limit <= 0 {
		limit = 10
	}
	hits := asmMem.RecallFromStore(func(h hippocampus.Memory) float64 { return 0.5 }) // 中性相似度，按 R/重要度排序
	asmMu.Unlock()
	out := make([]map[string]any, 0, limit)
	for i, h := range hits {
		if i >= limit {
			break
		}
		out = append(out, map[string]any{
			"id": h.Memory.ID, "text": h.Memory.Text, "score": h.Score,
			"retention": h.Retention, "importance": h.Memory.Importance,
		})
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// —— 情绪 + 记忆注入聊天 ——

// assembleSystem 在基础人设后追加"当前情绪语气"与"相关记忆"。
func assembleSystem(base, userMsg string) string {
	asmMu.Lock()
	ensureAssembly()
	state := asmMood.State()
	var memTexts []string
	for _, h := range recallLocked(userMsg, 5) {
		memTexts = append(memTexts, h.Memory.Text)
	}
	asmMu.Unlock()

	var sb strings.Builder
	sb.WriteString(base)
	directive := endocrine.DefaultCoupler(endocrine.DefaultRegistry()).Style(state).PromptDirective()
	if strings.TrimSpace(directive) != "" {
		sb.WriteString("\n")
		sb.WriteString(directive)
	}
	if len(memTexts) > 0 {
		sb.WriteString("\n【可能相关的记忆】")
		for _, mt := range memTexts {
			sb.WriteString("\n- ")
			sb.WriteString(mt)
		}
	}
	return sb.String()
}

func recallLocked(userMsg string, limit int) []hippocampus.Scored {
	terms := keywords(userMsg)
	hits := asmMem.RecallFromStore(func(m hippocampus.Memory) float64 {
		hay := strings.ToLower(m.Text)
		if len(terms) == 0 {
			return 0.0
		}
		hit := 0
		for _, t := range terms {
			if t != "" && strings.Contains(hay, t) {
				hit++
			}
		}
		return float64(hit) / float64(len(terms))
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// observeTurn 记录一轮对话：编码记忆 + 情绪演化。异步外调用应放后台线程。
func observeTurn(userText, assistantText string) {
	asmMu.Lock()
	defer asmMu.Unlock()
	ensureAssembly()
	// 记忆编码：用户话语为一条记忆（重要度按长度/情绪线索粗估）。
	if u := strings.TrimSpace(userText); u != "" {
		imp := 0.45
		emo := 0.35
		low := strings.ToLower(u)
		for _, w := range []string{"喜欢", "爱", "讨厌", "难过", "开心", "生日", "答应", "承诺", "重要", "记住"} {
			if strings.Contains(u, w) {
				imp, emo = 0.8, 0.7
				break
			}
		}
		_ = low
		id := fmt.Sprintf("mem-%d", time.Now().UnixNano())
		_ = asmMem.Encode(id, truncate(u, 300), imp, emo)
		asmRecs = append(asmRecs, memRecord{ID: id, Text: truncate(u, 300), Imp: imp, Emo: emo})
		if len(asmRecs) > memCap {
			asmRecs = asmRecs[len(asmRecs)-memCap:]
		}
	}
	// 情绪演化：有正常回复则亲密/好奇微增、疲劳微增；空回复视为不顺。
	if strings.TrimSpace(assistantText) != "" {
		asmMood.Interact(map[string]float64{
			endocrine.KeyIntimacy:  0.02,
			endocrine.KeyCuriosity: 0.01,
			endocrine.KeyValence:   0.03,
			endocrine.KeyFatigue:   0.01,
		})
	}
	asmMood.Tick()
	persistMoodLocked()
	persistMemLocked()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// —— 面板适配（realMoodMap / realMemoryHits 返回真实引擎数据）——

func realMoodMap() map[string]float64 {
	asmMu.Lock()
	ensureAssembly()
	st := asmMood.State()
	asmMu.Unlock()
	return st
}

// realMemoryHits 返回真实记忆命中（[]tool.MemoryHit 语义，用于 devPanels）。
func realMemoryHits(query string, limit int) []tool.MemoryHit {
	asmMu.Lock()
	ensureAssembly()
	var scored []hippocampus.Scored
	if strings.TrimSpace(query) == "" {
		scored = asmMem.RecallFromStore(func(m hippocampus.Memory) float64 { return 0.5 })
	} else {
		scored = recallLocked(query, limit)
	}
	asmMu.Unlock()
	out := make([]tool.MemoryHit, 0, limit)
	for i, h := range scored {
		if i >= limit {
			break
		}
		out = append(out, tool.MemoryHit{ID: h.Memory.ID, Text: h.Memory.Text, Score: h.Score})
	}
	return out
}
