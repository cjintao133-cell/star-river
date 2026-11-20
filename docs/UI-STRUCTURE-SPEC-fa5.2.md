# Star River · UI 结构规格书（交 fa5.2 照此实现）

> 用途：fa5.2 依本文件**重写 Android Compose UI**。数据层/桥层已经现成，**直接复用下面的类与函数，不要重造**。
> 交付方式：fa5.2 产出 `.kt` 文件 → 当前 AI（kernel/桥负责人）**搬入仓库、编译、修接缝、出 APK**。
> 配套：`docs/UI-HANDOFF-fa5.2.md` 里有逐条 bug 坐标。

---

## 1. 边界与角色
- **你只写** `ui-android/app/src/main/java/com/starriver/app/` 下的 Kotlin/Compose。
- **不要动** `kernel/`、`kernel/mobilebind/`、`ui-android/app/libs/mobilebind.aar`（内核桥，谁动谁重编 AAR）。
- **不要改** `AppShared.kt` 里的数据模型/桥封装语义（可读、可调用；如需新增纯 UI 工具函数可加，但别改已有签名）。
- 交付契约见 §10。

---

## 2. 技术栈与构建
- Kotlin + Jetpack Compose (Material3)，单 Activity。
- 构建：`cd ui-android && ./gradlew assembleDebug --no-daemon` → `ui-android/app/build/outputs/apk/debug/app-debug.apk`
- 只出 arm64-v8a。`applicationId=com.starriver.app`。
- **不引入第三方 UI/图标/网络库**（除非用户批准）。

---

## 3. 数据层（现成，直接调用）

### 3.1 异步（必须用，别主线程调桥）
```kotlin
object Bg {
  fun run(work: () -> String, done: (String) -> Unit)   // work 在后台线程；done 回主线程；异常→"ERR:msg"
  fun main(fn: () -> Unit)
  fun async(work: () -> Any?, done: (Any?) -> Unit)     // 任意返回，异常→null
}
```

### 3.2 数据模型（在 AppShared.kt）
```kotlin
data class SessionUi(val id:String, val title:String, val model:String, val updatedAt:Long)
data class MsgUi(val id:String, val role:String, val text:String, val streaming:Boolean=false)
data class CharUi(id,name,description,personality,scenario,firstMes,alternateGreetings:List<String>,mesExample,systemPrompt,postHistory,characterBook)
fun parseChars(raw:String): List<CharUi>            // 解析 ListCharacters() 的 JSON
fun loadMessages(sid:String): List<MsgUi>           // 内部已调 listMessages
fun buildCharacterSystem(c:CharUi): String          // 把角色人设拼成 system 提示（点角色新建会话用这个）
```

### 3.3 应用状态（全局单例，`remember { AppState() }` 持有）
```kotlin
class AppState {
  val sessions: SnapshotStateList<SessionUi>        // 会话列表
  var currentSessionId: String?                     // 当前会话
  var loading: Boolean
  val sessionSystem: SnapshotStateMap<String,String>// 每会话的 system(人设)覆盖：key=sessionId
  fun reload()                                      // 重新拉 listSessions
  fun newSession(charId:String, title:String, model:String, system:String?): String?  // 建会话并切过去
  fun delete(sid:String)
}
```

### 3.4 配置（BYOK）
```kotlin
data class ChatConfig(val baseUrl:String, val apiKey:String, val model:String, val system:String)
object ConfigStore {
  fun load(ctx): ChatConfig
  fun save(ctx, cfg)
  fun defaults(): ChatConfig   // 全空（端点/模型/人设默认空，用户自填）
}
```
> **注意**：`ChatConfig.system` 是"无角色普通新对话"的兜底 system。**连接配置页只该放 baseUrl/apiKey/model**，人设不属于这里（见 §8-6）。

### 3.5 偏好持久化（都现成）
- `Prefs(ctx)`: searchProvider/searxngBase/ttsVoice/ttsFormat/nativeSearch（getter+setter）
- `UiPrefs(ctx)`: themeMode/reduceMotion
- `LangPrefs(ctx)`: lang
- `ThemePrefs(ctx)`: 主题v2（mode/accent/radiusDp，见 Theme.kt）

### 3.6 主题令牌（别硬编码颜色/圆角）
```kotlin
enum class ThemeMode { DAY, NIGHT }
data class ThemeConfig(mode=DAY, accent:Int?=null, radiusDp:Int=16)
val LocalCornerRadius: Dp   // 组件圆角一律读它：RoundedCornerShape(LocalCornerRadius.current)
fun AppTheme(config, content)               // 包在根
val RadiusPresets: List<Pair<Int,String>>   // 8紧凑/16标准/24宽松
val AccentPresets: List<Pair<Int?,String>>  // null默认/暖橙/砖红/青蓝/星紫/青绿/樱粉
```
日间=暖橙+卡其底；夜间=青蓝+深空底（AppTheme 已实现，颜色走 `MaterialTheme.colorScheme.*`，不要写死色值）。

### 3.7 图标（AppIcons.kt，Canvas 手绘线性）
已有：`IconMic IconCamera IconPhoto IconSend IconStop IconBack IconTrash`。
**需要你补齐**：`IconSearch IconMessage IconContact IconCommunity IconSettings` 等（同风格：Canvas 线条、strokeWidth 统一、`Modifier.size()/tint` 参数化）。**禁止 emoji、禁止 Material Icons 库**。

---

## 4. 桥接口 `Mobilebind.*`（返回值格式）

> 调用一律 `Bg.run({ Mobilebind.xxx(...) }) { raw -> ... }`。返回多为 `String`：JSON / `"OK"` / `"ERR:..."`。

**内核/能力**
- `InitKernel(dbPath, cipherKeyB64):String`（启动已由 KernelBoot.ensure 调，UI 一般不直接调）
- `Version():String` · `Echo(s):String` · `DeviceProfile():String(JSON)` · `DeviceStrategy():String`

**端点/模型**
- `ListModels(base,key):String` → JSON `[{"id":"..."}]` 或 `"ERR:..."`
- `ProbeEndpoint(base,key,model):String` → `{"ok":bool,"category":"...","error":"...","model":"..."}`
- `ProbeModel(base,key,model):String`
- `SetChatProvider(name)` / `ChatProvider()` · `SetRequestHeaders(json)`

**聊天**
- `ChatStream(base,key,model,system,user, sink:ChatSink)` — openai-only、无历史（内部 BuildMinimal）
- `ChatStreamTools(base,key,model,system,user, sink:ChatSink)` — **全链路（多协议+工具循环+记忆注入），聊天主路径用这个**
- `ChatStreamImage(base,key,model,system,user,imageB64,imageMIME, sink)`
- `ChatSink` 接口：`OnDelta(text)` / `OnDone(model)` / `OnError(msg)`（Kotlin 实现，回调可能在后台线程→回抛到 UI 线程再改 state）

**会话/角色**
- `ListSessions()` · `CreateSession(charId,title,model)` → `{"session_id":...}` · `DeleteSession(sid)`
- `ListMessages(sid, limit:Long)` → `[{"id","role","text","created_at"}]` · `AppendMessage(sid,role,text)`
- `ListCharacters()` · `GetCharacter(id)` · `CreateCharacter(json)` · `UpdateCharacter(json)` · `DeleteCharacter(id)`
- `SetCharacterAvatar(id, bytes)` · `SetCharacterPersona(id, bytes)` · `ParseCharacterCard(bytes):JSON`

**记忆/情绪/进化**
- `MoodStateJSON()` → `{valence,arousal,intimacy,fatigue,curiosity}` · `MemoryHitsJSON(limit)` → `[{id,text,score,retention,importance}]`
- `DevPanels()` · `EvolutionStats()`（含 memoryCount/memCap/mood/eventCounts/recent/boundaries）

**视觉/自拍**
- `VisualInit(cfgJson)`（cfg: `base_url,api_key,model,backend=openai-image|siliconflow|generic,dialect=openai|siliconflow|chat|generic,header`）
- `VisualStatus()` · `VisualCapabilities()`→`{configured,model,backend,dialect,canImage,canImg2Img}`
- `VisualScenes()` → `[{key,prompt}]` · `VisualGenerateImage(prompt,w,h)` · `VisualSelfie(cid,sceneKey)`
- 返回图：`{image_b64,mime,degraded,note}`（**已带 AI 水印，别去**；degraded=true 时展示 note）

**搜索/语音/MCP/备份/合规/诊断/i18n**
- `WebSearch(provider,base,key,query,count,recency)` · `SttTranscribe(...)` · `TtsSynthesize(...)`
- `McpInit/AddServer/RemoveServer/SetEnabled/Restore/Servers/Tools`
- `SetBackupEndpoint/BackupExport/BackupImport/BackupStatus`
- `ComplianceFlavor/Policy` · `DiagCrashDump/ListDumps/ReadDump/PrivacyBoundary`
- `I18nT(key)/I18nHas/I18nLanguages` · `Proactive*` · `BusPublish/BusRecent`

---

## 5. i18n（单源，别硬编码中文）
现有 key（36）：`common.ok/cancel/save/delete/close/retry`、`chat.send/input_placeholder/new_session/stop`、`settings.title/language/endpoint/apikey/model/theme/privacy`、`memory.title/review_due`、`mood.title`、`error.network/timeout/auth/rate_limit/server/protocol`、`visual.generate/generating`、`proactive.title`、`nav.messages/contacts/community/settings`、`settings.tier.novice/advanced/developer`。
- 用法：`AppI18n.t("key")`（`kernel/i18n/zh.json|en.json` 为真源）。**新增文案 key 要同时补 zh/en**（真源在 kernel/i18n，需 kernel 侧配合——通过当前 AI 加）。
- 缺的 key 先列出来，别把中文写死进 Compose。

---

## 6. 屏幕结构（导航语义）
根：`MainActivity` → `Column { Box(weight1){ 当前tab } ; BottomBar(4 tab) }`。
- **Tab0 消息**（`MessageTab`）：会话列表 / 空态 / 进入某会话聊天（流式气泡、输入框、发送/停止、可开图片/语音）
- **Tab1 联系人**（`ContactsTab`）：角色列表 → 点角色用"角色人设"建会话；「创建角色 / 导入角色卡」两入口；角色表单（全字段+头像+人设图上传）
- **Tab2 社区**（`CommunityTab`）：市场/论坛（静态 JSON 后端）、MCP 热插拔管理
- **Tab3 设置**（`SettingsTab`）：三层（新手=端点/key/模型 | 进阶=主题/动效/搜索/语音 | 开发者=合规/诊断/面板），并进入子屏：
  - 权限、举报、安全·自愈、开发面板、诊断、相册自拍
子屏返回用 `screen` 状态切换（现有模式），或你换更规范的导航，但**保留所有入口与功能**。

---

## 7. 视觉铁律（用户明确要求，违反过）
1. **图标 Canvas 手绘线性，无 emoji、无第三方图标库**（当前有 6+ 处 emoji 违规，见 HANDOFF §5.3）。
2. **i18n 单源**，别硬编码中文。
3. **BYOK**：不预置任何厂商端点/模型/私人人设。
4. **生图强制水印**，UI 不能去掉。
5. **不造假数据**：面板/统计要真数据，无则标注。
6. **颜色/圆角走令牌**（MaterialTheme.colorScheme + LocalCornerRadius），别写死。

---

## 8. 系统层（黑框根因，务必处理）
- **当前顶栏是一坨黑**：`AndroidManifest.xml` 没设 `android:theme`、`res/` 无主题文件 → 平台默认 ActionBar(DarkActionBar) 画了黑条显示 `label="Star River"`。
- **处理**：新建 `res/values/themes.xml`（如 `Theme.Material3.DayNight.NoActionBar` 派生），`<application android:theme="@style/...">`；状态栏用 `enableEdgeToEdge()` / `WindowCompat` 透明并让内容延展到状态栏，或自绘与主题同色顶栏。**不要靠平台默认 ActionBar。**

---

## 9. 必改 bug 清单（细节见 UI-HANDOFF-fa5.2.md §5）
1. 顶部黑 ActionBar（§8 根因）
2. 设置页「校验/拉模型」点了无可见反馈（`note` 渲染在按钮下方被底部导航挡；改成上方/Toast+按钮进行中态+模型列表化）
3. 6+ 处 emoji 当图标（MainActivity/MessageTab/SafetyScreen/EvolutionPanel/SettingsTab/CommunityTab）
4. 默认配置已清空（别回退预设）
5. 空态死区缺视觉锚点
6. **人设错位**：设置页的「人设 system」输入框要移除；人设归角色（角色已有 systemPrompt/personality/description）；无角色新对话别把 system 塞回端点页

---

## 10. 交付契约（怎么交接）
- fa5.2 产出：改好/新增的 `.kt` 文件清单 + 一句变更摘要 + （可选）一张截图。
- **不要**直接改 `AppShared.kt` 的模型/桥封装语义；需要新数据字段或新桥函数 → 告诉当前 AI 加（kernel 侧）。
- 当前 AI 收到后：**搬入 `ui-android/.../app/` → `./gradlew assembleDebug` → 修接缝编译错误 → 出 APK → 交付**。
- 只交付 `.kt`（及 `res/values/themes.xml` 等必要资源）；**AAR/内核不动**。

---

## 11. 验收
- `assembleDebug` 通过、APK 可装、四栏+子屏都能进。
- §7 铁律全守、§9 bug 全清。
- 无第三方库新增（除用户批准）。
