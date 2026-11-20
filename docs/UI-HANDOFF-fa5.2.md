# UI 重做交接 · 致 fa5.2

> 本文件是交给 5.2 的 UI 重做说明。内核/桥逻辑是好的，问题集中在 Android Compose UI 层。
> 作者（当前 AI）已停手 UI，凡涉及视觉/交互一律以你（5.2）为准。下面只给**事实、坐标、坑**，不给风格指令（风格由你和用户定）。

---

## 0. 一句话
Star River 是一个**纯本地、零上传、BYOK 的双端「赛博生命容器」**：Go 内核（`kernel/`）负责记忆/情绪/协议/存储/视觉，Android 只是**展示 + 调桥**的门面。
**你只改 `ui-android/` 里的 Kotlin Compose**；不要在没重编 AAR 的情况下动内核。

---

## 1. 环境与构建
- 仓库：`/root/star-river`；UI 在 `ui-android/app/src/main/java/com/starriver/app/`（24 个 .kt，约 3400 行）。
- 构建 APK：
  ```
  cd /root/star-river/ui-android && ./gradlew assembleDebug --no-daemon
  ```
  产物：`ui-android/app/build/outputs/apk/debug/app-debug.apk`
- 内核桥 AAR 已就位：`ui-android/app/libs/mobilebind.aar`（**不用动**，除非改 `kernel/mobilebind/*.go`，那要先 `gomobile bind -target=android/arm64 -androidapi 21 -o build/mobilebind.aar ./kernel/mobilebind` 再 `cp` 进 libs/）。
- 设备：只出 **arm64-v8a**。当前 `applicationId=com.starriver.app`，`versionName=0.0.1-m0`。

---

## 2. 文件地图（都在上述目录）
导航/骨架：`MainActivity.kt`(127) 根导航+底部四栏；`Theme.kt`(88) 主题令牌；`AppShared.kt`(287) AppState/Bg/各种 Prefs；`KernelBoot.kt`(49) 开库；`I18n.kt`(24)+`AppIcons.kt`(106) 图标。
四栏：`MessageTab.kt`(378) 会话+聊天；`ContactsTab.kt`(272) 角色；`CommunityTab.kt`(269)+`Community.kt`(95)；`SettingsTab.kt`(378) 三层设置。
子屏：`AlbumScreen.kt`(170) 相册/自拍；`SafetyScreen.kt`(178) 安全自愈；`PanelsScreen.kt`(126)+`EvolutionPanel.kt`(121) 开发面板/自我进化；`DiagScreen.kt`(106)+`Diag.kt`；`PermissionScreen.kt`(140)。
其他：`ChatConfig.kt`(56) 端点配置；`SecretStore.kt`(82) Keystore；`Stt.kt` `Proactive.kt` `SysInt.kt` `MeteorBackground.kt`(167)。

---

## 3. 桥接口（Kotlin 侧 `Mobilebind.xxx(...)`，gomobile 生成）
> 返回大多是 `String`（JSON 或 `"OK"`/`"ERR:..."`）。**全部在后台线程调用**（见 Bg.run）。

**内核/配置**：`InitKernel(dbPath, cipherKeyB64)` `CloseKernel()` `Version()` `Echo(s)` `DeviceProfile()` `DeviceStrategy()`
**端点/模型**：`ListModels(baseUrl, apiKey)`→JSON数组`[{"id":...}]`或`ERR:` · `ProbeEndpoint(base,key,model)`→`{"ok":bool,"category","error","model"}` · `ProbeModel(...)` · `SetChatProvider(name)` `ChatProvider()` · `SetRequestHeaders(json)`
**聊天**：`ChatStream(base,key,model,system,user, ChatSink)`（openai-only，无历史）· `ChatStreamTools(...sink)`（**全链路：多协议+工具循环+记忆注入**，聊天主路径该用这个）· `ChatStreamImage(...)`
**会话/角色**：`ListSessions()` `CreateSession(cid,title,model)` `DeleteSession(id)` `ListMessages(sid,limit)` `AppendMessage(sid,role,text)` · `ListCharacters()` `GetCharacter(id)` `CreateCharacter(json)` `UpdateCharacter(json)` `DeleteCharacter(id)` `SetCharacterAvatar(id,bytes)` `SetCharacterPersona(id,bytes)` `ParseCharacterCard(bytes?)`
**记忆/情绪/进化**：`MoodStateJSON()` `MemoryHitsJSON(limit)` `DevPanels()` `EvolutionStats()` `TestAssembly*`
**视觉/自拍**：`VisualInit(cfgJson)`（cfg: base_url/api_key/model/backend=openai-image|siliconflow|generic/dialect=openai|siliconflow|chat|generic/header）`VisualStatus()` `VisualCapabilities()` `VisualScenes()` `VisualGenerateImage(prompt,w,h)` `VisualSelfie(cid,sceneKey)`（返回 `{image_b64,mime,degraded,note}`）
**搜索/语音/MCP/备份/合规/诊断/i18n**：`WebSearch(provider,base,key,query,count,recency)` `SttTranscribe(...)` `TtsSynthesize(...)` · `McpInit/AddServer/RemoveServer/SetEnabled/Restore/Servers/Tools` · `SetBackupEndpoint/BackupExport/BackupImport/BackupStatus` · `ComplianceFlavor/Policy` · `DiagCrashDump/ListDumps/ReadDump/PrivacyBoundary` · `I18nT(key)/I18nHas/I18nLanguages` · `Proactive*` `Bus*` `SetEmbeddingConfig/EmbeddingStatus`

---

## 4. 用户的铁律（违反过的，别再犯）
1. **图标一律 Canvas 手绘线性、无 emoji、无第三方图标库**。`AppIcons.kt` 已有 `IconMessage/IconContact/IconCamera/IconBack/IconSearch(如有请补)` 等可复用/扩展。**现在多处违规**（见 §5.3）。
2. **i18n 单源**：文案走 `AppI18n.t("key")`，中英 `kernel/i18n/zh.json|en.json`（资源同步脚本 `scripts/sync-i18n-assets.sh`）。别硬编码中文字符串到 Compose。
3. **BYOK、零上传**：不预置任何厂商端点/模型/私人人设（默认值必须为空，见 §5.4）。
4. **AI 生图强制水印**：自拍/生图返回的图已带水印，UI 别去掉。红线 10。
5. **别造假数据**：面板/统计要真数据；无数据就标注，不编。

---

## 5. 必须修的具体问题（带坐标 + 根因 + 建议）

### 5.1 顶部黑框（截图里最突兀）
- **根因**：`ui-android/app/src/main/AndroidManifest.xml` 的 `<application>`/`<activity>` **没有 `android:theme`**，`res/` 也没有 themes 文件 → 用平台默认主题（带 ActionBar，DarkActionBar），于是顶部画了一条黑 ActionBar 显示 `android:label="Star River"`，和米色 Compose 内容割裂。
- **建议**：给 app 一个 NoActionBar 主题（新建 `res/values/themes.xml`，如 `Theme.Material3.DayNight.NoActionBar` 的派生），在 manifest 上 `android:theme` 引用；然后状态栏用 `enableEdgeToEdge()`/`WindowCompat` 设透明并让内容延伸到状态栏，或自绘一个和主题同色的 Compose 顶栏。**当前没有任何 Compose TopAppBar**，"Star River"是系统的。

### 5.2 设置页「校验端点」「拉取模型」点了没反馈（重点 bug）
- **坐标**：`SettingsTab.kt` 的 `NoviceSettings`（约 95-140 行）。`probe()`/`listModels()` 把结果写到 `note`，而 `note` 的那行 `Text` **渲染在按钮 Row 之下、也就是滚动区最底部**，紧贴底部导航栏，基本看不见；且按钮点击瞬间除了 `busy=true` 没有"进行中"的可见提示。
- **根因**：结果输出位置在视觉盲区 + 无 toast/无按钮态变化。
- **建议**：① 把结果（成功/失败原因/模型列表）显示到**按钮上方或顶部固定区**，或直接 `Toast`/Snackbar；② 点击即禁用按钮并显示"进行中…"（`busy` 已存在，接上即可）；③ 拉回的模型 JSON 数组要**渲染成可选列表/胶囊**，不要原样打印 JSON。
- **注意（不是代码错）**：拉不出模型很可能是**用户 key 与 base 不同源**（硅基 base 配了星链 key → 401）。UI 要把 `ListModels` 的 `ERR:` 原因**显眼展示**，引导用户"base 与 key 需同一家"。

### 5.3 emoji 当图标（违反铁律 1）
- `MainActivity.kt:107-108` 底部导航 `"💬 "+...` / `"👤 "+...`
- `MessageTab.kt:77` `Text("🔍 搜索")`
- `SafetyScreen.kt:97/113/128/138/148` SectionCard 标题 `🐕 📦 🛡 🔧 🧬`
- `EvolutionPanel.kt:52` `🧬 自我进化…`
- `SettingsTab.kt:363` `🛡 系统·安全·自愈板块`
- `CommunityTab.kt:86` `💬`（回复数前缀，这个可为可去，但既然铁律禁 emoji，也换）
- **建议**：全部换成 `AppIcons.kt` 里的 Canvas 线性图标或新增同类；标题就不带 emoji。

### 5.4 默认配置（已修，确认别回退）
- `ChatConfig.kt` 的 `DEFAULT_BASE/DEFAULT_MODEL/DEFAULT_SYSTEM` 已**全部置空**（之前被写了硅基端点、假模型 `deepseek-ai/DeepSeek-V4.1-Flash`、以及"示例角色"人设——那些是之前的错误）。保持空，让用户自带。

### 5.6 「人设 system」错位在连接配置里（设计错误，必改）
- **现象**：设置·新手页（`SettingsTab.kt` 的 `NoviceSettings`）在「端点/API Key/模型 id」下面紧跟一个「人设 system」大输入框。**连接配置 ≠ 人设，不该放一起。**
- **现状机制**：`ChatConfig.system` 是全局兜底 system；`MessageTab.kt:79` 建"普通新对话"时传 `config.system`；`MessageTab.kt:123` 用 `sessionSystem[sid] ?: config.system`。
- **角色路径其实是好的**：`ContactsTab.kt:126-128` 点角色 → `buildCharacterSystem(c)` 取**角色自身的 systemPrompt/personality/description** → `newSession(c.id,...,sys)` 存进 `sessionSystem`。角色本来就自带人设（Go 侧 Character 有 `SystemPrompt/Personality/Description` 字段）。
- **正确目标架构**：
  1. 连接配置页只保留 **端点 base + API Key + 模型 id**（+provider 可选），**移除「人设 system」**。
  2. 人设归属**角色**（已在角色卡里，保留）。
  3. 无角色的"普通新对话"：要么不设 system（用模型默认），要么在**聊天页**给单个会话设人设，**绝不要放回端点/key 配置页**。
  4. 删掉/改 `ChatConfig.system` 这个"全局人设"的定位（可保留字段做会话级，但不要作为设置页输入）。
- **注**：`ChatConfig` 默认值已全空（§5.4），别再加任何预设人设。

### 5.5 空状态 / 视觉节奏
- `MessageTab` 空态一大片米色死区、仅一行灰字；无插画/无锚点。建议补 Canvas 线性小插画 + 引导 + 主按钮。整体留白/字号层级需要你重排。

---

## 6. 代码层须知（别踩）
- **异步统一 `Bg.run(work:{...}, done:{...})`**（`AppShared.kt`）：work 在后台线程、done 回主线程；异常被吞成 `"ERR:"+msg`。**不要在主线程直接调 `Mobilebind.*`**（网络/DB 会 ANR）。
- **状态刷新**：拨号 `AppState.reload()` 后 UI 才更新（见 `MainActivity` 的 `LaunchedEffect`）。
- **主题令牌**：`Theme.kt` 的调色板/圆角（`LocalCornerRadius.current` 等）是数据驱动（`ThemePrefs`），改视觉尽量走令牌，别散写颜色。
- **导航**：`MainActivity` 用 `tab` 0..3；子屏通过 `SettingsTab` 内部 `screen` 切换。重做导航时保留路由语义。
- **别删桥调用**：UI 里的 `Mobilebind.xxx` 都是真接口，删了功能就没了。

---

## 7. 交付验收
- `./gradlew assembleDebug` 成功、APK 可装。
- 上面 §5 五类问题逐条自检。
- 不引入第三方 UI/图标库（除非用户批准）。
- 改完写一句变更摘要，随 APK 一起回。

有任何桥返回格式/字段含义不清，问当前 AI（他负责 kernel/mobilebind，可给出精确返回样例）。
