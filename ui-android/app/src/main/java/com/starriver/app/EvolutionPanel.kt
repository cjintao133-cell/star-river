package com.starriver.app

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
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
import org.json.JSONObject

/**
 * 受控自我进化 · 审计面板（真实数据，不造假）。
 * 数据源：Mobilebind.evolutionStats()（记忆库条数/容量、情绪五维、演进事件门铃、边界硬约束）。
 * 语义：进化限定在「人格/记忆数据层」，不自主改代码、不越红线、仅本地、可审计、可回滚。
 */
@Composable
fun EvolutionPanel() {
    var raw by remember { mutableStateOf("") }
    var loading by remember { mutableStateOf(false) }
    fun load() {
        loading = true
        Bg.run({ try { Mobilebind.evolutionStats() } catch (t: Throwable) { "ERR:" + (t.message ?: "?") } }) { r -> raw = r; loading = false }
    }
    androidx.compose.runtime.LaunchedEffect(Unit) { load() }

    val o = try { JSONObject(raw) } catch (e: Exception) { JSONObject() }
    val memCount = o.optInt("memoryCount", 0)
    val memCap = o.optInt("memCap", 1).coerceAtLeast(1)
    val eventTotal = o.optInt("eventTotal", 0)
    val boundaries = o.optJSONObject("boundaries")
    val mood = o.optJSONObject("mood")
    val eventCounts = o.optJSONObject("eventCounts")
    val recent = o.optJSONArray("recent")

    Card(Modifier.fillMaxWidth(), shape = RoundedCornerShape(LocalCornerRadius.current)) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("🧬 自我进化（受控 · 审计）", fontWeight = FontWeight.Bold, fontSize = 15.sp)
            if (raw.startsWith("ERR:")) {
                Text(raw, color = MaterialTheme.colorScheme.error, fontSize = 12.sp)
            } else {
                // 记忆库
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("记忆库", fontSize = 12.sp, modifier = Modifier.weight(1f))
                    Text("$memCount / $memCap 条", fontSize = 12.sp, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.primary)
                }
                // 演进事件
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("演进事件（门铃）", fontSize = 12.sp, modifier = Modifier.weight(1f))
                    Text("共 $eventTotal 条", fontSize = 12.sp, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.primary)
                }
                if (eventCounts != null && eventCounts.length() > 0) {
                    val keys = eventCounts.keys()
                    while (keys.hasNext()) {
                        val k = keys.next()
                        Text("· $k × ${eventCounts.optInt(k, 0)}", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
                // 最近演进记录（新→旧）
                if (recent != null && recent.length() > 0) {
                    Text("最近演进：", fontSize = 11.sp, fontWeight = FontWeight.Bold, modifier = Modifier.padding(top = 2.dp))
                    for (i in 0 until recent.length()) {
                        val e = recent.optJSONObject(i) ?: continue
                        val ts = e.optLong("ts", 0L)
                        val ago = if (ts > 0) {
                            val s = ((System.currentTimeMillis() - ts) / 1000).coerceAtLeast(0)
                            if (s < 60) "${s}s 前" else if (s < 3600) "${s / 60}m 前" else "${s / 3600}h 前"
                        } else "—"
                        Text("· ${e.optString("topic")} · $ago", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
                // 情绪快照（五维）
                if (mood != null) {
                    Text("情绪快照：", fontSize = 11.sp, fontWeight = FontWeight.Bold, modifier = Modifier.padding(top = 2.dp))
                    val dims = listOf("valence" to "效价", "arousal" to "唤醒", "intimacy" to "亲密", "fatigue" to "疲劳", "curiosity" to "好奇")
                    dims.chunked(3).forEach { row ->
                        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            row.forEach { (k, label) ->
                                Text("$label ${String.format("%.2f", mood.optDouble(k, 0.0))}", fontSize = 11.sp, modifier = Modifier.weight(1f))
                            }
                        }
                    }
                }
                // 边界硬约束
                Text("边界（硬约束）：", fontSize = 11.sp, fontWeight = FontWeight.Bold, modifier = Modifier.padding(top = 4.dp))
                val bounds = listOf(
                    "noCodeEdit" to "不自主改内核代码 / 不自提权",
                    "localOnly" to "全本地，不外发",
                    "whitelistOnly" to "仅写白名单数据字段",
                    "redlineFloor" to "任何开关不得降低安全红线",
                    "auditable" to "演进经 bus 门铃留痕、可审计回滚"
                )
                bounds.forEach { (k, label) ->
                    val ok = boundaries?.optBoolean(k, false) ?: false
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                        Text(if (ok) "✓" else "~", fontWeight = FontWeight.Bold, color = if (ok) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.error, fontSize = 12.sp)
                        Text(label, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurface)
                    }
                }
                Text("说明：不做「AI 自主改代码」——与安全红线冲突，明确不支持；进化限定在人格/记忆数据层，全部本地且可审计。", fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 2.dp))
            }
            Button(onClick = { load() }, enabled = !loading, modifier = Modifier.fillMaxWidth()) {
                Text(if (loading) "读取中…" else "刷新自我进化审计")
            }
        }
    }
}
