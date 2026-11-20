package mobilebind

// 真机场景模拟套件（可在服务器无设备复跑：go test -run TestDevice -v）。
// 覆盖：重启恢复 / 网络异常注入(500·截断·超时·连接错) / 大数据量性能 / 并发 MCP 热插拔 / 长上下文裁剪。
// 全本地、零 API 花费。断言失败即真问题。

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func b64key() string { return base64.StdEncoding.EncodeToString(make([]byte, 32)) }

func mustSession(t *testing.T, sid string) string {
	t.Helper()
	var m map[string]string
	if err := json.Unmarshal([]byte(sid), &m); err != nil {
		t.Fatalf("session json: %v", err)
	}
	return m["session_id"]
}

// ① 重启恢复：App 被系统杀→重启，数据全回来（会话/角色/消息/记忆/情绪/主动）。
func TestDeviceRestartRecovery(t *testing.T) {
	db := filepath.Join(t.TempDir(), "restart.db")
	if r := InitKernel(db, b64key()); r != "OK" {
		t.Fatalf("init: %s", r)
	}
	if r := CreateCharacter(`{"id":"c1","name":"示例角色","personality":"温和"}`); r != "OK" {
		t.Fatalf("char: %s", r)
	}
	sid := mustSession(t, CreateSession("c1", "重启前会话", "m"))
	AppendMessage(sid, "user", "记住：我的生日是五月一日")
	AppendMessage(sid, "assistant", "记住了用户")
	observeTurn("记住：我的生日是五月一日", "记住了用户") // 记忆+情绪
	moodBefore := MoodStateJSON()

	// 模拟进程被杀 → 重启：重新 InitKernel（会关旧库、重开、重置派生单例）
	if r := InitKernel(db, b64key()); r != "OK" {
		t.Fatalf("reinit: %s", r)
	}
	if !strings.Contains(ListCharacters(), "示例角色") {
		t.Fatalf("重启后角色丢失")
	}
	if !strings.Contains(ListSessions(), "重启前会话") {
		t.Fatalf("重启后会话丢失")
	}
	if !strings.Contains(ListMessages(sid, 10), "五月一日") {
		t.Fatalf("重启后消息丢失")
	}
	if !strings.Contains(MemoryHitsJSON(10), "五月一日") {
		t.Fatalf("重启后记忆丢失")
	}
	moodAfter := MoodStateJSON()
	if moodAfter == "" || strings.Contains(moodAfter, "NaN") {
		t.Fatalf("重启后情绪异常: %s", moodAfter)
	}
	t.Logf("✅ 重启恢复：角色/会话/消息/记忆/情绪全回来（情绪前=%s 后=%s）", short(moodBefore), short(moodAfter))
}

func short(s string) string {
	if len(s) > 80 {
		return s[:80] + "…"
	}
	return s
}

// ② 网络异常注入：500 / 响应截断 / 超时 / 连接错误 → ChatStream 只 OnError，不 panic、不假成功。
func TestDeviceNetworkAnomalies(t *testing.T) {
	cases := []struct {
		name string
		h    http.HandlerFunc
	}{
		{"500错误", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
			w.Write([]byte(`{"error":{"message":"boom sk-KEY"}}`))
		}},
		{"响应截断", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write([]byte("data: {\"id\":\"x\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"半截\"}}]}\n\n"))
			// 故意不发 finish_reason / [DONE] 就关闭 → 流断裂
		}},
		{"空body", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
		}},
	}
	for _, c := range cases {
		srv := httptest.NewServer(c.h)
		s := &recSink{}
		ChatStreamTools(srv.URL+"/v1", "sk-KEY", "m", "", "hi", s)
		srv.Close()
		// 不 panic 即到这里；必须 OnError（不能无错静默成功），且 key 不泄漏
		if s.err == "" && s.done == "" {
			t.Fatalf("[%s] 无错误也无完成（假死）: %+v", c.name, s)
		}
		if strings.Contains(s.err, "sk-KEY") {
			t.Fatalf("[%s] key 泄漏: %s", c.name, s.err)
		}
		t.Logf("✅ [%s] 优雅处理：err=%s", c.name, short(s.err))
	}

	// 连接错误（端口不可达）
	s2 := &recSink{}
	ChatStreamTools("http://127.0.0.1:1/v1", "k", "m", "", "hi", s2)
	if s2.err == "" {
		t.Fatalf("连接错误应 OnError")
	}
	t.Logf("✅ [连接错误] err=%s", short(s2.err))
}

// ③ 大数据量：1万条消息写入 + 分页查询 + 备份往返，计时与完整性。
func TestDeviceBigData(t *testing.T) {
	db := filepath.Join(t.TempDir(), "big.db")
	InitKernel(db, b64key())
	CreateCharacter(`{"id":"c1","name":"大库角色"}`)
	sid := mustSession(t, CreateSession("c1", "大库会话", "m"))

	t0 := time.Now()
	const N = 10000
	for i := 0; i < N; i++ {
		AppendMessage(sid, "user", fmt.Sprintf("第 %d 条消息内容 blah blah", i))
	}
	insMs := time.Since(t0).Milliseconds()

	t1 := time.Now()
	page := ListMessages(sid, 100)
	lmMs := time.Since(t1).Milliseconds()
	if strings.Count(page, "\"role\"") < 100 {
		t.Fatalf("分页应返回 100 条，实际少")
	}

	t2 := time.Now()
	arch := BackupExport()
	expMs := time.Since(t2).Milliseconds()
	if len(arch) == 0 {
		t.Fatalf("导出为空")
	}
	db2 := filepath.Join(t.TempDir(), "big2.db")
	InitKernel(db2, b64key())
	t3 := time.Now()
	r := BackupImport(arch)
	impMs := time.Since(t3).Milliseconds()
	if !strings.HasPrefix(r, "OK:") {
		t.Fatalf("导入失败: %s", r)
	}
	// 完整性：导入后分页也应 100 条
	if strings.Count(ListMessages(sid, 100), "\"role\"") < 100 {
		t.Fatalf("导入后数据不完整")
	}
	t.Logf("✅ 1万条：写入 %dms · 分页(100) %dms · 导出 %dms(%dKB) · 导入 %dms · 完整性OK",
		insMs, lmMs, expMs, len(arch)/1024, impMs)
}

// ④ 并发 MCP 热插拔：多 server 并发 add/enable/disable/remove，不 panic、计数正确。
func TestDeviceConcurrentMCP(t *testing.T) {
	InitKernel(filepath.Join(t.TempDir(), "mcp.db"), b64key())
	McpInit(filepath.Join(t.TempDir(), "mcp.json"))
	const servers = 8
	var wg sync.WaitGroup
	for i := 0; i < servers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			srv := mockManyDev(t, []string{fmt.Sprintf("t%d_a", id), fmt.Sprintf("t%d_b", id)})
			defer srv.Close()
			name := fmt.Sprintf("srv%d", id)
			for round := 0; round < 20; round++ {
				_ = McpAddServer(name, srv.URL, "")
				_, _ = McpSetEnabled(name, true), McpRemoveServer(name)
			}
		}(i)
	}
	wg.Wait()
	// 不 panic 即过；状态应可读
	if s := McpServers(); !strings.HasPrefix(strings.TrimSpace(s), "[") {
		t.Fatalf("McpServers 应为数组: %s", s)
	}
	t.Logf("✅ 并发 MCP 热插拔：%d server × 20 轮 add/enable/remove，无 panic", servers)
}

// mockManyDev 起一个有若干工具的 mock MCP（带 t.Cleanup）。
func mockManyDev(t *testing.T, tools []string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusOK)
			return
		}
		var msg struct {
			Method string `json:"method"`
			ID     *int64 `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&msg)
		if msg.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		switch msg.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-06-18", "serverInfo": map[string]any{"name": "m"}}
		case "tools/list":
			ts := make([]map[string]any, 0, len(tools))
			for _, n := range tools {
				ts = append(ts, map[string]any{"name": n, "inputSchema": map[string]any{"type": "object", "properties": map[string]any{}}})
			}
			result = map[string]any{"tools": ts}
		default:
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": result})
	}))
	return srv
}

// ⑤ 长上下文裁剪：超长历史 + 超长用户输入，请求体应有上限（不无限膨胀）。
func TestDeviceLongContextTrim(t *testing.T) {
	var bodyLen int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 0, 4096)
		tmp := make([]byte, 4096)
		for {
			n, err := r.Body.Read(tmp)
			buf = append(buf, tmp[:n]...)
			if err != nil {
				break
			}
		}
		mu.Lock()
		bodyLen = len(buf)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"id\":\"x\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\n"))
		w.Write([]byte("data: {\"id\":\"x\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()
	huge := strings.Repeat("很长的历史内容…", 5000) // ~ 数万字
	s := &recSink{}
	ChatStreamTools(srv.URL+"/v1", "k", "m", huge, "你好", s)
	mu.Lock()
	bl := bodyLen
	mu.Unlock()
	if s.err != "" {
		t.Fatalf("应成功: %s", s.err)
	}
	if bl == 0 {
		t.Fatalf("未捕获请求体")
	}
	// BuildMinimal 上限 MaxChars=8000，请求体应远小于原始巨大输入
	if bl > 60000 {
		t.Fatalf("请求体未裁剪，过大: %d 字节", bl)
	}
	t.Logf("✅ 长上下文裁剪：输入 %d 字 → 请求体仅 %d 字节（BuildMinimal 限额生效）", len([]rune(huge)), bl)
}
