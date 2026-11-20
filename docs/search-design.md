# 网页搜索模块 · 设计结论（T-M 新增 / 供 kernel/search 落地）

> 来源：联网调研（各家搜索 API 定价/SearXNG）+ fa5.2(claude-fable-5-2) 架构评审交叉验证。
> 产品约束：纯本地、零上传、BYOK（用户自带密钥）、Android+Windows、国内国外通用、省成本。

## 一、验收目标（UI）
用户搜索框输入 → 结果页显示：**标题 + 链接 + 摘要 + 缩略图**（类百度）；同时供 AI 工具调用（主脑可搜索）。

## 二、候选后端（调研事实）
| 后端 | 成本 | 缩略图 | 备注 |
|---|---|---|---|
| **SearXNG（自托管）** | 免费 | 视引擎，不一定 | AGPL-3.0，聚合 70+ 引擎，JSON 返 title/url/snippet；用户可自建 |
| Brave Search API | ~$5/1k | 有（含 image 字段） | 海外稳定 |
| Exa | ~$7/1k | 有限 | 偏语义/长文检索 |
| SerpAPI | $25 起/月 | 有（Google 内联图） | 单价高，仅需 Google 图时用 |
| Tavily | 有免费额度 | 有限 | 偏 AI 检索 |
| Bing Search API | — | — | **已于 2025-08 退役，不接**（改为 Azure Grounding with Bing） |
| 国内：博查 Bocha / 智谱搜索 | 各家 | 有 | 国内直连更稳（Brave/Tavily 国内不稳） |

## 三、结论

### 1) 适配器抽象（kernel/search）
- **统一接口**：`Search(ctx, Query{Text, Lang, Region, Count, Recency}) → []Result`；另有 `HealthCheck()`、`Quota()`。
- **统一结果结构**：`Result{ Title, URL, Snippet, Thumbnail(可空), Favicon(可空), PublishedAt(可空), Source }`。
- 各适配器**声明能力**（是否带缩略图 / 支持中文 / 支持时效），供路由决策；上层不感知具体后端。

### 2) BYOK 后端组合 + 主备
- **免费优先**：用户自建 SearXNG（首选）→ Tavily 免费额度 → Brave 付费。
- **海外链路**：Brave 主、Tavily 备；Exa 仅语义/长文；SerpAPI 仅在要 Google 缩略图时启用。
- **国内链路**：增补国内适配器（博查/智谱），或 SearXNG 启用百度/搜狗引擎。
- **路由**：启动探测连通性 → 按「已填 Key、可达、有余额」自动排序 → 失败即降级。

### 3) 缩略图缺失处理（隐私友好）
- 降级链：后端自带图 → 本地抓目标页 `og:image`（仅前几条、短超时、异步、不阻塞文字）→ 站点 favicon → 域名首字母色块。
- **不用第三方 favicon 服务**（防泄露浏览行为）；提供「不加载远程图片」开关。

### 4) 缓存与限流（省钱核心）
- 结果落**本地 DB 缓存**：键=规范化查询+参数；时效类 ~15min、常规 ~24h；缩略图磁盘 LRU。
- 每后端独立**令牌桶** + 本地记账月配额 + 每日花费上限；429/超时 → 熔断+指数退避+切备用。
- **最大省钱点**：先由模型判断"是否需要搜索" + 相似查询去重。

### 5) AGPL 风险
- 通过 **HTTP 调用用户自己的 SearXNG 实例不构成衍生作品**，App 不受 AGPL 传染；若**捆绑/内嵌** SearXNG 代码或分发则需评估（建议：只做客户端连接，不分发 SearXNG 本体）。

### 6) 一句话最终架构
> `kernel/search` 定义统一 `SearchProvider` 接口 + 多适配器（SearXNG/Brave/博查/Tavily…）可插拔，配能力声明与自动路由主备；本地缓存+令牌桶限流控成本；缩略图多级降级且隐私友好；BYOK 用户自带 Key/自建实例，默认零上传。

## 四、接入热门搜索 MCP（硬要求，用户强调「记住」）

搜索**不只走原生 API 适配器**，还要能接各种**热门搜索类 MCP 服务器**，与原生适配器一起参与统一检索/主备路由。

- **已核实的常见搜索 MCP**（均把「搜索」暴露成 MCP 工具）：
  | 搜索 MCP | 认证 | 备注 |
  |---|---|---|
  | Tavily MCP | API key | 检索质量高、有免费额度 |
  | Exa MCP | 无 key(免费档限流)/OAuth/key | 远程 MCP，精炼检索+抓取 |
  | Brave MCP | API key | 传统搜索体验 |
  | Firecrawl MCP | API key | 搜索+整站抓取/提取 |
  | Perplexity Web MCP | Pro/Max 订阅 | 30+ 工具，含深度研究 |
  | DuckDuckGo MCP | 免费 | 无 key、轻量 |
  | **百度搜索 MCP** | API key | 国内首选。两类：① 第三方 baidu-search-mcp（基于文心API，支持深度搜索+时效过滤）；② 百度千帆官方实时搜索接口（query/count/freshness）。走 MCPProvider 统一接入 |
  | Parallel AI | API key | 面向 agent |
- **复用现有内核**：搜索 MCP 本质就是一个暴露 `search` 工具的 MCP server —— 走 `kernel/tool` 的 MCP 客户端（Streamable HTTP / stdio[Windows]）+ `kernel/tooldisc` 注册索引，**无需为每个 MCP 单独写代码**。
- **统一抽象**：`SearchProvider` 两种实现——① Native 适配器（SearXNG/Brave/博查…直连）；② MCP 适配器（在配置好的 MCP server 上调其 search 工具，结果映射到统一 `Result`）。上层/搜索页/路由不区分二者。
- **配置与切换**：设置里可「添加搜索 MCP 服务器」（填端点/key/工具名）→ 进 `tooldisc` 索引 → 搜索页与主备路由可选用；支持多接、排序、启用/停用。
- **路由**：统一按「已配置、可达、有额度」在 Native 与 MCP 后端间择优、互为备份。

> 结论补充：搜索后端 = **原生 API 适配器 + 热门搜索 MCP 适配器** 两路统一，均可插拔、一起路由。

## 五、落地下一步（建议）
1. `kernel/search`：`SearchProvider` 接口 + `Result` 结构 + 路由（主备/能力声明）+ 本地缓存 + 令牌桶。
2. 先实现 **SearXNG 适配器**（免费、可自托管、便于实测）+ **Brave 适配器**（海外付费档）；国内适配器（博查）接口先留位。
3. 测试：httptest mock 各后端 JSON（title/url/snippet/thumbnail 解析、缺图降级、429 熔断、主备切换、缓存命中）；不连真网。

> 语音（STT/TTS）另立设计（`kernel/voice`）：适配器抽象 + 音频前处理（native）+ 可选第三方；下一轮做。
