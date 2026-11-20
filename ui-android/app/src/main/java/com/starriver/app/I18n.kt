package com.starriver.app

import android.content.Context
import mobilebind.Mobilebind

/**
 * UI i18n：直接查内核 go:embed 的 zh.json/en.json（单源真源，经 I18nT 桥），
 * Kotlin 侧不再另存 assets。回退链由内核 Bundle 负责（lang→默认zh→key）。
 */
object AppI18n {
    @Volatile private var lang = "zh"

    fun load(ctx: Context, lang: String) {
        this.lang = if (lang == "en") "en" else "zh"
    }

    fun t(key: String): String = try {
        Mobilebind.i18nT(lang, key)
    } catch (t: Throwable) {
        key
    }

    fun current(): String = lang
}
