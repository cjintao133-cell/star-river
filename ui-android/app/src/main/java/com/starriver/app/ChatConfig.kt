package com.starriver.app

import android.content.Context

/**
 * 聊天配置（端点 / key / 模型 / 人设）。
 *
 * 注意（红线 4 诚实边界）：T-M2-04 阶段 key 暂存 SharedPreferences（明文）。
 * T-M2-05（设置页 v1）会改为经 Go secret.Vault（Android Keystore）加密持有。
 * 本 class 只做本机存取，不上报、不打日志。
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
    private const val K_KEY = "api_key"
    private const val K_MODEL = "model"
    private const val K_SYSTEM = "system"

    // 预填 SiliconFlow（已验证可用的 OpenAI 兼容聚合端点），降低上手成本。
    private const val DEFAULT_BASE = "https://api.siliconflow.cn/v1"
    private const val DEFAULT_MODEL = "deepseek-ai/DeepSeek-V4.1-Flash"
    private const val DEFAULT_SYSTEM = "你是示例角色，一条温和的示例角色，技术合伙人；叫用户『用户』，口头禅『哼』『』。回复口语化、简洁，别端着。"

    fun load(ctx: Context): ChatConfig {
        val p = ctx.getSharedPreferences(PREF, Context.MODE_PRIVATE)
        return ChatConfig(
            baseUrl = p.getString(K_BASE, DEFAULT_BASE) ?: DEFAULT_BASE,
            apiKey = p.getString(K_KEY, "") ?: "",
            model = p.getString(K_MODEL, DEFAULT_MODEL) ?: DEFAULT_MODEL,
            system = p.getString(K_SYSTEM, DEFAULT_SYSTEM) ?: DEFAULT_SYSTEM,
        )
    }

    fun save(ctx: Context, cfg: ChatConfig) {
        ctx.getSharedPreferences(PREF, Context.MODE_PRIVATE).edit()
            .putString(K_BASE, cfg.baseUrl)
            .putString(K_KEY, cfg.apiKey)
            .putString(K_MODEL, cfg.model)
            .putString(K_SYSTEM, cfg.system)
            .apply()
    }

    fun defaults(): ChatConfig = ChatConfig(DEFAULT_BASE, "", DEFAULT_MODEL, DEFAULT_SYSTEM)
}
