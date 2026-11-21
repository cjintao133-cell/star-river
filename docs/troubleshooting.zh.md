# 星海微内核 · 已知问题 / 未知风险与解决方案

> 面向"要真正用它"的人：把能踩到的坑、尚未验证的风险、以及对应解法列全。
> 与 `KNOWN-ISSUES.md`（发布前必读的安全/红线清单）互补：本文更偏**使用排障**，那份更偏**发布评估**。

---

## A. 构建与环境

| 问题 | 原因 | 解决方案 |
|---|---|---|
| `go build` 报工具链版本错 | `go.mod` 要求 **Go 1.26.11** | 升级到 Go ≥1.26；不降 `go.mod`（用了新特性） |
| `go test -race ./...` 慢或超时 | 并发包竞态检测开销大 | 只对并发包加 `-race`（bus/orchestrator/proactive），或调大超时 |
| C 相关报错 / `make -f native/Makefile` 失败 | 缺 C 工具链 | 装 `gcc`/`clang`；不构建 native 时纯 Go 部分可单独跑 |
| `staticcheck` 未找到 | 未安装 | `go install honnef.co/go/tools/cmd/staticcheck@latest`（CI 里另行安装） |
| `check-open-split.sh` 失败 | 底座某包误 import 了能力包 | 按报错行改回：底座只依赖底座；该守卫是分层约束（非许可） |

## B. 接入模型（最高频踩坑区）

| 问题 | 原因 | 解决方案 |
|---|---|---|
| 报 `必须提供 -base 与 -model` | 两个必填参数缺失 | 补齐；`-base` 形如 `https://host/v1` |
| 报 `未提供 key` | 没给 `-keyfile` 也没设 `SR_API_KEY` | 二选一提供 |
| 连本地 Ollama 报错 / 404 | OpenAI 兼容前缀缺失或服务没起 | `-base http://127.0.0.1:11434/v1`（**带 `/v1`**）；先 `ollama serve` 并 `ollama pull <model>` |
| 本地模型要求 key | 少数本地框架校验非空 | `-keyfile` 给任意非空文件即可（如 `/dev/null` 在某些实现下不行，用含一个字符的临时文件） |
| 某些 Gemini 文本模型返回 200 但正文空 | 已知边角（经 OpenAI 兼容 `/chat/completions` 的流式） | 换用其原生 `/v1/messages` 类适配器（`adapter/gemini`）或换模型；图像模型路径不受影响 |
| 用中转站 302/跳转后失败 | 内核**禁止跟随重定向**（安全红线，防 key 外泄） | 直接用**最终端点**，别用会跳转的短链/营销域名 |
| 拉模型列表失败/超时 | 端点探测超时或该端点不支持 models 接口 | 手动填 `-model`；或换支持 `/v1/models` 的端点 |
| 返回模型名和请求不一致 | 中转"掉包" | `adapter/discovery` 的 `ModelMismatch` 会标记；换可信端点 |

## C. 角色卡导入

| 问题 | 原因 | 解决方案 |
|---|---|---|
| PNG 卡解析不出 | 不是标准 `tEXt/iTXt` 内嵌，或是压缩写法的 chunk | 用 `Parse` 自动识别；必要时导出为 V2 JSON 再导 |
| charx 报"找不到 card.json" | zip 结构不在预期路径 | 会回退"任意 json"；仍失败则解压手动取卡 JSON 用 `ParseJSON` |
| 世界书不触发 | 触发词/`Constant`/递归深度设置问题 | 检查 `BookEntry.Keys`、`Constant`（常驻）、递归深度上限（默认 3，防爆） |

## D. 记忆 / 情绪 / 主动（行为类）

| 现象 | 说明 | 处理 |
|---|---|---|
| 记忆"想不起来" | 召回有阈值（默认 0.15），低于不进上下文；强度随时间衰减 | 提高该条重要度/情绪强度，或手动 `Review` 强化；这是设计而非 bug |
| 情绪值慢慢回到基线 | `Tick` 按时间指数衰减到基线 | 正常；需要保持就 `Interact` 叠加 |
| 主动"太吵"或"不吭声" | 受三重闸：去重/限频（3/时、8/天）/静默（23–7） | 调 `Config`：`Enabled`、`RateLimit`、`QuietHours`、`DefaultCooldown` |
| 主动默认不触发 | `Config.Enabled` 零值=false（出厂关闭，可撤回权限） | 明确 `SetEnabled(true)` 并按需打开对应 `SourceKind` |

## E. 安全实现（尚未外部评审 —— 重要）

以下做了自查加固，但**未经外部人工安全审计**，请在评估/上线前注意（对应 `KNOWN-ISSUES.md` 第一节）：

| 模块 | 现状 | 方案 |
|---|---|---|
| `store/crypto.go` AES-GCM 字段加密 | AAD 绑定行 id，已加固 | 上线前做外部密码学评审；AAD/版本可继续加固 |
| `secret/vault_windows.go` DPAPI | 原子创建，ACL 待审 | 外部评审 ACL 与失败路径 |
| `secret/keybackup.go` | 用户密码派生加密备份 | 评审 KDF 参数与盐管理 |
| `native/vector` cgo | Go 层已校验句柄/维度 | C 侧实现待审；对不可信输入做边界测试 |
| `desktopbind/jnibridge*.c` | 手写 JNI 指针边界 | 外部评审 + fuzz/ASan |
| `svc-dist` ed25519 清单签名 | 签名链在 | 单调版本序号需加固（防回滚） |
| `native/bridge` cgo | — | 评审 |

> **在完成外部评审前，请勿对外宣称"生产级/军用级加密"。**

## F. 未验证 / 未知风险（诚实清单）

- **真机长稳未验**：Android 72h、Windows 24h 后台送达率与稳定性需真实设备跑。
- **发布签名包未做**：当前仅 debug 构建，非正式发布签名。
- **图像效果未人工评审**：水印已强制，但真实自拍/生图观感未做人工验收。
- **红线未完全闭环**（部分实现）：删除覆盖未贯穿向量/派生数据全链路；记忆"出站分级"未完整落地；端点构造期冻结/仅 HTTPS 的完整校验待补。
- **UI 不在本仓库**：本版不含 UI；不作为发布评估对象。

## G. 排障通用流程

1. **先过门禁**：`go build ./... && go vet ./... && go test ./kernel/...`，确认是环境问题还是代码问题。
2. **接入问题最小复现**：`go run ./cmd/livecheck -mode chat -run -base ... -model ... -keyfile ...`，只跑一次最小对话定位是 key/endpoint/协议哪环。
3. **脱敏**：错误信息已脱敏（`cleanOut`/`Redact`/`Scrub`）；贴 issue 时勿贴原始 key。
4. **看诊断**：崩溃只落本地 dump（`diag`），自行决定是否附到 issue。
