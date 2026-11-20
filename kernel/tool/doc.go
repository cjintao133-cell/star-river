// Package tool 是工具运行时（T-M6-01 远程 MCP 客户端）。
//
// 依赖方向：本包为内核底座的一部分（开源，MCP 为开放协议），只依赖标准库；
// 不 import 任何能力插件包（hippocampus/endocrine/proactive）。
//
// MCP（Model Context Protocol）用 JSON-RPC 2.0 编码，本包实现 **Streamable HTTP**
// 传输（协议版本 2025-06-18，向后兼容尝试）：
//   - 单一 MCP 端点，POST 发送 JSON-RPC（Accept 同时声明 application/json 与 text/event-stream）。
//   - 响应体可能是一份 application/json，也可能是 text/event-stream（SSE），需都能解析。
//   - 初始化响应头 Mcp-Session-Id 指定会话，后续请求带上；MCP-Protocol-Version 头声明版本。
//   - 会话失效（404）→ 重开会话。
//
// 能力（T-M6-01 验收）：
//   - 工具列表发现 tools/list（含 cursor 分页）。
//   - 工具调用 tools/call（每请求可配超时；ctx 取消时尽力发 notifications/cancelled）。
//   - 热插拔：Manager 支持运行时 添加/移除/启停 服务器，无需重启进程。
//
// “工具调用前权限确认 UI”属界面层（押后），本包只提供内核侧的候选与调用入口。
package tool
