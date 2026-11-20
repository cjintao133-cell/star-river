package com.starriver.app

import android.content.Context
import org.json.JSONObject

/**
 * UI i18n 加载器：字符串真源 = kernel/i18n/{zh,en}.json（经 scripts/sync-i18n-assets.sh 同步到 assets）。
 * UI 侧不另抄文案；t() 走 lang → zh 回退 → key 三级回退，与 kernel/i18n 的 Bundle 语义一致。
 */
object AppI18n {
    @Volatile private var langCode = "zh"
    @Volatile private var table: JSONObject = JSONObject()
    @Volatile private var fallback: JSONObject = JSONObject()

    fun load(ctx: Context, lang: String) {
        val code = if (lang == "en") "en" else "zh"
        if (code != langCode || table.length() == 0) {
            langCode = code
            table = readAsset(ctx, "i18n/$code.json")
            fallback = if (code == "zh") table else readAsset(ctx, "i18n/zh.json")
        }
    }

    private fun readAsset(ctx: Context, path: String): JSONObject = try {
        JSONObject(ctx.assets.open(path).bufferedReader().use { it.readText() })
    } catch (t: Throwable) { JSONObject() }

    fun t(key: String): String {
        val v = table.optString(key, "")
        if (v.isNotEmpty()) return v
        val f = fallback.optString(key, "")
        return if (f.isNotEmpty()) f else key
    }

    fun current(): String = langCode
}
