package com.starriver.app

import android.content.Context
import android.os.Handler
import android.os.Looper
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import mobilebind.Mobilebind
import org.json.JSONArray

/** 后台执行桥调用并把结果切回主线程。桥函数可能耗时（DB/网络），一律离开 UI 线程。 */
object Bg {
    private val h = Handler(Looper.getMainLooper())
    fun run(work: () -> String, done: (String) -> Unit) {
        Thread {
            val r = try {
                work()
            } catch (t: Throwable) {
                "ERR:" + (t.message ?: "?")
            }
            h.post { done(r) }
        }.start()
    }
    fun main(fn: () -> Unit) { h.post(fn) }
    /** 通用异步：work 在后台线程跑（可返回任意值/网络/DB），done 回主线程。异常→null。 */
    fun async(work: () -> Any?, done: (Any?) -> Unit) {
        Thread { val r = try { work() } catch (t: Throwable) { null }; h.post { done(r) } }.start()
    }
}

/** 会话（对应 sessions 表一行）。 */
data class SessionUi(val id: String, val title: String, val model: String, val updatedAt: Long)

/** UI 一条消息（不可变；流式靠替换列表元素触发重组）。 */
data class MsgUi(val id: String, val role: String, val text: String, val streaming: Boolean = false)

/** 角色（characterDTO 子集，够表单用）。 */
data class CharUi(
    val id: String,
    val name: String,
    val description: String = "",
    val personality: String = "",
    val scenario: String = "",
    val firstMes: String = "",
    val alternateGreetings: List<String> = emptyList(),
    val mesExample: String = "",
    val systemPrompt: String = "",
    val postHistory: String = "",
    val characterBook: String = "",
)

fun parseChars(raw: String): List<CharUi> {
    val out = mutableListOf<CharUi>()
    if (raw.startsWith("ERR:")) return out
    val arr = try { JSONArray(raw) } catch (e: Exception) { JSONArray() }
    for (i in 0 until arr.length()) {
        val o = arr.optJSONObject(i) ?: continue
        val alt = mutableListOf<String>()
        val ag = o.optJSONArray("alternate_greetings")
        if (ag != null) for (j in 0 until ag.length()) alt.add(ag.optString(j))
        out.add(
            CharUi(
                id = o.optString("id"),
                name = o.optString("name"),
                description = o.optString("description"),
                personality = o.optString("personality"),
                scenario = o.optString("scenario"),
                firstMes = o.optString("first_mes"),
                alternateGreetings = alt,
                mesExample = o.optString("mes_example"),
                systemPrompt = o.optString("system_prompt"),
                postHistory = o.optString("post_history_instructions"),
                characterBook = o.optString("character_book"),
            )
        )
    }
    return out
}

fun loadMessages(sid: String): List<MsgUi> {
    val raw = try { Mobilebind.listMessages(sid, 200L) } catch (t: Throwable) { "ERR:" + (t.message ?: "?") }
    val out = mutableListOf<MsgUi>()
    if (raw.startsWith("ERR:")) return out
    val arr = try { JSONArray(raw) } catch (e: Exception) { JSONArray() }
    for (i in 0 until arr.length()) {
        val o = arr.optJSONObject(i) ?: continue
        out.add(MsgUi(o.optString("id"), o.optString("role"), o.optString("text"), false))
    }
    return out
}

/** 由角色字段拼 system 提示（点角色新建会话时用，把「人设」真正喂给模型）。 */
fun buildCharacterSystem(c: CharUi): String {
    val sb = StringBuilder()
    sb.append("你现在扮演角色「").append(c.name).append("」。")
    if (c.description.isNotBlank()) sb.append("\n设定：").append(c.description)
    if (c.personality.isNotBlank()) sb.append("\n性格：").append(c.personality)
    if (c.scenario.isNotBlank()) sb.append("\n场景：").append(c.scenario)
    if (c.systemPrompt.isNotBlank()) sb.append("\n").append(c.systemPrompt)
    if (c.postHistory.isNotBlank()) sb.append("\n").append(c.postHistory)
    return sb.toString().trim()
}

/** 应用级共享状态：会话列表 + 当前会话 + 每会话的 system 覆盖。 */
class AppState {
    val sessions = mutableStateListOf<SessionUi>()
    var currentSessionId by mutableStateOf<String?>(null)
    var loading by mutableStateOf(false)
    val sessionSystem = mutableStateMapOf<String, String>()

    fun reload() {
        loading = true
        val raw = try { Mobilebind.listSessions() } catch (t: Throwable) { "ERR:" + (t.message ?: "?") }
        val list = mutableListOf<SessionUi>()
        if (!raw.startsWith("ERR:")) {
            val arr = try { JSONArray(raw) } catch (e: Exception) { JSONArray() }
            for (i in 0 until arr.length()) {
                val o = arr.optJSONObject(i) ?: continue
                list.add(SessionUi(o.optString("id"), o.optString("title"), o.optString("model"), o.optLong("updated_at")))
            }
        }
        sessions.clear(); sessions.addAll(list)
        loading = false
    }

    /** 为角色新建会话并切过去。charId 可空（=普通新对话）。返回 session id 或 null。 */
    fun newSession(charId: String, title: String, model: String, system: String?): String? {
        val raw = try { Mobilebind.createSession(charId, title, model) } catch (t: Throwable) { "ERR:" + (t.message ?: "?") }
        if (raw.startsWith("ERR:")) return null
        val id = try { org.json.JSONObject(raw).optString("session_id") } catch (e: Exception) { "" }
        if (id.isEmpty()) return null
        if (!system.isNullOrBlank()) sessionSystem[id] = system
        reload()
        currentSessionId = id
        return id
    }

    fun delete(sid: String) {
        try { Mobilebind.deleteSession(sid) } catch (t: Throwable) { }
        if (currentSessionId == sid) currentSessionId = null
        reload()
    }
}

/** 非敏感配置（搜索源 / 语音），SharedPreferences 存；key 走 SecretStore 另行加密。 */
object Prefs {
    private const val PREF = "star_river_prefs"
    private fun p(ctx: Context) = ctx.getSharedPreferences(PREF, Context.MODE_PRIVATE)

    fun searchProvider(ctx: Context): String = p(ctx).getString("search.provider", "searxng") ?: "searxng"
    fun setSearchProvider(ctx: Context, v: String) = p(ctx).edit().putString("search.provider", v).apply()

    fun searxngBase(ctx: Context): String = p(ctx).getString("search.searxng", "") ?: ""
    fun setSearxngBase(ctx: Context, v: String) = p(ctx).edit().putString("search.searxng", v).apply()

    fun ttsVoice(ctx: Context): String = p(ctx).getString("tts.voice", "alloy") ?: "alloy"
    fun setTtsVoice(ctx: Context, v: String) = p(ctx).edit().putString("tts.voice", v).apply()

    fun ttsFormat(ctx: Context): String = p(ctx).getString("tts.format", "mp3") ?: "mp3"
    fun setTtsFormat(ctx: Context, v: String) = p(ctx).edit().putString("tts.format", v).apply()

    fun nativeSearch(ctx: Context): Boolean = p(ctx).getBoolean("search.native", true)
    fun setNativeSearch(ctx: Context, v: Boolean) = p(ctx).edit().putBoolean("search.native", v).apply()
}

/** 主题 / 减少动效 持久化（阶段 C）。追加于 Prefs 之外，独立 object 避免改原闭括号。 */
object UiPrefs {
    private const val PREF = "star_river_prefs"
    private fun p(ctx: Context) = ctx.getSharedPreferences(PREF, Context.MODE_PRIVATE)
    fun themeMode(ctx: Context): ThemeMode =
        if (p(ctx).getInt("ui.theme", 0) == 1) ThemeMode.NIGHT else ThemeMode.DAY
    fun setThemeMode(ctx: Context, m: ThemeMode) =
        p(ctx).edit().putInt("ui.theme", if (m == ThemeMode.NIGHT) 1 else 0).apply()
    fun reduceMotion(ctx: Context): Boolean = p(ctx).getBoolean("ui.reduce_motion", false)
    fun setReduceMotion(ctx: Context, v: Boolean) = p(ctx).edit().putBoolean("ui.reduce_motion", v).apply()
}

/** 语言偏好（阶段 C，语言切换 UI）。 */
object LangPrefs {
    private const val PREF = "star_river_prefs"
    private fun p(ctx: Context) = ctx.getSharedPreferences(PREF, Context.MODE_PRIVATE)
    fun lang(ctx: Context): String = p(ctx).getString("ui.lang", "zh") ?: "zh"
    fun setLang(ctx: Context, v: String) = p(ctx).edit().putString("ui.lang", v).apply()
}

/** 主题 v2 偏好（自定义强调色 + 圆角档）。 */
object ThemePrefs {
    private const val PREF = "star_river_prefs"
    private fun p(ctx: Context) = ctx.getSharedPreferences(PREF, Context.MODE_PRIVATE)
    fun load(ctx: Context): ThemeConfig {
        val raw = p(ctx).getInt("ui.accent", Int.MIN_VALUE)
        val accent = if (raw == Int.MIN_VALUE) null else raw
        val mode = if (p(ctx).getInt("ui.theme", 0) == 1) ThemeMode.NIGHT else ThemeMode.DAY
        return ThemeConfig(mode = mode, accent = accent, radiusDp = p(ctx).getInt("ui.radius", 16))
    }
    fun save(ctx: Context, cfg: ThemeConfig) {
        val ed = p(ctx).edit()
            .putInt("ui.theme", if (cfg.mode == ThemeMode.NIGHT) 1 else 0)
            .putInt("ui.radius", cfg.radiusDp)
        if (cfg.accent == null) ed.remove("ui.accent") else ed.putInt("ui.accent", cfg.accent)
        ed.apply()
    }
}

/** 系统安全与自愈板块偏好。 */
object SafetyPrefs {
    private const val PREF = "star_river_prefs"
    private fun p(ctx: Context) = ctx.getSharedPreferences(PREF, Context.MODE_PRIVATE)
    fun watchdog(ctx: Context): Boolean = p(ctx).getBoolean("safety.watchdog", true)
    fun setWatchdog(ctx: Context, v: Boolean) = p(ctx).edit().putBoolean("safety.watchdog", v).apply()
    fun sandboxStrict(ctx: Context): Boolean = p(ctx).getBoolean("safety.sandbox_strict", true)
    fun setSandboxStrict(ctx: Context, v: Boolean) = p(ctx).edit().putBoolean("safety.sandbox_strict", v).apply()
}

/** 应用级 Context（社区拉取等无 Compose 作用域处使用；MainActivity.onCreate 设置）。 */
object AppCtx { lateinit var ctx: Context }

/** 社区源 URL 偏好（Git 仓库 raw / jsDelivr / Pages / 自建 API，同一契约）。 */
object CommunityPrefs {
    private const val PREF = "star_river_prefs"
    private fun p(ctx: Context) = ctx.getSharedPreferences(PREF, Context.MODE_PRIVATE)
    fun base(ctx: Context): String = p(ctx).getString("community.base", "") ?: ""
    fun setBase(ctx: Context, v: String) = p(ctx).edit().putString("community.base", v).apply()
}

/** 画图模型配置（相册/自拍用）：base/model 明文存；apiKey 走 SecretStore 加密。 */
object ImagePrefs {
    private const val PREF = "star_river_prefs"
    private const val KEY_ID = "image.api_key"
    private fun p(ctx: Context) = ctx.getSharedPreferences(PREF, Context.MODE_PRIVATE)
    fun base(ctx: Context): String = p(ctx).getString("image.base", "") ?: ""
    fun setBase(ctx: Context, v: String) = p(ctx).edit().putString("image.base", v).apply()
    fun model(ctx: Context): String = p(ctx).getString("image.model", "") ?: ""
    fun setModel(ctx: Context, v: String) = p(ctx).edit().putString("image.model", v).apply()
    fun editPath(ctx: Context): String = p(ctx).getString("image.edit_path", "") ?: ""
    fun setEditPath(ctx: Context, v: String) = p(ctx).edit().putString("image.edit_path", v).apply()
    fun apiKey(ctx: Context): String = SecretStore.get(ctx, KEY_ID) ?: ""
    fun setApiKey(ctx: Context, v: String) { if (v.isNotBlank()) SecretStore.put(ctx, KEY_ID, v) }
    fun configured(ctx: Context): Boolean = base(ctx).isNotBlank() && model(ctx).isNotBlank()
}

/** 聊天协议 provider 偏好（openai/anthropic/gemini）。 */
object ProtoPrefs {
    private const val PREF = "star_river_prefs"
    private fun p(ctx: Context) = ctx.getSharedPreferences(PREF, Context.MODE_PRIVATE)
    fun provider(ctx: Context): String = p(ctx).getString("chat.provider", "openai") ?: "openai"
    fun setProvider(ctx: Context, v: String) = p(ctx).edit().putString("chat.provider", v).apply()
    fun apply(ctx: Context) { try { Mobilebind.setChatProvider(provider(ctx)) } catch (t: Throwable) { } }
}
