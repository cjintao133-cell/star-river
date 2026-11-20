# Star River（星河）· 发布说明

> 纯本地、零上传、BYOK 的「赛博生命容器」**内核 / 框架**（Go 内核 + C 热点）。
> 本仓库聚焦**内核与协议**；UI 为参考实现，已暂时移出主树（WIP）。

## 这是什么
一个伴侣型 AI 的**内核底座**：通用底层（事件总线 / 多协议适配 / SQLite 存储 / 工具与 MCP / 七层上下文编排）
+ 可插拔「伴侣插件包」（记忆遗忘曲线 / 五维情绪 / 主动调度）。底座保持干净、可单独编译；
数据与密钥全在本地，凭据只经平台 Keystore/DPAPI。

## 仓库内容
```
kernel/     Go 内核（26 个包）：adapter(openai/anthropic/gemini/router) orchestrator store
            tool(MCP) cards bus hippocampus endocrine proactive visual voice search secret
            backup diag compliance i18n netx native(vector) mobilebind desktopbind …
native/     C/H（device_probe、vector 等），含 Makefile 三平台静态库
cmd/        srchat / livecheck / stresssim（CLI 与调试工具）
scripts/    check-open-split.sh（开源区零闭源依赖校验）等
docs/       架构、契约、评审记录
```
规模：Go 生产 ≈ 19k 行 / 137 文件；C/H ≈ 0.6k 行。

## 快速开始（纯内核）
```bash
# 依赖：Go；C 工具链（Makefile 出三平台静态库）
go build ./...
go test ./kernel/...
staticcheck ./...
make -f native/Makefile all vector     # 产出 build/native/*/lib*.a
bash scripts/check-open-split.sh        # 开源区物理拆分校验（红线2，应 PASS）
```

## 关于 UI
Android(Kotlin Compose) / Windows 桌面 UI 为**参考实现**，已移出主树（还在做）。
需要时：`git checkout with-ui`（或 tag `pre-ui-strip`）取回；AAR/DLL 由 gomobile / c-shared 自建。

## 已知问题
**发布前请务必阅读 [`KNOWN-ISSUES.md`](KNOWN-ISSUES.md)**：含待第三方评审的加密/JNI 边界、未闭环红线项、
真机未验证清单等。

## 许可
Apache License 2.0（见 LICENSE）。「按原样」提供，不含担保。
