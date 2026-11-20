package com.starriver.app

import android.content.Context

/**
 * 聊天配置（端点 / key / 模型 / 人设）。
 *
 * 红线 4：key 不再明文存 SharedPreferences，改为经 SecretStore（Android Keystore
 * AES/GCM）加密持有。SharedPreferences 仅存非敏感的 base/model/system。
 * Config 数据类里的 apiKey 为运行时明文（仅内存），存取由 ConfigStore 走 SecretStore。
 */
data class ChatConfig(
    val baseUrl: String,
    val apiKey: String,
    val model: String,
    val system: String,
) {
    val isConfigured: Boolean get() = baseUrl.isNotBlank() && apiKey.isNotBlank() && model.isNotBlank()
}

object ConfigStore {
    private const val PREF = "star_river_cfg"
    private const val K_BASE = "base_url"
    private const val K_MODEL = "model"
    private const val K_SYSTEM = "system"
    private const val SECRET_KEY_ID = "endpoint.api_key"

    // 不再内置任何厂商/模型/人设：BYOK，全部由用户填写（避免预设错误端点或夹带私人设定）。
    private const val DEFAULT_BASE = ""
    private const val DEFAULT_MODEL = ""
    private const val DEFAULT_SYSTEM = ""

    fun load(ctx: Context): ChatConfig {
        val p = ctx.getSharedPreferences(PREF, Context.MODE_PRIVATE)
        return ChatConfig(
            baseUrl = p.getString(K_BASE, DEFAULT_BASE) ?: DEFAULT_BASE,
            apiKey = SecretStore.get(ctx, SECRET_KEY_ID) ?: "",
            model = p.getString(K_MODEL, DEFAULT_MODEL) ?: DEFAULT_MODEL,
            system = p.getString(K_SYSTEM, DEFAULT_SYSTEM) ?: DEFAULT_SYSTEM,
        )
    }

    fun save(ctx: Context, cfg: ChatConfig) {
        ctx.getSharedPreferences(PREF, Context.MODE_PRIVATE).edit()
            .putString(K_BASE, cfg.baseUrl)
            .putString(K_MODEL, cfg.model)
            .putString(K_SYSTEM, cfg.system)
            .apply()
        // key 走 Keystore 加密；空 key 时不覆盖已有。
        if (cfg.apiKey.isNotBlank()) {
            SecretStore.put(ctx, SECRET_KEY_ID, cfg.apiKey)
        }
    }

    fun defaults(): ChatConfig = ChatConfig(DEFAULT_BASE, "", DEFAULT_MODEL, DEFAULT_SYSTEM)
}
