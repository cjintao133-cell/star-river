// Package uikit 定义"生成式 UI / 产物(Artifact)"契约与安全渲染策略，对齐 2026 的
// MCP-Apps(SEP-1865) 与声明式生成式 UI(A2UI/OpenUI 风格)：
//
//   - Artifact：模型产出的一块可展示产物（markdown/svg/mermaid/chart/table/form/code/html）。
//   - Component/Catalog：声明式 UI —— 模型只输出"受信任目录内组件组成的树"，宿主原生渲染，
//     不执行任意代码（安全、跨端）。
//   - SandboxPolicy：开放型(html)产物的沙箱与 CSP 策略（最小权限）。
//   - MCP-Apps：把产物包成 ui:// 资源，供宿主在沙箱 iframe 里渲染。
//
// 零内部依赖（仅标准库），便于任何宿主复用与离线测试。
package uikit

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Kind 产物类型。
type Kind string

const (
	KindMarkdown Kind = "markdown"
	KindSVG      Kind = "svg"
	KindMermaid  Kind = "mermaid"
	KindChart    Kind = "chart" // Data 为图表 JSON
	KindTable    Kind = "table" // Data 为二维数组/对象数组
	KindForm     Kind = "form"  // Data 为表单 schema
	KindCode     Kind = "code"
	KindHTML     Kind = "html" // 开放型，需沙箱
)

// Valid 报告 kind 是否已知。
func (k Kind) Valid() bool {
	switch k {
	case KindMarkdown, KindSVG, KindMermaid, KindChart, KindTable, KindForm, KindCode, KindHTML:
		return true
	}
	return false
}

// 限制。
const (
	MaxContentBytes = 2 << 20 // 2MB
	MaxDataBytes    = 2 << 20
)

// Artifact 一块产物。
type Artifact struct {
	ID      string            `json:"id"`
	Kind    Kind              `json:"kind"`
	Title   string            `json:"title,omitempty"`
	Content string            `json:"content,omitempty"` // markdown/svg/mermaid/code/html
	Data    json.RawMessage   `json:"data,omitempty"`    // chart/table/form 的结构化数据
	Meta    map[string]string `json:"meta,omitempty"`
}

// Validate 校验产物（类型、内容/数据、体量）。html 允许存在但由 SandboxPolicy 决定渲染方式。
func (a Artifact) Validate() error {
	if strings.TrimSpace(a.ID) == "" {
		return errors.New("uikit: artifact id empty")
	}
	if !a.Kind.Valid() {
		return fmt.Errorf("uikit: unknown kind %q", a.Kind)
	}
	if len(a.Content) > MaxContentBytes || len(a.Data) > MaxDataBytes {
		return errors.New("uikit: artifact too large")
	}
	switch a.Kind {
	case KindChart, KindTable, KindForm:
		if len(a.Data) == 0 {
			return fmt.Errorf("uikit: %s requires data", a.Kind)
		}
		if !json.Valid(a.Data) {
			return fmt.Errorf("uikit: %s data invalid json", a.Kind)
		}
	case KindHTML:
		if HasScript(a.Content) && !HasMetaTrue(a.Meta, "trusted") {
			// 未标记 trusted 的 html 含脚本 → 交由沙箱；这里不拒绝，仅提示（Validate 仍通过）。
		}
	}
	return nil
}

// HasScript 检测 HTML 内是否有脚本/事件处理器/危险协议。
func HasScript(html string) bool {
	l := strings.ToLower(html)
	return strings.Contains(l, "<script") || strings.Contains(l, "javascript:") ||
		strings.Contains(l, " onload=") || strings.Contains(l, " onclick=") ||
		strings.Contains(l, " onerror=")
}

func HasMetaTrue(meta map[string]string, key string) bool {
	if meta == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(meta[key]), "true")
}

// ---- 声明式 UI 组件树 ----

// Component 声明式组件节点（模型只组合受信任目录内组件）。
type Component struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Props    map[string]any `json:"props,omitempty"`
	Children []Component    `json:"children,omitempty"`
}

// Catalog 允许的组件类型与规模上限。
type Catalog struct {
	Types    map[string]bool
	MaxNodes int
	MaxDepth int
}

// DefaultCatalog 一套中性默认组件目录（原生渲染可映射到 Compose/Flutter/Web）。
func DefaultCatalog() Catalog {
	types := map[string]bool{}
	for _, t := range []string{
		"card", "text", "heading", "button", "input", "image", "list", "table",
		"chart", "form", "row", "column", "divider", "badge", "link", "code", "spacer",
	} {
		types[t] = true
	}
	return Catalog{Types: types, MaxNodes: 500, MaxDepth: 20}
}

// Validate 校验组件树：类型必须在目录内、规模不超限。
func (c Catalog) Validate(root Component) error {
	if len(c.Types) == 0 {
		c = DefaultCatalog()
	}
	maxNodes := c.MaxNodes
	if maxNodes <= 0 {
		maxNodes = 500
	}
	maxDepth := c.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 20
	}
	nodes := 0
	var walk func(comp Component, depth int) error
	walk = func(comp Component, depth int) error {
		if depth > maxDepth {
			return errors.New("uikit: component tree too deep")
		}
		nodes++
		if nodes > maxNodes {
			return errors.New("uikit: too many components")
		}
		if strings.TrimSpace(comp.Type) == "" {
			return errors.New("uikit: component type empty")
		}
		if !c.Types[comp.Type] {
			return fmt.Errorf("uikit: component type %q not in catalog", comp.Type)
		}
		for _, ch := range comp.Children {
			if err := walk(ch, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(root, 0)
}

// ---- 沙箱策略 ----

// SandboxPolicy 开放型产物（html）的渲染策略（最小权限默认）。
type SandboxPolicy struct {
	AllowScripts bool     `json:"allow_scripts"`
	AllowNetwork bool     `json:"allow_network"`
	AllowedHosts []string `json:"allowed_hosts,omitempty"`
	Permissions  []string `json:"permissions,omitempty"` // camera/geolocation/clipboardWrite...
}

// SecureDefault 安全默认：禁止脚本、禁止网络、无权限（纯静态展示）。
func SecureDefault() SandboxPolicy { return SandboxPolicy{} }

// CSP 生成内容安全策略头。默认最严：默认全禁，仅按策略放宽。
func (p SandboxPolicy) CSP() string {
	parts := []string{"default-src 'none'", "style-src 'unsafe-inline'", "img-src data: blob:"}
	if p.AllowNetwork {
		hosts := "'self'"
		if len(p.AllowedHosts) > 0 {
			hosts += " " + strings.Join(p.AllowedHosts, " ")
		}
		parts = append(parts, "connect-src "+hosts)
	} else {
		parts = append(parts, "connect-src 'none'")
	}
	if p.AllowScripts {
		parts = append(parts, "script-src 'unsafe-inline'")
	} else {
		parts = append(parts, "script-src 'none'")
	}
	return strings.Join(parts, "; ")
}

// SandboxAttrs 生成 iframe sandbox 属性值（是否允许脚本）。
func (p SandboxPolicy) SandboxAttrs() string {
	if p.AllowScripts {
		return "allow-scripts"
	}
	return ""
}

// ---- MCP-Apps 资源 ----

// UIResourceURI 生成 ui:// 资源 URI。
func UIResourceURI(id string) string { return "ui://" + id }

// ToMCPResource 把 html 产物包成 MCP-Apps 资源（{uri,mimeType,text}）。非 html 返回错误。
func ToMCPResource(a Artifact, p SandboxPolicy) (map[string]any, error) {
	if a.Kind != KindHTML {
		return nil, errors.New("uikit: only html artifacts become MCP-Apps resources")
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	// 注入 CSP meta（宿主可再强制），便于自包含渲染。
	html := a.Content
	if !strings.Contains(strings.ToLower(html), "content-security-policy") {
		meta := fmt.Sprintf(`<meta http-equiv="Content-Security-Policy" content="%s">`, p.CSP())
		if i := strings.Index(strings.ToLower(html), "<head>"); i >= 0 {
			html = html[:i+6] + meta + html[i+6:]
		} else {
			html = meta + html
		}
	}
	return map[string]any{
		"uri":      UIResourceURI(a.ID),
		"mimeType": "text/html",
		"text":     html,
		"meta": map[string]any{
			"csp":         p.CSP(),
			"sandbox":     p.SandboxAttrs(),
			"permissions": p.Permissions,
		},
	}, nil
}

// ---- 降级 ----

// ToMarkdown 把任意产物降级为可读 Markdown（供不支持富渲染的宿主兜底）。
func ToMarkdown(a Artifact) string {
	switch a.Kind {
	case KindMarkdown:
		return a.Content
	case KindCode:
		return "```\n" + a.Content + "\n```"
	case KindMermaid:
		return "```mermaid\n" + a.Content + "\n```"
	case KindSVG:
		return "```svg\n" + a.Content + "\n```"
	case KindHTML:
		return "(HTML 产物：" + a.Title + ")"
	case KindTable, KindChart, KindForm:
		return "(" + string(a.Kind) + " 产物：" + a.Title + ")\n```json\n" + string(a.Data) + "\n```"
	}
	return a.Content
}
