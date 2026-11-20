package com.starriver.app

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
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
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import org.json.JSONArray

/** 崩溃诊断 + 隐私边界（diag，本地 dump 绝不上传）。 */
@Composable
fun DiagScreen(onBack: () -> Unit) {
    var privacy by remember { mutableStateOf("") }
    var dumps by remember { mutableStateOf<List<Triple<String, Long, Long>>>(emptyList()) }
    var viewName by remember { mutableStateOf("") }
    var viewContent by remember { mutableStateOf("") }
    var events by remember { mutableStateOf("") }

    fun refresh() {
        Bg.async({ Diag.listDumps() }) { r ->
            val arr = try { JSONArray(r as? String ?: "[]") } catch (e: Exception) { JSONArray() }
            val list = mutableListOf<Triple<String, Long, Long>>()
            for (i in 0 until arr.length()) {
                val o = arr.optJSONObject(i) ?: continue
                list.add(Triple(o.optString("name"), o.optLong("size"), o.optLong("time")))
            }
            dumps = list
        }
    }
    LaunchedEffect(Unit) {
        Bg.async({ Diag.privacyBoundary() }) { privacy = it as? String ?: "" }
        Bg.async({ mobilebind.Mobilebind.busRecent() }) { events = it as? String ?: "" }
        refresh()
    }

    Column(Modifier.fillMaxSize().padding(12.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            TextButton(onClick = onBack) { Text("← 返回") }
            Text("崩溃诊断 · 隐私边界", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
        }
        Text("崩溃只写本地 dump（0600），绝不联网上传；需要时手动把 dump 内容附进 issue。", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)

        Card(Modifier.fillMaxWidth()) {
            Column(Modifier.padding(12.dp)) {
                Text("本地崩溃 dump（${dumps.size}）", fontWeight = FontWeight.Bold, fontSize = 14.sp)
                if (dumps.isEmpty()) Text("暂无崩溃记录", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                dumps.forEach { (name, size, _) ->
                    Row(Modifier.fillMaxWidth().padding(vertical = 2.dp), verticalAlignment = Alignment.CenterVertically) {
                        Text(name, fontSize = 12.sp, modifier = Modifier.weight(1f))
                        Text("$size B", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        TextButton(onClick = {
                            viewName = name
                            Bg.async({ Diag.readDump(name) }) { viewContent = it as? String ?: "" }
                        }) { Text("查看") }
                    }
                }
                OutlinedButton(onClick = { refresh() }, modifier = Modifier.padding(top = 4.dp)) { Text("刷新") }
            }
        }

        if (viewName.isNotEmpty()) {
            Card(Modifier.fillMaxWidth()) {
                Column(Modifier.padding(12.dp)) {
                    Text("dump：$viewName", fontWeight = FontWeight.Bold, fontSize = 13.sp)
                    Text(viewContent, fontSize = 10.sp, fontFamily = FontFamily.Monospace, lineHeight = 13.sp)
                }
            }
        }

        Card(Modifier.fillMaxWidth()) {
            Column(Modifier.padding(12.dp)) {
                Text("最近事件流（总线）", fontWeight = FontWeight.Bold, fontSize = 14.sp)
                if (events.isBlank()) Text("暂无", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                else Text(events, fontSize = 10.sp, fontFamily = FontFamily.Monospace, lineHeight = 13.sp)
            }
        }

        Card(Modifier.fillMaxWidth()) {
            Column(Modifier.padding(12.dp)) {
                Text("隐私边界说明", fontWeight = FontWeight.Bold, fontSize = 14.sp)
                if (privacy.isBlank()) Text("（加载中/不可用）", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                else Text(privacy, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurface)
            }
        }
    }
}
