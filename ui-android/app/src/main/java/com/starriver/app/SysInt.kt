package com.starriver.app

import android.content.Context
import mobilebind.Mobilebind
import org.json.JSONObject
import java.util.Locale

/**
 * 系统集成小工具：系统语言自动检测 / 设备策略建议 / 应用标识请求头。
 * 对应内核 C 探针的应用侧消费（deviceStrategy）与请求头（setRequestHeaders）。
 */
object LocaleHelp {
    /** 系统语言映射到内置语言（en / zh；其余默认 zh）。 */
    fun systemLang(): String = if (Locale.getDefault().language == "en") "en" else "zh"

    fun followSystem(ctx: Context): Boolean = SysPrefs.followSystem(ctx)

    /** 实际生效语言：跟随系统→系统语言；否则用存储值。 */
    fun effective(ctx: Context): String =
        if (followSystem(ctx)) systemLang() else LangPrefs.lang(ctx)
}

object DeviceAdvice {
    data class Advice(val tier: String, val suggestReduceMotion: Boolean, val animation: Boolean, val maxHistory: Int)

    /** 读内核设备策略；失败给中性默认（不降级、不崩）。 */
    fun recommend(): Advice = try {
        val o = JSONObject(Mobilebind.deviceStrategy())
        val pol = o.optJSONObject("policy")
        Advice(
            tier = o.optString("tier", "high"),
            suggestReduceMotion = pol?.optBoolean("suggest_reduce_motion", false) ?: false,
            animation = pol?.optBoolean("animation", true) ?: true,
            maxHistory = pol?.optInt("max_history", 24) ?: 24,
        )
    } catch (t: Throwable) {
        Advice("high", false, true, 24)
    }
}

/** 应用标识请求头：默认内核恒带 X-Star-River-Client；此处补用户自定义 tag。 */
object HeaderHelp {
    fun apply(ctx: Context) {
        val tag = SysPrefs.personalTag(ctx).trim()
        val json = if (tag.isEmpty()) "" else "{\"X-Star-River-Tag\":\"${tag.replace("\"", "\\\"")}\"}"
        try { Mobilebind.setRequestHeaders(json) } catch (t: Throwable) { }
    }
}

/** 系统集成偏好（语言跟随 / 个人标识 tag / 减动态是否用户已手动设）。 */
object SysPrefs {
    private const val PREF = "star_river_prefs"
    private fun p(ctx: Context) = ctx.getSharedPreferences(PREF, Context.MODE_PRIVATE)

    fun followSystem(ctx: Context): Boolean = p(ctx).getBoolean("ui.lang_follow_system", true)
    fun setFollowSystem(ctx: Context, v: Boolean) = p(ctx).edit().putBoolean("ui.lang_follow_system", v).apply()

    fun personalTag(ctx: Context): String = p(ctx).getString("ui.personal_tag", "") ?: ""
    fun setPersonalTag(ctx: Context, v: String) = p(ctx).edit().putString("ui.personal_tag", v).apply()

    fun reduceMotionSet(ctx: Context): Boolean = p(ctx).getBoolean("ui.reduce_motion_set", false)
    fun markReduceMotionSet(ctx: Context) = p(ctx).edit().putBoolean("ui.reduce_motion_set", true).apply()
}
