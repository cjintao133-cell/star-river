package tool

import "runtime"

// PlatformCapability 平台 MCP 能力声明（跨平台能力表，配合 docs/platform-capability.md）。
type PlatformCapability struct {
	OS        string `json:"os"`
	RemoteMCP bool   `json:"remote_mcp"` // Streamable HTTP 远程 MCP（全平台）
	StdioMCP  bool   `json:"stdio_mcp"`  // stdio 子进程 MCP（仅 Windows）
}

// Capability 返回当前运行平台的能力声明。
//
// 依据 docs/architecture.md「平台差异（12 条 #5）」：
//   - Android：MCP 仅远程协议（stdio 不可用，不执行动态下载二进制，红线 9）。
//   - Windows：远程 + stdio（独立进程）。
//   - 其他（Linux 等开发/测试平台）：远程可用；stdio 传输实现存在、可跑测试，
//     但**非产品目标**，能力位按 Windows-only 声明为 false（产品层面不启用）。
func Capability() PlatformCapability {
	return PlatformCapability{
		OS:        runtime.GOOS,
		RemoteMCP: true,
		StdioMCP:  runtime.GOOS == "windows",
	}
}

// StdioMCPEnabled 报告当前平台在产品层面是否启用 stdio MCP（仅 Windows）。
func StdioMCPEnabled() bool { return Capability().StdioMCP }
