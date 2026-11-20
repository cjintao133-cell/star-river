# 星海微内核 · 功能文档

> 本文档逐模块描述**代码里真实实现的能力**，每项都能在对应 `kernel/` 包里找到出处。
> 定位：**可嵌入的 AI 助手内核**（Go + C，Android/Windows 双端）。它不运行模型权重，负责"模型之外、App 之内"的那一层：上下文装配、记忆、情绪、主动、工具/MCP、多模态适配、加密存储、双端桥接。
> 规模：Go 生产 ≈ 19.5k 行 / 139 文件；C/H ≈ 0.9k 行；26 个内核包 + 3 个命令。

---

## 一、模型接入层（`kernel/adapter`）

**统一把"任意模型端点"变成内核可调用的对话能力。**

| 子包 | 真实能力（导出接口） |
|---|---|
| `adapter/openai` | `New(baseURL, key)` → `Client`；`ChatRequest/ChatResult/Usage/ToolDef/Citation/ModelInfo`；流式（`core.StreamEvent`）。**OpenAI 兼容端点都能接**，故可指向 Ollama `http://127.0.0.1:11434/v1`、LM Studio、vLLM、各类中转。 |
| `adapter/anthropic` | `New` → Anthropic Messages 协议客户端（`x-api-key` + `anthropic-version`）。 |
| `adapter/gemini` | `New` → Gemini 协议客户端。 |
| `adapter/discovery` | 模型目录发现与分类：`Discover`、`Classify`、`BuildCards`、`HeuristicCaps`、`LazyValidate`、`ModelMismatch`；`Capability`/`CapabilityCard`/`Catalog`。用于"拉模型列表 + 猜能力 + 校验掉包"。 |
| `adapter/probe` | 能力探测：`Caps`、`Signal`、`AsProbeFn`。探测端点真实能力（如是否支持图像/工具）。 |
| `adapter/router` | 多端点路由 + 主备/灾备：`New`、`WithFailoverPredicate`、`WithStats`、`WithPublisher`、`WithAllowHigherTier`；`PlanEntry`/`PriceTier`/`StatsProvider`。按成功率/延迟/价格选路。 |

> 三协议手写实现、零第三方 SDK；统一出站经 `kernel/netx`（禁止跟随重定向，防凭据跨域外泄）。

## 二、上下文引擎（`kernel/orchestrator`）

**决定"把哪些内容、按什么优先级、塞进有限的上下文窗口"——这是内核最硬的一块。**

- **七层上下文**（`Layer`）：系统指令 / 人设 / 世界书 / 长期记忆召回 / 滚动摘要 / 近期消息 / 本次用户输入。
- **预算分配**（`Config`：`TotalChars` 总预算、`Priority[NumLayers]` 各层优先级、`MinRecentMessages` 近期不可压下限）。
- **装配**（`Assembler.Assemble(Layers, MemorySink) → Report`）：超预算时从最低优先级层开始压缩/裁剪，整条目级拟合、摘要按字符截断。
- **截断前强制记忆沉淀**（`MemorySink`）：被裁掉的近期消息交给记忆引擎落库，**不静默丢失**。
- **最小装配**（`BuildMinimal`）与**滚动摘要接力**。

## 三、存储层（`kernel/store`）

**端侧加密数据基座，纯 Go、零 cgo。**

- `Open` 打开 SQLite（WAL 单写）；`LatestVersion` 迁移版本化。
- `FieldCipher`（`NewFieldCipher`/`FieldCipherFromVault`）：**字段级 AES-GCM 加密**（消息正文 `content_cipher`）。
- `Repo`（会话/消息）、`CharacterRepo`（角色卡）、`StatsRepo`（模型统计）；`MessagePage` 分页；`Redact` 脱敏；`ModelStats` 成功率/延迟/token。

## 四、密钥层（`kernel/secret`）

- `Vault` 抽象：`DPAPIVault`（Windows DPAPI）、`MemVault`（内存，测试）、`DelegateVault`（委托）。
- `EncryptBackup`/`DecryptBackup`：密钥加密备份（用户密码派生）。
- `Scrub`/`ScrubString`：日志脱敏（红线：密钥不进日志）。

## 五、角色卡（`kernel/cards`）

`Parse` 自动识别四载体：`ParseJSON`（V1/V2/V3）、`ParsePNG`（`tEXt`/`iTXt` 内嵌 base64，`ccv3` 优先 `chara`）、`ParseCharx`（zip 内 `card.json`）；`IsPNG`/`IsCharx` 判定。世界书 `BookEntry` + `ActivatedEntry`（`Constant` 常驻 + 触发词递归激活，深度上限 + 循环检测）。

## 六、工具/MCP（`kernel/tool` + `kernel/tooldisc`）

- `Client`（Streamable HTTP）/`StdioClient`/`StdioManager`：连远程或 stdio 的 MCP server。
- `Manager`：**多 server 热插拔**（`Add`/`Remove`/`SetEnabled`/`RefreshTools`/`CallTool`），运行时增删不重启。
- 内置工具：`NewMemoryQueryTool`、`NewMoodReportTool`、`NewVisualRequestTool`（进程内、供主脑调用）。
- `tooldisc`：**工具两段式按需发现**（`Index`/`SearchTool`/`EstimateTokens`/`FootprintTokens`），千级工具不撑爆上下文。

## 七、记忆（`kernel/hippocampus`）

- `Engine`：`Encode`（编码记忆，按重要度/情绪定初始稳定度）、`Review`（命中强化）、`Recall`/`RecallFromStore`（按留存度打分召回）。
- `Retention`：遗忘曲线 `R = exp(-t/S)`；`Memory` 含稳定度/强度/复习时刻。
- `MemorySim`/`RunMonth`：年/月尺度模拟（可测）。

## 八、情绪（`kernel/endocrine`）

- `Engine`：`Tick`（定时衰减）、`Interact`（对话触发叠加增量）、`State`/`Snapshot`/`Restore`。
- `Registry`/`Dimension`：**数据驱动维度表**（默认五维，可扩维/注销，如 `DominanceDimension`）。
- `Coupler`：情绪→表达风格耦合规则（`DefaultCoupler`/`DefaultRules`/`StyleParams`）。

## 九、主动（`kernel/proactive`）

- `Scheduler`：`Tick(reviews, emotion)` 评估一轮、`SetEnabled`/`SetSourceEnabled` 开关、`Snapshot`/`Restore` 持久化。
- 三类触发源：`MemoryReviewSource`（记忆到期）、`ScheduleSource`（日程）、`EmotionSource`（情绪阈值）。
- 三重闸门：同轮去重 + 频率限流（默认 3/时、8/天）+ 静默时段（默认 23:00–07:00）。

## 十、视觉（`kernel/visual`）

- `Service` + `GenerateRequest`/`GenerateResult`；`ImageBackend` 实现：`ComfyUIBackend`（ComfyUI）、`GenericImageBackend`。
- `ImageDialect`：多方言自适应（`DialectOpenAI`/`DialectSiliconFlow`/`DialectGeneric`/`DialectChatGemini`），吃 data/images/output/choices/markdown 多种响应。
- `Watermarker` + `HasAIWatermark`：**强制 AI 水印**（不可关）。
- `ModelCapability`：能力探测降级（非画图模型执行前报错、不支持垫图自动降级纯文生图）。
- `Gallery`/`SelfiePipeline`：图库与自拍管线。

## 十一、语音 / 搜索（`kernel/voice`、`kernel/search`）

- `voice`：`SttProvider`/`TtsProvider` 抽象 + `OpenAIClient`（Whisper/audio，BYOK）。
- `search`：`Provider`（`BraveProvider`/`MCPProvider`）+ `Router`（主备）+ `MemoryCache` + `TokenBucket`（限流）；多后端可插拔。

## 十二、备份 / 诊断 / 合规 / 多语言

- `backup`：`WriteArchive`/`ReadArchive`，zip + 逐文件 SHA-256，导出→清库→导入往返一致。
- `diag`：`Capture`/`Guard`/`GuardThen`，`Dump` **仅写本地、无网络上传能力**；`PrivacyBoundaryMarkdown` 生成隐私边界说明。
- `compliance`：`ContentPolicy`/`Flavor`（国内外 build 默认策略不同）。
- `i18n`：`Bundle`/`New`/`NewFrom`，界面文案与模型语种解耦。

## 十三、安全传输（`kernel/netx`）

`Client(timeout)` 统一出站 + `RedirectPolicy`：**禁止跟随重定向**（红线 3/7，防 key 跨域外泄）。

## 十四、C 热点（`native/`）

`Index`/`Hit`（向量检索，int8/NEON）；`CPUFeatures`/`AvailMemBytes`（设备探测）。`kernel/native` 提供 Go 侧封装（`NewIndex`/`Deserialize`）。

## 十五、双端桥（`mobilebind` / `desktopbind`）

- `mobilebind`（gomobile → Android AAR，**71 个导出函数**）：`ChatStream`/`ChatStreamTools`/`ChatStreamImage`、`CreateCharacter`/`ListCharacters`、`AppendMessage`、`BusPublish`/`BusRecent`、`BackupExport`/`BackupImport`、`ComplianceFlavor`、`DeviceProfile` 等。
- `desktopbind`（c-shared DLL + JNI，`Sr*`）：`SrInitKernel`/`SrChatOnce`/`SrCreateCharacter`/`SrListCharacters`/`SrListMessages`/`SrVersion`/`SrDeviceProfile` 等。

## 十六、事件总线与核心契约（`bus` / `core`）

- `bus`：`Bus`/`Subscription` 门铃广播（事件只带 topic/版本，订阅者收铃拉快照；有界缓冲、丢最旧、发布永不阻塞）。
- `core`：六类型 `Message`/`ContentBlock`/`ToolCall`/`Event`/`StreamEvent`/`Role` + `Clock`（真实/假时钟，便于确定性测试）。

## 十七、命令（`cmd/`）

- `srchat`：最小 CLI 对话闭环（七层最小版 + 流式；key 只从文件/环境变量读）。
- `livecheck`：真机级冒烟（selftest 水印 / chat / image / all）。
- `stresssim`：年尺度压力模拟。

---

## 一张图看依赖方向（分层约束）

```
应用层（你的 UI）
  └─ mobilebind / desktopbind（AAR / DLL+JNI）
      └─ 内核底座：core · bus · adapter · orchestrator · store · secret · tool · tooldisc
                   · cards · visual · voice · search · backup · diag · compliance · i18n · netx
          └─ 可插拔能力包：hippocampus(记忆) · endocrine(情绪) · proactive(主动)
              └─ C 热点（native/vector, device_probe）
```
底座包**不 import** 能力包（`scripts/check-open-split.sh` 守卫），底座可单独编译。
