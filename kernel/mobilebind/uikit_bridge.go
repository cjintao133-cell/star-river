package mobilebind

// 生成式 UI / 产物桥：校验产物与声明式组件树、生成 MCP-Apps 资源、CSP 与降级 Markdown。

import (
	"encoding/json"

	"github.com/cjintao133-cell/star-river/kernel/uikit"
)

// UIArtifactValidate 校验产物 JSON，返回 "OK"/"ERR:..."。
func UIArtifactValidate(artifactJSON string) string {
	var a uikit.Artifact
	if err := json.Unmarshal([]byte(artifactJSON), &a); err != nil {
		return "ERR:bad json"
	}
	if err := a.Validate(); err != nil {
		return "ERR:" + err.Error()
	}
	return "OK"
}

// UIArtifactToMarkdown 把产物降级为 Markdown。
func UIArtifactToMarkdown(artifactJSON string) string {
	var a uikit.Artifact
	if err := json.Unmarshal([]byte(artifactJSON), &a); err != nil {
		return "ERR:bad json"
	}
	return uikit.ToMarkdown(a)
}

// UIRenderSpecValidate 用默认组件目录校验声明式组件树 JSON。返回 "OK"/"ERR:..."。
func UIRenderSpecValidate(specJSON string) string {
	var c uikit.Component
	if err := json.Unmarshal([]byte(specJSON), &c); err != nil {
		return "ERR:bad json"
	}
	if err := uikit.DefaultCatalog().Validate(c); err != nil {
		return "ERR:" + err.Error()
	}
	return "OK"
}

// UICatalogJSON 返回默认组件目录（组件类型列表）。
func UICatalogJSON() string {
	cat := uikit.DefaultCatalog()
	types := make([]string, 0, len(cat.Types))
	for t := range cat.Types {
		types = append(types, t)
	}
	b, _ := json.Marshal(map[string]any{"types": types, "max_nodes": cat.MaxNodes, "max_depth": cat.MaxDepth})
	return string(b)
}

// UIResourceForMcp 把 html 产物包成 MCP-Apps 资源。policyJSON 可空（安全默认）。
func UIResourceForMcp(artifactJSON, policyJSON string) string {
	var a uikit.Artifact
	if err := json.Unmarshal([]byte(artifactJSON), &a); err != nil {
		return "ERR:bad json"
	}
	p := uikit.SecureDefault()
	if policyJSON != "" {
		_ = json.Unmarshal([]byte(policyJSON), &p)
	}
	res, err := uikit.ToMCPResource(a, p)
	if err != nil {
		return "ERR:" + err.Error()
	}
	b, _ := json.Marshal(res)
	return string(b)
}

// UISandboxCSP 返回给定策略的 CSP 字符串。
func UISandboxCSP(policyJSON string) string {
	p := uikit.SecureDefault()
	if policyJSON != "" {
		_ = json.Unmarshal([]byte(policyJSON), &p)
	}
	return p.CSP()
}
