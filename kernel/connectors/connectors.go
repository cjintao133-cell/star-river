// Package connectors 是"连接器/集成"框架骨架：统一声明连接器（AuthSpec + 工具集），
// 生成 OAuth 授权 URL、把连接器工具转成中性工具定义、管理令牌引用（TokenStore 接口）。
//
// 不做真实网络：授权跳转的 URL 构造、state 生成、工具清单派生都是纯逻辑；真正的令牌交换
// 与 API 调用由入口层实现。零内部依赖（仅标准库）。
package connectors

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

// AuthType 授权类型。
type AuthType string

const (
	AuthOAuth2 AuthType = "oauth2"
	AuthAPIKey AuthType = "apikey"
	AuthNone   AuthType = "none"
)

// AuthSpec 授权规格。
type AuthSpec struct {
	Type     AuthType `json:"type"`
	AuthURL  string   `json:"auth_url,omitempty"`
	TokenURL string   `json:"token_url,omitempty"`
	Scopes   []string `json:"scopes,omitempty"`
	ClientID string   `json:"client_id,omitempty"`
	// APIKey 放置方式（供 APIKey 类型）。
	KeyHeader string `json:"key_header,omitempty"`
}

// ToolDef 连接器提供的工具定义（中性）。
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

// ConnectorDef 连接器定义。
type ConnectorDef struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Auth    AuthSpec  `json:"auth"`
	Tools   []ToolDef `json:"tools"`
	BaseURL string    `json:"base_url,omitempty"`
}

// Connector 一个连接器实例（持有定义）。
type Connector struct{ Def ConnectorDef }

// Registry 连接器注册表。
type Registry struct {
	mu  sync.Mutex
	m   map[string]*Connector
	ord []string
}

// NewRegistry 空注册表。
func NewRegistry() *Registry { return &Registry{m: map[string]*Connector{}} }

// Register 注册连接器（同 id 覆盖）。
func (r *Registry) Register(def ConnectorDef) error {
	if strings.TrimSpace(def.ID) == "" {
		return errors.New("connectors: id required")
	}
	if def.Auth.Type == "" {
		def.Auth.Type = AuthNone
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.m[def.ID]; !ok {
		r.ord = append(r.ord, def.ID)
	}
	r.m[def.ID] = &Connector{Def: def}
	return nil
}

// List 全部连接器定义。
func (r *Registry) List() []ConnectorDef {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ConnectorDef, 0, len(r.ord))
	for _, id := range r.ord {
		out = append(out, r.m[id].Def)
	}
	return out
}

// AllTools 派生全部连接器工具（名称加 connector 前缀，避免冲突）。
func (r *Registry) AllTools() []ToolDef {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []ToolDef
	for _, id := range r.ord {
		for _, t := range r.m[id].Def.Tools {
			tt := t
			tt.Name = id + "_" + t.Name
			out = append(out, tt)
		}
	}
	return out
}

// Get 取连接器定义。
func (r *Registry) Get(id string) (ConnectorDef, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.m[id]
	if !ok {
		return ConnectorDef{}, false
	}
	return c.Def, true
}

// BuildAuthURL 构造 OAuth2 授权 URL（含 client_id、redirect_uri、scope、state）。
// 非 oauth2 连接器返回错误。state 由调用方提供（应为不可猜测值）。
func BuildAuthURL(def ConnectorDef, redirectURI, state string) (string, error) {
	if def.Auth.Type != AuthOAuth2 {
		return "", fmt.Errorf("connectors: %s is not oauth2", def.ID)
	}
	if strings.TrimSpace(def.Auth.AuthURL) == "" {
		return "", errors.New("connectors: auth_url empty")
	}
	u, err := url.Parse(def.Auth.AuthURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("response_type", "code")
	if def.Auth.ClientID != "" {
		q.Set("client_id", def.Auth.ClientID)
	}
	q.Set("redirect_uri", redirectURI)
	if len(def.Auth.Scopes) > 0 {
		q.Set("scope", strings.Join(def.Auth.Scopes, " "))
	}
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// NewState 生成随机 state（防 CSRF）。
func NewState() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "state-fallback"
	}
	return hex.EncodeToString(b[:])
}

// TokenRef 一个令牌引用（不存明文，只存引用位置 + 元数据）。
type TokenRef struct {
	ConnectorID string   `json:"connector_id"`
	SecretID    string   `json:"secret_id"` // 指向 secret.Vault 的键
	Scopes      []string `json:"scopes,omitempty"`
	ExpiresAt   int64    `json:"expires_at,omitempty"`
}

// TokenStore 令牌引用持久化接口（入口层实现）。
type TokenStore interface {
	Save(TokenRef) error
	Get(connectorID string) (TokenRef, bool, error)
	List() ([]TokenRef, error)
	Delete(connectorID string) error
}

// MemStore 内存令牌引用存储。
type MemStore struct {
	mu sync.Mutex
	m  map[string]TokenRef
}

// NewMemStore 建内存存储。
func NewMemStore() *MemStore { return &MemStore{m: map[string]TokenRef{}} }

// Save 实现 TokenStore。
func (s *MemStore) Save(t TokenRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[t.ConnectorID] = t
	return nil
}

// Get 实现 TokenStore。
func (s *MemStore) Get(id string) (TokenRef, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.m[id]
	return t, ok, nil
}

// List 实现 TokenStore。
func (s *MemStore) List() ([]TokenRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]TokenRef, 0, len(s.m))
	for _, t := range s.m {
		out = append(out, t)
	}
	return out, nil
}

// Delete 实现 TokenStore。
func (s *MemStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, id)
	return nil
}

// MarshalConnector 便于桥层输出 JSON。
func MarshalConnector(d ConnectorDef) string {
	b, _ := json.Marshal(d)
	return string(b)
}
