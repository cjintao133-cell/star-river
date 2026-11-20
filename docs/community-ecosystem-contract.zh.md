# 社区生态 · Git 仓库当后端的接口契约（v1）

> 目的：**不搭服务器**，先用一个 Git 仓库（GitHub/Gitee/GitLab）承载「MCP 市场目录 + 论坛内容」。
> 客户端只做 **HTTPS GET 拉静态 JSON**（只读、不带用户数据、符合零上传）。
> 关键设计：**契约与「托管在哪」解耦**——`base` 今天指 raw.githubusercontent，明天指自建 API，客户端一行不用改。

## 0. 能/不能（先说清楚）

| 能力 | Git 仓库能否 | 说明 |
|---|---|---|
| MCP 市场**目录**（读） | ✅ | 仓库里放 `market/index.json`，客户端拉取 |
| MCP 插件**安装** | ✅ | 目录条目含 `endpoint`，客户端 `mcpAddServer` 直接连（插件本体是远端 MCP server，不在仓库里） |
| 论坛**浏览**（读） | ✅ | 仓库里放 `forum/posts.json`（合并后的帖子） |
| 论坛**发帖/回复**（写） | ⚠️ 半 | Git 本身不适合高频写。两种做法：①帖子合并到仓库=只读镜像；②**用 GitHub Issues / Discussions 当写入口**（客户端打开 issue 或调 API，需用户 GitHub 账号）。 |
| 审核/防投毒 | ✅ | 靠仓库 PR 审核（你 merge 才算官方上架）+ 可选签名文件 |

**结论**：目录/浏览→Git 仓库完美；发帖→用 GitHub Issues 兜，或后续接真后端。契约不变。

## 1. 托管方式（任选，客户端只认 base）

- **GitHub raw**：`https://raw.githubusercontent.com/<owner>/<repo>/main/`
- **jsDelivr CDN**（更快、有缓存）：`https://cdn.jsdelivr.net/gh/<owner>/<repo>@main/`
- **GitHub Pages**：`https://<owner>.github.io/<repo>/`
- 自建 API（以后）：`https://api.你的域名/`
客户端把上面之一填进「设置 → 社区源」，后面路径固定拼接。

## 2. `market/index.json`（MCP 市场目录）

```json
{
  "version": 1,
  "updated": "2026-11-20T00:00:00Z",
  "plugins": [
    {
      "id": "tavily",
      "name": "Tavily 搜索",
      "description": "网页搜索，摘要省 token，适合日常",
      "transport": "http",
      "endpoint": "https://mcp.tavily.com/mcp",
      "categories": ["search"],
      "homepage": "https://tavily.com",
      "publisher": "tavily",
      "auth": { "type": "none" },
      "version": "1.0.0"
    },
    {
      "id": "brave-search",
      "name": "Brave 搜索",
      "description": "Brave Search MCP，需 API Key",
      "transport": "sse",
      "endpoint": "https://mcp.brave.com/sse",
      "categories": ["search"],
      "auth": { "type": "header", "header": "Authorization", "hint": "Bearer <你的 Brave Key>" },
      "version": "1.0.0"
    }
  ]
}
```

字段：
- `transport`: `http` | `sse`（**Android 仅这两类**；`stdio` 标了也只在桌面可用，客户端在安卓上置灰）。
- `auth.type`: `none` 直接装；`header` 需用户填值（填到 `header` 头里），**不进 URL、不写日志**。
- `id` 唯一；同名（`name`）作为本地服务器标识，重装以 `id` 去重。

## 3. `forum/posts.json`（论坛只读镜像 / 审核合并后）

```json
{
  "version": 1,
  "updated": "2026-11-20T00:00:00Z",
  "posts": [
    {
      "id": "p1",
      "title": "2GB 安卓跑动效背景怎么降级不卡",
      "author": "新人阿柴",
      "created": "2026-11-18T10:00:00Z",
      "replies": 7,
      "tags": ["help"],
      "body": "开了流星背景，切后台回来微微掉帧。"
    }
  ]
}
```
写入口（可选）：`forum.write` 指向仓库 Discussions/Issues，客户端「发帖」按钮直接打开对应 URL（用户 GitHub 账号提交）。

## 4. 客户端行为

1. 进「MCP 集市」→ GET `{base}market/index.json`；成功则渲染目录卡片（安装按钮），失败/超时 → 回落内置种子并提示「源不可达」。
2. 进「圈内论坛」→ GET `{base}forum/posts.json`；失败回落种子。
3. 安装 = 读条目 →（auth=none）`mcpAddServer(name, endpoint, "")`；auth=header → 弹框填值 → `mcpAddServer`。
4. 全部只读 GET，无用户数据外发；base 可配置，便于换自建。

## 5. 安全 / 防投毒

- **只有你 merge 进主分支的条目才算"官方上架"**——PR 审核即审核。
- 可选 `market/index.json.sig`（ed25519，沿用 svc-dist 的签名思路）——后续加，v1 先靠仓库权限。
- 客户端对 `transport=stdio` 在安卓置灰；`endpoint` 必须 `https://`（拒明文 HTTP）。

## 6. 何时该换真后端

当出现：需要**实时发帖/点赞/用户系统**、**私有插件付费/鉴权**、**频繁更新目录**。这时把 `base` 指向真 API，**返回同样的 JSON 结构即可平滑切换**。
