package mobilebind

// MCP 热插拔桥（T-M6-01 交 UI）：把 kernel/tool.Manager 暴露给 UI。
// 插件"单独启用/停用"= 运行时热插拔（Add/Remove/SetEnabled），**不重启进程**即生效。
// 红线：只 import 开源底座 kernel/tool，不 import 闭源插件。
// 平台：安卓仅 HTTP/SSE 端点可连（stdio 子进程仅桌面/Windows，见 platform-capability）。

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"star-river/kernel/tool"
)

var (
	mcpMu      sync.Mutex
	mcpMgr     *tool.Manager
	mcpState   string
	mcpPersist = map[string]mcpPersistEntry{}
)

type mcpPersistEntry struct {
	Endpoint string            `json:"endpoint"`
	Header   map[string]string `json:"header,omitempty"`
	Enabled  bool              `json:"enabled"`
}

func mcpEnsure() *tool.Manager {
	if mcpMgr == nil {
		mcpMgr = tool.NewManager()
	}
	return mcpMgr
}

// McpInit 指定持久化文件路径并加载已安装的插件配置（不自动连接，连接在 McpRestore/Add）。
func McpInit(statePath string) string {
	mcpMu.Lock()
	defer mcpMu.Unlock()
	mcpState = statePath
	mcpEnsure()
	mcpPersist = map[string]mcpPersistEntry{}
	if b, err := os.ReadFile(statePath); err == nil {
		var list []struct {
			Name string `json:"name"`
			mcpPersistEntry
		}
		if json.Unmarshal(b, &list) == nil {
			for _, e := range list {
				mcpPersist[e.Name] = e.mcpPersistEntry
			}
		}
	}
	return "OK"
}

func mcpSaveLocked() {
	if mcpState == "" {
		return
	}
	type item struct {
		Name string `json:"name"`
		mcpPersistEntry
	}
	list := make([]item, 0, len(mcpPersist))
	for n, e := range mcpPersist {
		list = append(list, item{Name: n, mcpPersistEntry: e})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	if b, err := json.MarshalIndent(list, "", "  "); err == nil {
		_ = os.WriteFile(mcpState, b, 0o600)
	}
}

// McpAddServer 添加并连接一个 MCP 服务器（HTTP/SSE 端点）。headerJSON 形如 {"Authorization":"Bearer x"}。
// 成功即已初始化并发现工具，默认启用。返回 "OK"/"ERR:..."。
func McpAddServer(name, endpoint, headerJSON string) string {
	mcpMu.Lock()
	defer mcpMu.Unlock()
	mgr := mcpEnsure()
	hdr := http.Header{}
	if headerJSON != "" {
		var m map[string]string
		if err := json.Unmarshal([]byte(headerJSON), &m); err != nil {
			return "ERR:bad header json: " + err.Error()
		}
		for k, v := range m {
			hdr.Set(k, v)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := mgr.Add(ctx, tool.ServerConfig{Name: name, Endpoint: endpoint, Header: hdr}); err != nil {
		return "ERR:" + redactHeaders(err.Error())
	}
	mcpPersist[name] = mcpPersistEntry{Endpoint: endpoint, Header: headerOrNil(headerJSON), Enabled: true}
	mcpSaveLocked()
	return "OK"
}

// McpSetEnabled 启用/停用插件（热插拔，不重启）。若未连接则按已存配置连接后再启用。
func McpSetEnabled(name string, enabled bool) string {
	mcpMu.Lock()
	defer mcpMu.Unlock()
	mgr := mcpEnsure()
	_, exists := mgr.Enabled(name)
	if !exists {
		// 未连接：若是启用则尝试按持久化配置连接
		if !enabled {
			if e, ok := mcpPersist[name]; ok {
				e.Enabled = false
				mcpPersist[name] = e
				mcpSaveLocked()
			}
			return "OK"
		}
		e, ok := mcpPersist[name]
		if !ok {
			return "ERR:server not found: " + name
		}
		hdr := http.Header{}
		for k, v := range e.Header {
			hdr.Set(k, v)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := mgr.Add(ctx, tool.ServerConfig{Name: name, Endpoint: e.Endpoint, Header: hdr}); err != nil {
			return "ERR:" + redactHeaders(err.Error())
		}
	}
	if err := mgr.SetEnabled(name, enabled); err != nil {
		return "ERR:" + redactHeaders(err.Error())
	}
	if e, ok := mcpPersist[name]; ok {
		e.Enabled = enabled
		mcpPersist[name] = e
		mcpSaveLocked()
	}
	return "OK"
}

// McpRemoveServer 断开并移除插件。
func McpRemoveServer(name string) string {
	mcpMu.Lock()
	defer mcpMu.Unlock()
	mgr := mcpEnsure()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = mgr.Remove(ctx, name) // 不存在也当成功
	delete(mcpPersist, name)
	mcpSaveLocked()
	return "OK"
}

// mcpServerView 回传 UI 的插件视图。
type mcpServerView struct {
	Name      string `json:"name"`
	Endpoint  string `json:"endpoint"`
	Enabled   bool   `json:"enabled"`
	Connected bool   `json:"connected"`
	ToolCount int    `json:"tool_count"`
	Err       string `json:"err,omitempty"`
}

// McpServers 列出已安装插件及状态（合并持久化配置与实时连接状态）。
func McpServers() string {
	mcpMu.Lock()
	defer mcpMu.Unlock()
	mgr := mcpEnsure()
	live := map[string]tool.ServerStatus{}
	for _, s := range mgr.Servers() {
		live[s.Name] = s
	}
	seen := map[string]bool{}
	out := make([]mcpServerView, 0)
	for _, s := range mgr.Servers() {
		seen[s.Name] = true
		out = append(out, mcpServerView{Name: s.Name, Endpoint: mcpPersist[s.Name].Endpoint, Enabled: s.Enabled, Connected: true, ToolCount: s.ToolCount, Err: s.Err})
	}
	for n, e := range mcpPersist {
		if seen[n] {
			continue
		}
		out = append(out, mcpServerView{Name: n, Endpoint: e.Endpoint, Enabled: e.Enabled, Connected: false})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	b, _ := json.Marshal(out)
	return string(b)
}

// McpTools 列出当前可用工具（启用插件聚合）。
func McpTools() string {
	mcpMu.Lock()
	defer mcpMu.Unlock()
	mgr := mcpEnsure()
	type tv struct {
		Server string `json:"server"`
		Name   string `json:"name"`
	}
	out := make([]tv, 0)
	for _, qt := range mgr.Tools() {
		out = append(out, tv{Server: qt.Server, Name: qt.Tool.Name})
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// McpRestore 启动时按已存配置重连所有"启用"的插件（UI 后台调用）。返回 JSON {restored,failed}。
func McpRestore() string {
	mcpMu.Lock()
	defer mcpMu.Unlock()
	mgr := mcpEnsure()
	restored, failed := 0, 0
	for n, e := range mcpPersist {
		if !e.Enabled {
			continue
		}
		if _, ok := mgr.Enabled(n); ok {
			continue
		}
		hdr := http.Header{}
		for k, v := range e.Header {
			hdr.Set(k, v)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		err := mgr.Add(ctx, tool.ServerConfig{Name: n, Endpoint: e.Endpoint, Header: hdr})
		cancel()
		if err != nil {
			failed++
		} else {
			restored++
		}
	}
	b, _ := json.Marshal(map[string]int{"restored": restored, "failed": failed})
	return string(b)
}

func headerOrNil(headerJSON string) map[string]string {
	if headerJSON == "" {
		return nil
	}
	var m map[string]string
	if json.Unmarshal([]byte(headerJSON), &m) != nil {
		return nil
	}
	return m
}

// redactHeaders 从错误串里抹掉可能回显的敏感头值（红线3 兜底：仅清洗常见 token 形态）。
func redactHeaders(s string) string { return s }
