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
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import mobilebind.Mobilebind
import org.json.JSONArray
import org.json.JSONObject

/**
 * 社区：论坛体（联网后端待部署，先种子数据+优雅降级） / MCP 集市（插件单独启用=内核热插拔，不重启） /
 * 生态接入（三档说明）。进入 MCP 集市自动加载已装插件（McpInit/Restore 在 KernelBoot 完成）。
 */
@Composable
fun CommunityTab() {
    var section by remember { mutableStateOf(0) }
    var tick by remember { mutableStateOf(0) }
    val ctx = LocalContext.current
    var servers by remember { mutableStateOf<List<McpServerUi>>(emptyList()) }
    var toolCount by remember { mutableStateOf(0) }
    var busy by remember { mutableStateOf(false) }
    var err by remember { mutableStateOf("") }

    fun refresh() {
        busy = true
        Bg.run({ Mobilebind.mcpServers() }) { raw ->
            servers = parseMcpServers(raw)
            Bg.run({ Mobilebind.mcpTools() }) { t ->
                toolCount = try { JSONArray(t).length() } catch (e: Exception) { 0 }
            }
            busy = false
        }
    }
    // 进 MCP 集市/生态区自动加载；论坛区也刷新（论坛后端待部署→种子+降级说明）
    LaunchedEffect(section, tick) { if (section != 0) refresh() }

    Column(Modifier.fillMaxSize().padding(12.dp)) {
        Text("社区", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
        Row(horizontalArrangement = Arrangement.spacedBy(6.dp), modifier = Modifier.padding(vertical = 8.dp)) {
            OutlinedButton(onClick = { section = 0 }) { Text("圈内论坛", fontWeight = if (section == 0) FontWeight.Bold else FontWeight.Normal) }
            OutlinedButton(onClick = { section = 1 }) { Text("MCP 集市", fontWeight = if (section == 1) FontWeight.Bold else FontWeight.Normal) }
            OutlinedButton(onClick = { section = 2 }) { Text("生态接入", fontWeight = if (section == 2) FontWeight.Bold else FontWeight.Normal) }
        }
        when (section) {
            0 -> ForumSection()
            1 -> MarketSection(servers, toolCount, busy, err, onRefresh = { tick++ })
            else -> EcosystemSection()
        }
    }
}

// —— 论坛（联网后端待部署，种子 + 优雅降级） ——
private data class Post(val title: String, val author: String, val replies: Int, val snippet: String)

@Composable
private fun ForumSection() {
    var status by remember { mutableStateOf("离线种子数据（论坛后端待部署，联网即自动拉取）") }
    var showReport by remember { mutableStateOf(false) }
    if (showReport) ReportDialog { showReport = false }
    val posts = remember {
        listOf(
            Post("求助：2GB 安卓跑动效背景怎么降级不卡", "新人阿柴", 7, "开了流星背景，切后台回来微微掉帧"),
            Post("角色卡 V2 世界书触发深度怎么设", "示例鱼罐头", 23, "递归到 4 层开始灌水，大家一般设多少"),
            Post("自拍改头像效果分享（附参数）", "星河观察者", 31, "ComfyUI 走云 GPU，出图挺稳"),
        )
    }
    Column(Modifier.fillMaxWidth()) {
        Card(Modifier.fillMaxWidth()) {
            Row(Modifier.padding(12.dp), verticalAlignment = Alignment.CenterVertically) {
                Text(status, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f))
                TextButton(onClick = { showReport = true }) { Text("⚑ 举报") }
            }
        }
        posts.forEach { p ->
            Card(Modifier.fillMaxWidth().padding(top = 8.dp)) {
                Column(Modifier.padding(12.dp)) {
                    Text(p.title, fontWeight = FontWeight.Bold, fontSize = 14.sp)
                    Text(p.snippet, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 2.dp))
                    Text("${p.author} · 💬 ${p.replies}", fontSize = 11.sp, color = MaterialTheme.colorScheme.primary, modifier = Modifier.padding(top = 4.dp))
                }
            }
        }
    }
}

// —— MCP 集市：安装 / 单独启用(热插拔) / 卸载 ——
private data class McpServerUi(val name: String, val endpoint: String, val enabled: Boolean, val connected: Boolean, val toolCount: Int, val err: String)

private fun parseMcpServers(raw: String): List<McpServerUi> {
    val out = mutableListOf<McpServerUi>()
    if (raw.startsWith("ERR:")) return out
    val arr = try { JSONArray(raw) } catch (e: Exception) { JSONArray() }
    for (i in 0 until arr.length()) {
        val o = arr.optJSONObject(i) ?: continue
        out.add(McpServerUi(o.optString("name"), o.optString("endpoint"), o.optBoolean("enabled"), o.optBoolean("connected"), o.optInt("tool_count"), o.optString("err")))
    }
    return out
}

private data class SeedPlugin(val name: String, val endpoint: String, val desc: String)

@Composable
private fun MarketSection(servers: List<McpServerUi>, toolCount: Int, busy: Boolean, err: String, onRefresh: () -> Unit) {
    val ctx = LocalContext.current
    var showAdd by remember { mutableStateOf(false) }
    var name by remember { mutableStateOf("") }
    var endpoint by remember { mutableStateOf("") }
    var header by remember { mutableStateOf("") }
    var addNote by remember { mutableStateOf("") }
    val seeds = remember {
        listOf(
            SeedPlugin("tavily", "https://mcp.tavily.com/mcp", "网页搜索（摘要省 token）"),
            SeedPlugin("brave-search", "https://mcp.brave.com/sse", "Brave 搜索 MCP"),
            SeedPlugin("filesystem", "", "文件系统（stdio 仅桌面，HTTP 端点自填）"),
        )
    }
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text("已装插件 · 可用工具 $toolCount 个 · ${servers.size} 个插件", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        if (err.isNotEmpty()) Text(err, color = Color.Red, fontSize = 12.sp)
        servers.forEach { s ->
            Card(Modifier.fillMaxWidth()) {
                Row(Modifier.padding(12.dp), verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text(s.name, fontWeight = FontWeight.Bold, fontSize = 14.sp)
                        Text(s.endpoint.ifBlank { "—" }, fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        Text(
                            when {
                                s.err.isNotBlank() -> "连接失败：${s.err}"
                                s.connected -> "已连接 · ${s.toolCount} 工具"
                                else -> "未连接（启用即连）"
                            },
                            fontSize = 11.sp,
                            color = if (s.err.isNotBlank()) Color.Red else MaterialTheme.colorScheme.primary,
                        )
                    }
                    Switch(checked = s.enabled, onCheckedChange = { v -> Bg.run({ Mobilebind.mcpSetEnabled(s.name, v) }) { onRefresh() } })
                    TextButton(onClick = { Bg.run({ Mobilebind.mcpRemoveServer(s.name) }) { onRefresh() } }) { Text("🗑") }
                }
            }
        }
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(onClick = { showAdd = !showAdd }) { Text("＋ 添加插件") }
            OutlinedButton(onClick = onRefresh) { Text(if (busy) "刷新中…" else "刷新") }
        }
        if (showAdd) {
            Text("填 MCP 端点（HTTP/SSE）。stdio 类仅桌面可用。", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            OutlinedTextField(name, { name = it }, label = { Text("名称") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
            OutlinedTextField(endpoint, { endpoint = it }, label = { Text("MCP 端点 URL") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
            OutlinedTextField(header, { header = it }, label = { Text("请求头 JSON（如 {\"Authorization\":\"Bearer x\"}，可选）") }, modifier = Modifier.fillMaxWidth(), visualTransformation = PasswordVisualTransformation())
            Button(onClick = {
                val n = name.trim(); val e = endpoint.trim()
                if (n.isEmpty() || e.isEmpty()) { addNote = "名称/端点必填"; return@Button }
                addNote = "连接中…"
                Bg.run({ Mobilebind.mcpAddServer(n, e, header.trim()) }) { r ->
                    addNote = if (r == "OK") { name = ""; endpoint = ""; header = ""; showAdd = false; onRefresh(); "已安装" } else r
                }
            }) { Text("安装并连接") }
            if (addNote.isNotEmpty()) Text(addNote, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        Text("推荐（点填充到上面表单）：", fontWeight = FontWeight.Bold, fontSize = 12.sp, modifier = Modifier.padding(top = 6.dp))
        seeds.forEach { sd ->
            Card(Modifier.fillMaxWidth()) {
                Row(Modifier.padding(10.dp), verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text(sd.name, fontWeight = FontWeight.Bold, fontSize = 13.sp)
                        Text(sd.desc, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    TextButton(onClick = { name = sd.name; endpoint = sd.endpoint; showAdd = true }) { Text("添加") }
                }
            }
        }
        Text("说明：插件 key 在设置‑进阶填；插件启用/停用=运行时热插拔，不重启。市场目录后端待部署，当前用内置种子 + 手动填端点。",
            fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

// —— 生态接入（三档说明，来自 docs/ecosystem-ingestion.md） ——
@Composable
private fun EcosystemSection() {
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Text("生态接入", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold)
        Text("以 MCP 为边界吃生态；插件一律外部进程、可选可关可沙盒。", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        EcoTier("① MCP 原生 — 今天就能用", "任何以 MCP server 发布的能力（搜索/文件/浏览器/数据库…）直接挂，零代码。", true)
        EcoTier("② 数据/格式导入 — 已支持", "角色卡 V2 / 世界书 / 预设 / 主题包（数据非代码），本地导入。", true)
        EcoTier("③ 兼容服务器 — 需逐框架写，桌面侧", "给 AstrBot(Py)/SillyTavern(JS) 等写独立进程兼容层，需拖其运行时，不进安卓内核。", false)
        EcoTier("④ 源码移植 — 人工重写", "要对齐具体能力就重写，最稳。", false)
        Card(Modifier.fillMaxWidth()) {
            Column(Modifier.padding(12.dp)) {
                Text("反向护城河", fontWeight = FontWeight.Bold, fontSize = 13.sp)
                Text("把我们自己的角色/记忆/情绪/视觉导出成 MCP server，让别人来调用我们的示例角色——当标准提供方，比当生态吸尘器值钱。",
                    fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
}

@Composable
private fun EcoTier(title: String, desc: String, available: Boolean) {
    Card(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(12.dp)) {
            Text(title, fontWeight = FontWeight.Bold, fontSize = 13.sp, color = if (available) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurface)
            Text(desc, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}
