package mobilebind

// 连接器/集成桥：注册连接器、派生工具、构造 OAuth 授权 URL、令牌引用持久化（app_meta）。

import (
	"encoding/json"
	"sync"

	"github.com/cjintao133-cell/star-river/kernel/connectors"
)

const metaConnectorsKey = "connectors.registry.v1"

var connMu sync.Mutex

// loadConnectors 从 app_meta 读出并构建注册表（调用方持 connMu）。
func loadConnectors() *connectors.Registry {
	r := connectors.NewRegistry()
	if st := storeRef(); st != nil {
		if v := metaGet(st, metaConnectorsKey); v != "" {
			var defs []connectors.ConnectorDef
			if json.Unmarshal([]byte(v), &defs) == nil {
				for _, d := range defs {
					_ = r.Register(d)
				}
			}
		}
	}
	return r
}

func persistConnectors(r *connectors.Registry) {
	st := storeRef()
	if st == nil {
		return
	}
	if b, err := json.Marshal(r.List()); err == nil {
		metaSet(st, metaConnectorsKey, string(b))
	}
}

// ConnectorsAdd 注册一个连接器（JSON: ConnectorDef），持久化。
func ConnectorsAdd(defJSON string) string {
	var d connectors.ConnectorDef
	if err := json.Unmarshal([]byte(defJSON), &d); err != nil {
		return "ERR:bad json"
	}
	connMu.Lock()
	defer connMu.Unlock()
	r := loadConnectors()
	if err := r.Register(d); err != nil {
		return "ERR:" + err.Error()
	}
	persistConnectors(r)
	return "OK"
}

// ConnectorsList 列出连接器定义 JSON 数组。
func ConnectorsList() string {
	connMu.Lock()
	defer connMu.Unlock()
	b, _ := json.Marshal(loadConnectors().List())
	return string(b)
}

// ConnectorsTools 派生全部连接器工具 JSON 数组。
func ConnectorsTools() string {
	connMu.Lock()
	defer connMu.Unlock()
	b, _ := json.Marshal(loadConnectors().AllTools())
	return string(b)
}

// ConnectorsAuthURL 构造 OAuth2 授权 URL。state 空则自动生成。
func ConnectorsAuthURL(id, redirectURI, state string) string {
	connMu.Lock()
	d, ok := loadConnectors().Get(id)
	connMu.Unlock()
	if !ok {
		return "ERR:connector not found"
	}
	if state == "" {
		state = connectors.NewState()
	}
	u, err := connectors.BuildAuthURL(d, redirectURI, state)
	if err != nil {
		return "ERR:" + err.Error()
	}
	return u
}

// ConnectorsSaveToken 保存令牌引用（JSON: TokenRef），不落明文。
func ConnectorsSaveToken(tokenRefJSON string) string {
	var t connectors.TokenRef
	if err := json.Unmarshal([]byte(tokenRefJSON), &t); err != nil {
		return "ERR:bad json"
	}
	if t.ConnectorID == "" {
		return "ERR:connector_id empty"
	}
	st := storeRef()
	if st == nil {
		return "ERR:no store"
	}
	key := "connectors.token." + t.ConnectorID
	if b, err := json.Marshal(t); err == nil {
		metaSet(st, key, string(b))
	}
	return "OK"
}

// ConnectorsGetToken 取令牌引用。无则 "ERR:not found"。
func ConnectorsGetToken(id string) string {
	st := storeRef()
	if st == nil {
		return "ERR:no store"
	}
	v := metaGet(st, "connectors.token."+id)
	if v == "" {
		return "ERR:not found"
	}
	return v
}

// ConnectorsRemove 删除一个连接器定义并持久化。返回 "OK"/"ERR:not found"。
func ConnectorsRemove(id string) string {
	connMu.Lock()
	defer connMu.Unlock()
	r := loadConnectors()
	found := false
	nr := connectors.NewRegistry()
	for _, d := range r.List() {
		if d.ID == id {
			found = true
			continue
		}
		_ = nr.Register(d)
	}
	if !found {
		return "ERR:not found"
	}
	persistConnectors(nr)
	return "OK"
}
