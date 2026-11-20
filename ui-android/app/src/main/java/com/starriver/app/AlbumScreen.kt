package com.starriver.app

import android.graphics.BitmapFactory
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import mobilebind.Mobilebind
import org.json.JSONArray
import org.json.JSONObject

/** 画图模型初始化：把 ImagePrefs(+SecretStore key) 组装成 VisualInit 配置并初始化。返回 status。 */
object VisualCfg {
    fun initIfConfigured(ctx: android.content.Context): String {
        if (!ImagePrefs.configured(ctx)) return "not-configured"
        val o = JSONObject()
            .put("base_url", ImagePrefs.base(ctx))
            .put("api_key", ImagePrefs.apiKey(ctx))
            .put("model", ImagePrefs.model(ctx))
            .put("backend", "openai-image")
        val ep = ImagePrefs.editPath(ctx)
        if (ep.isNotBlank()) o.put("edit_path", ep)
        return try { Mobilebind.visualInit(o.toString()) } catch (t: Throwable) { "ERR:" + (t.message ?: "?") }
    }
}

private data class AlbumChar(val id: String, val name: String)
private data class SceneOpt(val key: String, val prompt: String)

/**
 * 相册 / 自拍：选角色 → 选场景 → 用其人设图垫图直接画图模型生自拍（已强制水印）→ 设为头像。
 * 干净图标、无文字标签；无画图模型/无人设图都有明确提示。
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun AlbumScreen(app: AppState, onBack: () -> Unit) {
    val ctx = LocalContext.current
    var chars by remember { mutableStateOf<List<AlbumChar>>(emptyList()) }
    var selId by remember { mutableStateOf("") }
    var scenes by remember { mutableStateOf<List<SceneOpt>>(emptyList()) }
    var selScene by remember { mutableStateOf("") }
    var status by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    var resultBmp by remember { mutableStateOf<androidx.compose.ui.graphics.ImageBitmap?>(null) }
    var resultBytes by remember { mutableStateOf<ByteArray?>(null) }

    LaunchedEffect(Unit) {
        // 初始化画图模型（若已配）
        val init = try { VisualCfg.initIfConfigured(ctx) } catch (t: Throwable) { "ERR" }
        if (init == "not-configured") status = "未配置画图模型：去 设置‑进阶 填画图模型（base/key/model）"
        else if (init.startsWith("ERR")) status = init else status = ""
        // 角色
        Bg.run({ Mobilebind.listCharacters() }) { raw ->
            chars = parseChars(raw).map { AlbumChar(it.id, it.name) }
            if (selId.isEmpty()) selId = chars.firstOrNull()?.id ?: ""
        }
        // 场景
        Bg.run({ Mobilebind.visualScenes() }) { raw ->
            val arr = try { JSONArray(raw) } catch (e: Exception) { JSONArray() }
            val list = mutableListOf<SceneOpt>()
            for (i in 0 until arr.length()) {
                val o = arr.optJSONObject(i) ?: continue
                list.add(SceneOpt(o.optString("key"), o.optString("prompt")))
            }
            scenes = list
            selScene = list.firstOrNull()?.key ?: ""
        }
    }

    fun selfie() {
        if (busy) return
        if (selId.isEmpty()) { status = "先选一个角色"; return }
        busy = true; status = "生成中…（垫图保脸，需几秒）"; resultBmp = null; resultBytes = null
        Bg.run({ Mobilebind.visualSelfie(selId, selScene) }) { raw ->
            busy = false
            if (raw.startsWith("ERR:")) { status = raw.removePrefix("ERR:") ; return@run }
            try {
                val o = JSONObject(raw)
                val bytes = android.util.Base64.decode(o.optString("image_b64"), android.util.Base64.DEFAULT)
                resultBytes = bytes
                val bmp = BitmapFactory.decodeByteArray(bytes, 0, bytes.size)
                resultBmp = bmp?.asImageBitmap()
                status = if (o.optBoolean("degraded", false))
                    "生成完成（已带 AI 水印）· " + o.optString("note", "该模型不支持垫图，已降级纯文生图，角色一致性可能下降")
                else "生成完成（已带 AI 水印）"
            } catch (t: Throwable) { status = "解析失败：" + (t.message ?: "?") }
        }
    }

    Column(Modifier.fillMaxSize().padding(12.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            TextButton(onClick = onBack) { IconBack(MaterialTheme.colorScheme.onSurface); Text("返回", modifier = Modifier.padding(start = 2.dp)) }
            Text("相册 · 自拍", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
        }

        // 角色选择
        Text("角色", fontWeight = FontWeight.Bold, fontSize = 13.sp)
        if (chars.isEmpty()) Text("还没有角色，先去联系人创建/导入", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            chars.forEach { c ->
                OutlinedButton(onClick = { selId = c.id }) {
                    Text(c.name.ifBlank { "?" }, fontWeight = if (selId == c.id) FontWeight.Bold else FontWeight.Normal)
                }
            }
        }

        // 场景
        if (scenes.isNotEmpty()) {
            Text("场景", fontWeight = FontWeight.Bold, fontSize = 13.sp)
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                scenes.forEach { s ->
                    OutlinedButton(onClick = { selScene = s.key }) {
                        Text(s.prompt.take(6).ifBlank { s.key }, fontWeight = if (selScene == s.key) FontWeight.Bold else FontWeight.Normal, fontSize = 12.sp)
                    }
                }
            }
        }

        Button(onClick = { selfie() }, enabled = !busy) { IconCamera(MaterialTheme.colorScheme.onPrimary, 18.dp); Text(if (busy) "生成中…" else "生成自拍", modifier = Modifier.padding(start = 6.dp)) }
        if (status.isNotEmpty()) Text(status, fontSize = 12.sp, color = if (status.startsWith("ERR:") || status.contains("失败")) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant)

        resultBmp?.let { bmp ->
            Card(Modifier.fillMaxWidth(), shape = RoundedCornerShape(LocalCornerRadius.current)) {
                Column(Modifier.padding(10.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                    Image(bitmap = bmp, contentDescription = "自拍", modifier = Modifier.fillMaxWidth().height(320.dp))
                    Row(horizontalArrangement = Arrangement.spacedBy(10.dp), modifier = Modifier.padding(top = 8.dp)) {
                        Button(onClick = {
                            val b = resultBytes ?: return@Button
                            Bg.run({ Mobilebind.setCharacterAvatar(selId, b) }) { r -> status = if (r == "OK") "已设为该角色头像" else r }
                        }) { Text("设为头像") }
                        OutlinedButton(onClick = { selfie() }, enabled = !busy) { Text("重拍") }
                    }
                }
            }
        }
    }
}
