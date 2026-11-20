// Package search 是网页搜索模块（新增任务）：统一 SearchProvider 抽象 + 多后端可插拔 + 主备路由 + 本地缓存 + 令牌桶限流。
//
// 两种 Provider 实现，上层不区分：
//   - Native 适配器：直连搜索 API（SearXNG 自托管 / Brave / 百度千帆 …）。
//   - MCP 适配器：在某个已配置的 MCP server 上调其「搜索」工具（Tavily/Exa/Brave/Firecrawl/Perplexity/DuckDuckGo/百度 等搜索 MCP），
//     复用 kernel/tool 的 MCP 客户端；结果映射为统一 Result。
//
// 设计见 docs/search-design.md。产品约束：纯本地、零上传、BYOK、国内外通用、省成本
// （缓存 + 令牌桶 + 「先判断是否需要搜索」在应用层）。
//
// 依赖方向（红线 2）：本包为内核底座，只 import 标准库与 kernel/tool（开源）；不 import 任何闭源插件包。
package search
