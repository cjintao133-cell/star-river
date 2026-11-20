# Star River 社区生态仓库模板

把这个文件夹推到一个 Git 仓库（GitHub/Gitee/GitLab），就能**当星河的社区后端用**，零服务器。

## 目录
```
market/index.json    MCP 市场目录（客户端"集市"拉这个）
forum/posts.json     论坛帖子（客户端"圈内论坛"拉这个，只读镜像）
```

## 推上去后，把 base 告诉 App
App → 设置 → 进阶 → 社区源 URL，三选一：
- GitHub raw：`https://raw.githubusercontent.com/<owner>/<repo>/main/`
- jsDelivr（更快）：`https://cdn.jsdelivr.net/gh/<owner>/<repo>@main/`
- GitHub Pages：`https://<owner>.github.io/<repo>/`

客户端会拼成 `{base}market/index.json` 和 `{base}forum/posts.json` 去拉。

## 加插件（上架）
在 `market/index.json` 的 `plugins` 里加一条：
```json
{ "id":"xxx","name":"xxx","description":"...","transport":"http",
  "endpoint":"https://...","categories":["search"],
  "auth":{"type":"none"},"version":"1.0.0" }
```
- `transport`：`http` | `sse`（`stdio` 只在桌面可用）。
- `auth.type=header` 时，客户端会让你填 key（进 Authorization 头，不进 URL）。
提交 PR → 你 merge = 官方上架（这就是审核）。

## 发帖
- 简单：`forum/posts.json` 里手动/脚本加帖子（只读镜像）。
- 更好：把 `forum/posts.json` 的 `write` 指向仓库 Issues 新建页，客户端「发帖」直接开官方 Issue，用户 GitHub 提交，你审核合并。

## 换成真后端
以后有服务器了，`base` 指过去，**返回同样 JSON 结构**即可，客户端零改动。详见 `docs/community-ecosystem-contract.md`。
