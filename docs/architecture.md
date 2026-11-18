# 架构与包边界（初版，随 T-M0-03 建立）

正典链：prd/星海微内核开发任务清单.md → prd/增补任务-v1范围.md → docs/blueprint-review.md（12 条硬约束，台账 docs/constraints-12.md）。

## 硬事实
纯本地、零用户数据上传、BYOK。Go 内核 + C 热点模块 + Kotlin Compose UI。Android 与 Windows 双端；构建矩阵在腾讯云 Linux 交叉编译（docs/env-linux.md）。

## 包边界
| 包 | 职责 | 许可区 |
|---|---|---|
| kernel/core/types | 内核数据契约（六类型 + Validate，纯标准库） | 开源 |
| kernel/bus | 事件广播（门铃）+ 直调混合 | 开源 |
| kernel/adapter | openai/anthropic/gemini 三协议 + 发现/分类/灾备 | 开源 |
| kernel/orchestrator | 七层上下文 + 预算分配 + 摘要接力 | 未定（暂私有） |
| kernel/store | SQLite WAL 单写 + AES-GCM 字段加密 | 未定（暂私有） |
| kernel/hippocampus | 遗忘曲线记忆引擎 | **闭源** |
| kernel/endocrine | 五维情绪状态机 | **闭源** |
| kernel/proactive | 主动调度 | **闭源** |
| kernel/cards | V2/V3/chara PNG 角色卡解析 | 开源 |
| kernel/tool | 远程 MCP + Windows stdio + 内置工具 | SDK 部分开源 |
| kernel/visual | 视觉配方/生成适配/AI 水印（不可关） | 未定（暂私有） |
| kernel/codebrain | v1 仅 JSON 配置，v2 真实代码 | 未定 |
| native/ | C：device_probe、向量检索、音频前处理 | probe 开源，向量默认闭源 |
| ui-android / ui-desktop | Kotlin Compose / Compose Multiplatform | 可开源（发布时定） |
| svc-dist | manifest/CDN/ed25519 签名/更新链 | 未定（暂私有） |

硬规则：开源区不得 import 闭源包（红线 2）；反向允许。记忆/情绪/主动逻辑只许住在三闭源包内。

## 通信约定
- 命令/查询：直接接口调用（同步，可追堆栈可管超时）。
- 状态变更：bus 广播。事件只当门铃：payload 带版本号，订阅者收铃拉最新快照，容忍丢铃（批准优化#4）。
- 背压：有界缓冲，满时丢弃最旧 + 计数；慢订阅者不得影响发布者（T-M1-02 验收）。
- v1 实现注记（T-M1-02）：kernel/bus 落地；订阅者收全部主题、过滤在消费侧；缓冲默认 64（New 参数≤0 取默认）；ErrClosed 语义；丢弃数经 Subscription.Dropped() 读取；隔离测试为锁步设计。

## 依赖清单（红线 1：未列者不得引入）
- 标准库优先。
- modernc.org/sqlite：纯 Go SQLite 驱动，三平台交叉编译零 cgo（批准优化#3；T-M2-01 前正式 go get 入账本行）。
- golang.org/x/mobile（gomobile）：构建链工具，已装。
- 工具链：staticcheck、garble（构建期，不进产物依赖）。
- hnswlib：缓行，真机基准不达标再议，届时须单独批准（批准优化#2）。

## 已批准优化落文（评审记录见 STATUS）
1. 上下文按稳定性排序：稳定层前、易变层（情绪/时间）尾，系统提示词禁注每轮必变字段，最大化前缀缓存（T-M3-03）。
2. 向量 v1：int8 量化 + NEON 暴力检索，接口留 HNSW 升级位（T-M3-01）。
3. SQLite 驱动 modernc.org/sqlite（见依赖清单）。
4. bus 门铃 + 快照（见通信约定）。
5. AES-GCM AAD 绑定行 id + 字段名，防跨行密文搬运；REVIEW-NEEDED（T-M2-02）。
6. embedding 按内容 hash 落 SQLite 缓存，命中不调端点（T-M3-01/02）。
7. garble spike 提前至 T-M0-04/05 之后（T-M0-09 顺序微调，Gate 不变）。

## 平台差异（12 条 #5，代码不得假设全平台一致）
- Android：MCP 仅远程协议；内置工具进程内；不执行动态下载二进制（红线 9）。
- Windows：stdio MCP 独立进程；托盘/DPAPI 走接口抽象 + Linux mock 测试。
- 平台能力表：docs/platform-capability.md（T-M6-02 前建立，M0 结论先记 m0-conclusions.md）。

## 工程纪律
- UI 文案一律走字符串表，禁硬编码（增补 T-M2-07）。
- 每提交门禁：go vet + staticcheck + 相关单测（并发包 -race）；core 覆盖率 ≥70%；C 加 ASan+Valgrind 串行零报告。
- go test -p 2、make -j4、Xvfb 单实例；构建 TMPDIR=/root/build-tmp（/tmp 是 tmpfs）。
- 测试只访问 mock server 与 localhost。

## T-M0-04 增补（实测后落文）
- 新包 kernel/mobilebind：gomobile AAR 桥接门面（开源区，薄层，函数签名稳定，内核逻辑不得写在桥层）。PRD 结构表未列，属桥接必需，随 T-M0-04 入表。
- go.mod：tool golang.org/x/mobile/cmd/gomobile 入账（v0.0.0-20261117202452）；随带间接 x/mod/x/sync/x/tools 为工具链内部用；go 指令抬至 1.26.0（PRD 下限 1.22+ 不违反）。
- 安卓构建链锁定：Gradle 8.9 wrapper + AGP 8.5.2 + Kotlin 2.0.21 + Compose BOM 2024.09.03（仅构建期，不进内核依赖）。
- 新包 kernel/desktopbind（T-M0-05）：c-shared 桌面出口 + 手写 JNI 桥（jnibridge.c，REVIEW-NEEDED 待评审）+ wininc 最小交叉头；开源区，薄门面复用 mobilebind。
