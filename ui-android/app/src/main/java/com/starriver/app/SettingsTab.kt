package com.starriver.app

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
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

/** 设置三层：新手（端点）/ 进阶（主题·搜索·语音·原生搜索）/ 开发者（诊断·探测）。 */
@Composable
fun SettingsTab(
    config: ChatConfig,
    themeMode: ThemeMode,
    onThemeChange: (ThemeMode) -> Unit,
    onSave: (ChatConfig) -> Unit,
) {
    var tier by remember { mutableStateOf(0) }
    Column(Modifier.fillMaxSize().padding(12.dp)) {
        Text("设置", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
        Row(horizontalArrangement = Arrangement.spacedBy(6.dp), modifier = Modifier.padding(vertical = 8.dp)) {
            TierBtn("新手", 0, tier) { tier = it }
            TierBtn("进阶", 1, tier) { tier = it }
            TierBtn("开发者", 2, tier) { tier = it }
        }
        when (tier) {
            0 -> NoviceSettings(config, onSave)
            1 -> AdvancedSettings(themeMode, onThemeChange)
            else -> DeveloperSettings(config)
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

@Composable
private fun AdvancedSettings(themeMode: ThemeMode, onThemeChange: (ThemeMode) -> Unit) {
    val ctx = LocalContext.current
    var provider by remember { mutableStateOf(Prefs.searchProvider(ctx)) }
    var searxng by remember { mutableStateOf(Prefs.searxngBase(ctx)) }
    var braveKey by remember { mutableStateOf(SecretStore.get(ctx, "search.brave_key") ?: "") }
    var ttsVoice by remember { mutableStateOf(Prefs.ttsVoice(ctx)) }
    var ttsFormat by remember { mutableStateOf(Prefs.ttsFormat(ctx)) }
    var nativeSearch by remember { mutableStateOf(Prefs.nativeSearch(ctx)) }
    var saved by remember { mutableStateOf("") }

    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Text("主题", fontWeight = FontWeight.Bold)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(onClick = { onThemeChange(ThemeMode.DAY) }, enabled = themeMode != ThemeMode.DAY) { Text("卡其日间") }
            OutlinedButton(onClick = { onThemeChange(ThemeMode.NIGHT) }, enabled = themeMode != ThemeMode.NIGHT) { Text("夜间流星") }
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

        Button(onClick = {
            Prefs.setSearchProvider(ctx, provider)
            Prefs.setSearxngBase(ctx, searxng.trim())
            if (braveKey.isNotBlank()) SecretStore.put(ctx, "search.brave_key", braveKey)
            Prefs.setTtsVoice(ctx, ttsVoice.trim().ifBlank { "alloy" })
            Prefs.setTtsFormat(ctx, ttsFormat.trim().ifBlank { "mp3" })
            Prefs.setNativeSearch(ctx, nativeSearch)
            saved = "已保存进阶设置"
        }, modifier = Modifier.padding(top = 8.dp)) { Text("保存进阶设置") }
        if (saved.isNotEmpty()) Text(saved, fontSize = 12.sp, color = MaterialTheme.colorScheme.primary)
    }
}

@Composable
private fun DeveloperSettings(config: ChatConfig) {
    var info by remember { mutableStateOf("") }
    var probe by remember { mutableStateOf("") }
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Text("内核信息", fontWeight = FontWeight.Bold)
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
