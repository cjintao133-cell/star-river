# 星海微内核（Star River Microkernel）

**[English](./README.md)** | 简体中文

> **AI 应用的运行时 —— 不是又一个 AI 应用。**
> 星海微内核（Star River）是一颗可嵌入的 Go 内核：纯 Go 核心 + 一个可选的小型 C 模块（SIMD int8 向量索引）。
> 它接管"模型之外、App 之内"那一层——上下文装配、记忆、情绪、工具/MCP、多模态、加密本地存储；
> 你自带模型（BYOK），它补齐模型周边的一切。无 Python、无容器、无自有后端、无遥测。
>
> **为什么是 Go、为什么小**：单文件静态二进制约 **6.7MB**（`-s -w`，纯 Go 构建、不含可选 C 向量索引）；
> 直接依赖**仅 1 个**（`modernc.org/sqlite`，纯 Go）；冷启动毫秒级；长时记忆状态**有界**（365 天假时钟年测兜底）；
> channel/goroutine 原生；一条命令交叉编译到 Linux/Windows/macOS/arm64，Android 走 `gomobile bind`（需 NDK）。
> 测试扎实：**kernel 各包语句覆盖率平均约 92%**（全树含 CLI 入口二进制按语句加权为 81.7%）；680+ 测试/模糊/基准函数，并发包跑 `-race`，覆盖率在 CI 中设门禁。
> **C 的范围（实话）**：约 1,350 行 C，仅用于可选的 SIMD int8 向量索引、设备探测、JNI/c-shared 桥；核心逻辑为纯 Go。
>
> **状态**：本仓库聚焦**内核 / 框架**，**不含成品 UI**；请以**内核架构 / 协议 / 数据安全**评估本项目。如需成品 App，请基于内核自行开发。

> 文档导航：[产品说明](docs/product-brief.zh.md) · [功能文档](docs/features.zh.md) · [使用说明](docs/usage.zh.md) · [常见问题](docs/faq.zh.md) · [已知问题与解决方案](docs/troubleshooting.zh.md)

星海微内核是一个**可嵌入的 AI 助手内核**：通用底座 + 可插拔的「能力插件包」（记忆遗忘曲线 /
五维情绪 / 主动调度）。底座保持干净、可单独编译、开源；人格与情感等能力以插件形式承载，不是内核的定义。

## 架构

```
        ┌─────────────────────────── 应用层（UI）───────────────────────────┐
        │   Android: Kotlin Compose      Windows: Compose Multiplatform       │
        └───────────────▲───────────────────────────────▲───────────────────┘
                        │ gomobile AAR / c-shared DLL + JNI                │
        ┌───────────────┴───────────────────────────────┴───────────────────┐
        │                    Go 内核（star-river-kernel）                     │
        │  core/types  core/clock  bus   adapter(openai/anthropic/gemini)   │
        │  orchestrator(七层上下文)  store(SQLite WAL+AES-GCM)              │
        │  tool(远程/stdio MCP + 内置工具)  cards(角色卡)  svc-dist(分发)     │
        │  ────────────── 能力插件包（可插拔）──────────────                │
        │  hippocampus(遗忘曲线)  endocrine(五维情绪)  proactive(主动调度)  │
        └───────────────▲───────────────────────────────────────────────────┘
                        │ cgo
        ┌───────────────┴───────────────┐
        │  C 热点模块：device_probe / 向量检索(int8·NEON)  │
        └────────────────────────────────┘
```

- **通信**：命令/查询走直接接口调用；状态变更走 `bus` 门铃广播（订阅者收铃拉最新快照，容忍丢铃，有界缓冲背压）。
- **分层约束（依赖方向）**：底座包**不得** import 能力插件包；插件包可 import 底座。工程内有 `scripts/check-open-split.sh` 守卫（底座可单独编译）。

## 目录结构

```
star-river/
  kernel/       Go 内核
    core/{types,clock}  bus  adapter/  orchestrator/  store/
    cards/  tool/  svc-dist/
    hippocampus/ endocrine/ proactive/   能力插件包
    native/{bridge,vector}  mobilebind/ desktopbind/
  native/       C 模块（device_probe、向量检索）
  cmd/srchat/       最小 CLI 聊天闭环
  docs/             产品说明/使用/FAQ/架构/许可/平台能力/隐私边界（见 docs/README.md）
  scripts/          构建与守卫脚本
```

## 快速开始

前置：Go 1.26+（见 `go.mod`）、C 工具链；Android 另需 NDK r26+/JDK17/Gradle 8.9。

```sh
# 全仓门禁（构建 / 静态检查 / 竞态测试）
go build ./...
go vet ./...
staticcheck ./...
go test -race ./...

# 分层依赖守卫（底座不依赖能力插件包）
sh scripts/check-open-split.sh

# 最小 CLI 对话（key 只经 -keyfile 或 SR_API_KEY，不进命令行/日志）
go run ./cmd/srchat -base https://host/v1 -model <model> -keyfile /path/to/key
```

构建产物：Android `gomobile bind` → AAR；Windows/桌面 `-buildmode=c-shared` → DLL + JNI 桥。

## 角色卡格式

支持四载体导入（`kernel/cards`）：

- **V2 / V3 JSON**：`spec` 或 `spec_v2` + `data` 嵌套结构。
- **chara PNG**：PNG `tEXt`/`iTXt` chunk 内嵌 base64 卡 JSON（`ccv3` 优先 `chara`）。
- **charx**：zip 内 `card.json`（或任意 json 兜底）。
- 世界书条目带触发词选择性激活（`Constant` 常驻 + 关键词激活，深度上限 + 循环检测）。

## 依赖红线

未列入 `docs/architecture.md` 依赖清单者不得引入。当前仅：
标准库优先；`modernc.org/sqlite`（纯 Go SQLite，零 cgo）；`golang.org/x/mobile`（构建链）。
不引第三方 SDK；agent 协议（OpenAI/Anthropic/Gemini/MCP）均手写实现。

## 贡献

- 每提交门禁：`gofmt` 干净 + `go vet` + `staticcheck` + 相关单测（并发包 `-race`）；kernel 包覆盖率 ≥90%；C 代码 ASan+Valgrind 串行零报告。
- 红线（`docs/architecture.md`）不可违反，尤其依赖方向（底座不 import 能力插件包）与密钥不进日志/URL。
- 涉及安全敏感实现（加密、cgo 句柄、签名链）标 `REVIEW-NEEDED`，发布前外部评审。

## 许可

- **整仓 Apache-2.0**（见 `LICENSE` / `docs/licensing.md`），包含全部内核包与记忆/情绪/主动等能力插件包。

## 状态

内核开发中（未闭环项与真机未验项见 `KNOWN-ISSUES.md`；文档索引见 `docs/README.md`）。UI 精修与真机验收集中在后期统一进行。
