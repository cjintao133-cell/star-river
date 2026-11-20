package com.starriver.app

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import mobilebind.Mobilebind
import org.json.JSONObject

/** 设置三层：新手（端点）/ 进阶（主题·动效·搜索·语音·原生搜索）/ 开发者（合规·诊断·探测）。 */
@Composable
fun SettingsTab(
    config: ChatConfig,
    theme: ThemeConfig,
    reduceMotion: Boolean,
    lang: String,
    followSystem: Boolean,
    onThemeChange: (ThemeConfig) -> Unit,
    onReduceMotionChange: (Boolean) -> Unit,
    onLangChange: (String) -> Unit,
    onFollowSystem: () -> Unit,
    onSave: (ChatConfig) -> Unit,
) {
    var tier by remember { mutableStateOf(0) }
    var screen by remember { mutableStateOf(0) }
    if (screen == 1) { PermissionScreen { screen = 0 }; return }
    if (screen == 2) { ReportScreen { screen = 0 }; return }
    if (screen == 3) { SafetyScreen { screen = 0 }; return }
    if (screen == 4) { PanelsScreen { screen = 0 }; return }
    Column(Modifier.fillMaxSize().padding(12.dp)) {
        Text(AppI18n.t("settings.title"), style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
        Row(Modifier.fillMaxWidth().padding(bottom = 4.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(AppI18n.t("settings.language"), fontWeight = FontWeight.Bold)
            OutlinedButton(onClick = { onFollowSystem() }, enabled = !followSystem) { Text("跟随系统") }
            OutlinedButton(onClick = { onLangChange("zh") }, enabled = followSystem || lang != "zh") { Text("中文") }
            OutlinedButton(onClick = { onLangChange("en") }, enabled = followSystem || lang != "en") { Text("English") }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(6.dp), modifier = Modifier.padding(vertical = 8.dp)) {
            TierBtn(AppI18n.t("settings.tier.novice"), 0, tier) { tier = it }
            TierBtn(AppI18n.t("settings.tier.advanced"), 1, tier) { tier = it }
            TierBtn(AppI18n.t("settings.tier.developer"), 2, tier) { tier = it }
        }
        when (tier) {
            0 -> NoviceSettings(config, onSave)
            1 -> AdvancedSettings(theme, reduceMotion, onThemeChange, onReduceMotionChange)
            else -> DeveloperSettings(config,
                onOpenPermission = { screen = 1 },
                onOpenReport = { screen = 2 },
                onOpenSafety = { screen = 3 },
                onOpenPanels = { screen = 4 })
        }
    }
}

@Composable
private fun TierBtn(label: String, idx: Int, cur: Int, onSel: (Int) -> Unit) {
    OutlinedButton(onClick = { onSel(idx) }) {
        Text(label, fontWeight = if (cur == idx) FontWeight.Bold else FontWeight.Normal,
            color = if (cur == idx) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun NoviceSettings(config: ChatConfig, onSave: (ChatConfig) -> Unit) {
    var base by remember { mutableStateOf(config.baseUrl) }
    var key by remember { mutableStateOf(config.apiKey) }
    var model by remember { mutableStateOf(config.model) }
    var system by remember { mutableStateOf(config.system) }
    var note by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }

    fun probe() {
        if (busy) return; busy = true; note = "校验中…"
        Bg.run({ Mobilebind.probeEndpoint(base.trim(), key.trim(), model.trim()) }) { raw ->
            note = try {
                val o = JSONObject(raw)
                if (o.optBoolean("ok")) "✅ 可用（模型 " + o.optString("model") + "）"
                else "❌ " + o.optString("category") + (if (o.optString("error").isNotEmpty()) "\n" + o.optString("error") else "")
            } catch (e: Exception) { raw }
            busy = false
        }
    }
    fun listModels() {
        if (busy) return; busy = true; note = "拉取模型…"
        Bg.run({ Mobilebind.listModels(base.trim(), key.trim()) }) { raw ->
            note = if (raw.startsWith("ERR:")) "拉取失败：" + raw.removePrefix("ERR:") else raw
            busy = false
        }
    }

    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Text("端点（OpenAI 兼容）。key 经 Android Keystore 加密，不落明文。", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        OutlinedTextField(value = base, onValueChange = { base = it }, label = { Text("端点 base") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
        OutlinedTextField(value = key, onValueChange = { key = it }, label = { Text("API Key（加密存储）") }, modifier = Modifier.fillMaxWidth(), singleLine = true, visualTransformation = PasswordVisualTransformation())
        OutlinedTextField(value = model, onValueChange = { model = it }, label = { Text("模型 id") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
        OutlinedTextField(value = system, onValueChange = { system = it }, label = { Text("人设 system") }, modifier = Modifier.fillMaxWidth(), minLines = 3)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(onClick = { onSave(ChatConfig(base.trim(), key.trim(), model.trim(), system)) },
                enabled = base.isNotBlank() && key.isNotBlank() && model.isNotBlank()) { Text("保存") }
            OutlinedButton(onClick = { probe() }, enabled = !busy && base.isNotBlank() && model.isNotBlank()) { Text("校验端点") }
            OutlinedButton(onClick = { listModels() }, enabled = !busy && base.isNotBlank()) { Text("拉取模型") }
        }
        if (note.isNotEmpty()) Text(note, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun AdvancedSettings(
    theme: ThemeConfig,
    reduceMotion: Boolean,
    onThemeChange: (ThemeConfig) -> Unit,
    onReduceMotionChange: (Boolean) -> Unit,
) {
    val ctx = LocalContext.current
    var provider by remember { mutableStateOf(Prefs.searchProvider(ctx)) }
    var searxng by remember { mutableStateOf(Prefs.searxngBase(ctx)) }
    var braveKey by remember { mutableStateOf(SecretStore.get(ctx, "search.brave_key") ?: "") }
    var ttsVoice by remember { mutableStateOf(Prefs.ttsVoice(ctx)) }
    var ttsFormat by remember { mutableStateOf(Prefs.ttsFormat(ctx)) }
    var nativeSearch by remember { mutableStateOf(Prefs.nativeSearch(ctx)) }
    var communityBase by remember { mutableStateOf(CommunityPrefs.base(ctx)) }
    var personalTag by remember { mutableStateOf(SysPrefs.personalTag(ctx)) }
    var imgBase by remember { mutableStateOf(ImagePrefs.base(ctx)) }
    var imgModel by remember { mutableStateOf(ImagePrefs.model(ctx)) }
    var imgKey by remember { mutableStateOf(ImagePrefs.apiKey(ctx)) }
    var imgEditPath by remember { mutableStateOf(ImagePrefs.editPath(ctx)) }
    var chatProto by remember { mutableStateOf(ProtoPrefs.provider(ctx)) }
    var saved by remember { mutableStateOf("") }

    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Text("聊天协议（模型厂商）", fontWeight = FontWeight.Bold)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(onClick = { chatProto = "openai" }, enabled = chatProto != "openai") { Text("OpenAI 兼容") }
            OutlinedButton(onClick = { chatProto = "anthropic" }, enabled = chatProto != "anthropic") { Text("Claude") }
            OutlinedButton(onClick = { chatProto = "gemini" }, enabled = chatProto != "gemini") { Text("Gemini") }
        }
        Text("选 Claude/Gemini 时，端点/模型按对应官方或中转地址填。", fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)

        Text(AppI18n.t("settings.theme"), fontWeight = FontWeight.Bold)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(onClick = { onThemeChange(theme.copy(mode = ThemeMode.DAY)) }, enabled = theme.mode != ThemeMode.DAY) { Text("卡其日间") }
            OutlinedButton(onClick = { onThemeChange(theme.copy(mode = ThemeMode.NIGHT)) }, enabled = theme.mode != ThemeMode.NIGHT) { Text("夜间流星") }
        }
        Text("自定义强调色", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 4.dp))
        FlowRow(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            AccentPresets.forEach { (argb, name) ->
                val c = if (argb == null) MaterialTheme.colorScheme.primary else Color(argb)
                val selected = theme.accent == argb
                Box(
                    Modifier.size(34.dp).background(c, CircleShape)
                        .border(if (selected) 3.dp else 1.dp, if (selected) MaterialTheme.colorScheme.onSurface else MaterialTheme.colorScheme.outline, CircleShape)
                        .clickable { onThemeChange(theme.copy(accent = argb)) },
                )
            }
        }
        Text("圆角档", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            RadiusPresets.forEach { (dpv, name) ->
                OutlinedButton(onClick = { onThemeChange(theme.copy(radiusDp = dpv)) }, enabled = theme.radiusDp != dpv) { Text(name) }
            }
        }

        Row(Modifier.fillMaxWidth().padding(top = 4.dp), verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                Text("减少动态 / 关闭氛围动效", fontWeight = FontWeight.Bold)
                Text("夜间流星背景静止为静态星空（省电、无障碍）", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            Switch(checked = reduceMotion, onCheckedChange = onReduceMotionChange)
        }

        Text("网页搜索源", fontWeight = FontWeight.Bold, modifier = Modifier.padding(top = 6.dp))
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(onClick = { provider = "searxng" }, enabled = provider != "searxng") { Text("SearXNG 自建") }
            OutlinedButton(onClick = { provider = "brave" }, enabled = provider != "brave") { Text("Brave") }
        }
        if (provider == "searxng") {
            OutlinedTextField(value = searxng, onValueChange = { searxng = it }, label = { Text("SearXNG base（自建实例）") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
        } else {
            OutlinedTextField(value = braveKey, onValueChange = { braveKey = it }, label = { Text("Brave API Key（加密存储）") }, modifier = Modifier.fillMaxWidth(), singleLine = true, visualTransformation = PasswordVisualTransformation())
        }

        Text("语音（TTS 朗读）", fontWeight = FontWeight.Bold, modifier = Modifier.padding(top = 6.dp))
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedTextField(value = ttsVoice, onValueChange = { ttsVoice = it }, label = { Text("音色 voice") }, modifier = Modifier.weight(1f), singleLine = true)
            OutlinedTextField(value = ttsFormat, onValueChange = { ttsFormat = it }, label = { Text("格式 mp3") }, modifier = Modifier.weight(1f), singleLine = true)
        }

        Row(Modifier.fillMaxWidth().padding(top = 6.dp), verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                Text("原生搜索（按模型）", fontWeight = FontWeight.Bold)
                Text("开：模型自带 web_search；关：走外部搜索源", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            Switch(checked = nativeSearch, onCheckedChange = { nativeSearch = it })
        }

        Text("社区源（Git 仓库 / 自建 API，同一契约）", fontWeight = FontWeight.Bold, modifier = Modifier.padding(top = 6.dp))
        OutlinedTextField(value = communityBase, onValueChange = { communityBase = it },
            label = { Text("base，如 https://raw.githubusercontent.com/你/repo/main/") },
            modifier = Modifier.fillMaxWidth(), singleLine = true)
        Text("留空=用内置种子。见 docs/community-ecosystem-contract.md", fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)

        Text("应用标识请求头（随请求发给你自配端点）", fontWeight = FontWeight.Bold, modifier = Modifier.padding(top = 6.dp))
        OutlinedTextField(value = personalTag, onValueChange = { personalTag = it },
            label = { Text("X-Star-River-Tag（如 用户；默认头 X-Star-River-Client 恒带）") },
            modifier = Modifier.fillMaxWidth(), singleLine = true)

        Text("画图模型（相册/自拍：人设图垫图，直接画图模型）", fontWeight = FontWeight.Bold, modifier = Modifier.padding(top = 6.dp))
        OutlinedTextField(value = imgBase, onValueChange = { imgBase = it }, label = { Text("画图端点 base（如 https://api.xxx.com/v1）") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
        OutlinedTextField(value = imgModel, onValueChange = { imgModel = it }, label = { Text("画图模型（如 gpt-image-1 / 支持的图生图模型）") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
        OutlinedTextField(value = imgKey, onValueChange = { imgKey = it }, label = { Text("画图 API Key（加密存储）") }, modifier = Modifier.fillMaxWidth(), singleLine = true, visualTransformation = PasswordVisualTransformation())
        OutlinedTextField(value = imgEditPath, onValueChange = { imgEditPath = it }, label = { Text("图生图路径（默认 /images/edits，可留空）") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
        Text("垫图=人设图；有垫图走图生图端点保角色一致性，无需 ComfyUI。", fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)

        Button(onClick = {
            Prefs.setSearchProvider(ctx, provider)
            Prefs.setSearxngBase(ctx, searxng.trim())
            if (braveKey.isNotBlank()) SecretStore.put(ctx, "search.brave_key", braveKey)
            Prefs.setTtsVoice(ctx, ttsVoice.trim().ifBlank { "alloy" })
            Prefs.setTtsFormat(ctx, ttsFormat.trim().ifBlank { "mp3" })
            Prefs.setNativeSearch(ctx, nativeSearch)
            CommunityPrefs.setBase(ctx, communityBase.trim())
            SysPrefs.setPersonalTag(ctx, personalTag.trim())
            HeaderHelp.apply(ctx)
            ProtoPrefs.setProvider(ctx, chatProto); ProtoPrefs.apply(ctx)
            ImagePrefs.setBase(ctx, imgBase.trim()); ImagePrefs.setModel(ctx, imgModel.trim())
            ImagePrefs.setEditPath(ctx, imgEditPath.trim()); ImagePrefs.setApiKey(ctx, imgKey.trim())
            val vr = VisualCfg.initIfConfigured(ctx)
            saved = if (vr == "not-configured") "已保存（画图模型未配全）" else if (vr.startsWith("ERR")) "已保存，画图初始化失败：$vr" else "已保存，画图模型就绪"
        }, modifier = Modifier.padding(top = 8.dp)) { Text("保存进阶设置") }
        if (saved.isNotEmpty()) Text(saved, fontSize = 12.sp, color = MaterialTheme.colorScheme.primary)
    }
}

@Composable
private fun DeveloperSettings(
    config: ChatConfig,
    onOpenPermission: () -> Unit,
    onOpenReport: () -> Unit,
    onOpenSafety: () -> Unit,
    onOpenPanels: () -> Unit,
) {
    var info by remember { mutableStateOf("") }
    var probe by remember { mutableStateOf("") }
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Text("合规 / 系统", fontWeight = FontWeight.Bold)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(onClick = onOpenPermission) { Text("权限管理") }
            OutlinedButton(onClick = onOpenReport) { Text("举报与反馈") }
        }
        OutlinedButton(onClick = onOpenSafety, modifier = Modifier.fillMaxWidth()) { Text("🛡 系统 · 安全 · 自愈板块") }
        OutlinedButton(onClick = onOpenPanels, modifier = Modifier.fillMaxWidth()) { Text("📊 开发者三面板（情绪/记忆/上下文）") }
        Text("内核信息", fontWeight = FontWeight.Bold, modifier = Modifier.padding(top = 6.dp))
        Button(onClick = {
            Bg.run({ "version=" + Mobilebind.version() + "\n\nDeviceProfile=\n" + Mobilebind.deviceProfile() }) { info = it }
        }) { Text("读取内核版本 / 设备档") }
        if (info.isNotEmpty()) Text(info, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Text("模型实测探测（有真实成本，仅在选中模型时点）", fontWeight = FontWeight.Bold, modifier = Modifier.padding(top = 6.dp))
        Button(onClick = {
            if (config.baseUrl.isBlank() || config.model.isBlank()) { probe = "端点/模型未配置"; return@Button }
            Bg.run({ Mobilebind.probeModel(config.baseUrl, config.apiKey, config.model) }) { r -> probe = r }
        }) { Text("探测当前模型能力") }
        if (probe.isNotEmpty()) Text(probe, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}
