package com.starriver.app

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import mobilebind.Mobilebind
import org.json.JSONArray
import org.json.JSONObject

/**
 * T-M4-03 开发者模式三面板：情绪曲线（五维）/ 记忆强度列表 / 七层上下文预览。
 * 数据走 kernel/tool 中性接口（MemorySource/MoodSource）形态；当前为演示数据（界面明确标注），
 * 真实数据待应用装配层注入 hippocampus/endocrine 适配器（红线2）。
 */
@Composable
fun PanelsScreen(onBack: () -> Unit) {
    var raw by remember { mutableStateOf("") }
    var loading by remember { mutableStateOf(false) }
    fun load() {
        loading = true
        Bg.run({ Mobilebind.devPanels() }) { r -> raw = r; loading = false }
    }
    androidx.compose.runtime.LaunchedEffect(Unit) { load() }

    Column(Modifier.fillMaxSize().padding(12.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            TextButton(onClick = onBack) { Text("← 返回") }
            Text("开发者三面板", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
        }
        val demo = try { JSONObject(raw).optBoolean("demo", false) } catch (e: Exception) { false }
        if (demo) {
            Text("⚠ 演示数据：待 hippocampus/endocrine 经中性接口注入真实数据后自动切换", fontSize = 11.sp, color = MaterialTheme.colorScheme.secondary)
        }
        Button(onClick = { load() }, enabled = !loading) { Text(if (loading) "读取中…" else "刷新") }
        if (raw.startsWith("ERR:")) { Text(raw, color = MaterialTheme.colorScheme.error); return@Column }

        val o = try { JSONObject(raw) } catch (e: Exception) { JSONObject() }

        // 面板①：情绪曲线（五维）
        PanelCard("① 情绪曲线（五维）") {
            val mood = o.optJSONObject("mood")
            val dims = listOf("valence" to "效价", "arousal" to "唤醒", "intimacy" to "亲密", "fatigue" to "疲劳", "curiosity" to "好奇")
            dims.forEach { (k, label) ->
                val v = mood?.optDouble(k, 0.0) ?: 0.0
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(label, fontSize = 12.sp, modifier = Modifier.padding(end = 8.dp), fontWeight = FontWeight.Bold)
                    Box(Modifier.weight(1f).height(14.dp).background(MaterialTheme.colorScheme.surfaceVariant, RoundedCornerShape(7.dp))) {
                        Box(Modifier.fillMaxWidth(v.coerceIn(0.0, 1.0).toFloat()).height(14.dp).background(MaterialTheme.colorScheme.primary, RoundedCornerShape(7.dp)))
                    }
                    Text(String.format("%.2f", v), fontSize = 11.sp, modifier = Modifier.padding(start = 8.dp))
                }
            }
        }

        // 面板②：记忆强度列表
        PanelCard("② 记忆强度（检索命中）") {
            val mem = o.optJSONArray("memory") ?: JSONArray()
            for (i in 0 until mem.length()) {
                val m = mem.optJSONObject(i) ?: continue
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(String.format("%.2f", m.optDouble("score", 0.0)), fontSize = 11.sp, color = MaterialTheme.colorScheme.primary, fontWeight = FontWeight.Bold, modifier = Modifier.padding(end = 8.dp))
                    Text(m.optString("text"), fontSize = 12.sp)
                }
            }
            if (mem.length() == 0) Text("（无记忆命中）", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }

        // 面板③：七层上下文预览（token 体量）
        PanelCard("③ 七层上下文组装预览（token）") {
            val ctx = o.optJSONArray("context") ?: JSONArray()
            var total = 0
            for (i in 0 until ctx.length()) {
                val c = ctx.optJSONObject(i) ?: continue
                val tk = c.optInt("tokens", 0); total += tk
                Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                    Text("${i + 1}. ${c.optString("name")}", fontSize = 12.sp, modifier = Modifier.weight(1f))
                    Text("$tk tok", fontSize = 11.sp, fontFamily = androidx.compose.ui.text.font.FontFamily.Monospace)
                }
                Text(c.optString("note"), fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            Text("合计 ≈ $total token", fontSize = 12.sp, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.primary, modifier = Modifier.padding(top = 4.dp))
        }
    }
}

@Composable
private fun PanelCard(title: String, content: @Composable () -> Unit) {
    Card(Modifier.fillMaxWidth(), shape = RoundedCornerShape(LocalCornerRadius.current)) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(title, fontWeight = FontWeight.Bold, fontSize = 15.sp)
            content()
        }
    }
}
