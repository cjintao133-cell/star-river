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

/**
 * 社区：论坛体 / MCP 集市 / 生态接入。
 * 后端 = Git 仓库静态 JSON（docs/community-ecosystem-contract.md），HTTP GET 拉取，失败回落种子。
 * MCP 插件单独启用 = 内核热插拔（mcpSetEnabled），不重启。
 */
@Composable
fun CommunityTab() {
    var section by remember { mutableStateOf(0) }
    var tick by remember { mutableStateOf(0) }
    Column(Modifier.fillMaxSize().padding(12.dp)) {
        Text("社区", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
        Row(horizontalArrangement = Arrangement.spacedBy(6.dp), modifier = Modifier.padding(vertical = 8.dp)) {
            OutlinedButton(onClick = { section = 0 }) { Text("圈内论坛", fontWeight = if (section == 0) FontWeight.Bold else FontWeight.Normal) }
            OutlinedButton(onClick = { section = 1 }) { Text("MCP 集市", fontWeight = if (section == 1) FontWeight.Bold else FontWeight.Normal) }
            OutlinedButton(onClick = { section = 2 }) { Text("生态接入", fontWeight = if (section == 2) FontWeight.Bold else FontWeight.Normal) }
        }
        when (section) {
            0 -> ForumSection(tick)
            1 -> MarketSection(tick) { tick++ }
            else -> EcosystemSection()
        }
    }
}

// —— 论坛：远端 posts.json，失败回落种子 ——
@Composable
private fun ForumSection(tick: Int) {
    var posts by remember { mutableStateOf<List<Community.ForumPost>?>(null) }
    var offline by remember { mutableStateOf(false) }
    var showReport by remember { mutableStateOf(false) }
    if (showReport) ReportDialog { showReport = false }
    LaunchedEffect(tick) {
        Bg.async({ Community.fetchPosts() }) { r ->
            @Suppress("UNCHECKED_CAST") val p = r as? List<Community.ForumPost>
            if (p == null) { offline = true; posts = seedPosts() } else { offline = false; posts = p }
        }
    }
    Column(Modifier.fillMaxWidth().verticalScroll(rememberScrollState())) {
        Card(Modifier.fillMaxWidth()) {
            Row(Modifier.padding(12.dp), verticalAlignment = Alignment.CenterVertically) {
                Text(if (offline) "离线种子（社区源未配置/不可达，设置‑进阶 可填源）" else "已连社区源",
                    fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f))
                TextButton(onClick = { showReport = true }) { Text("⚑ 举报") }
            }
        }
        (posts ?: emptyList()).forEach { p ->
            Card(Modifier.fillMaxWidth().padding(top = 8.dp)) {
                Column(Modifier.padding(12.dp)) {
                    Text(p.title, fontWeight = FontWeight.Bold, fontSize = 14.sp)
                    if (p.body.isNotBlank()) Text(p.body, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 2.dp))
                    Text("${p.author} · 💬 ${p.replies}" + if (p.tags.isNotEmpty()) " · " + p.tags.joinToString("/") else "", fontSize = 11.sp, color = MaterialTheme.colorScheme.primary, modifier = Modifier.padding(top = 4.dp))
                }
            }
        }
    }
}

private fun seedPosts(): List<Community.ForumPost> = listOf(
    Community.ForumPost("s1", "2GB 安卓跑动效背景怎么降级不卡", "新人阿柴", 7, listOf("help"), "开了流星背景，切后台回来微微掉帧。"),
    Community.ForumPost("s2", "角色卡 V2 世界书触发深度怎么设", "示例鱼罐头", 23, listOf("角色卡"), "递归到 4 层开始灌水。"),
    Community.ForumPost("s3", "自拍改头像效果分享（附参数）", "星河观察者", 31, listOf("视觉"), "ComfyUI 走云 GPU，垫图 0.6。"),
)

// —— MCP 集市：远端目录（失败回落种子）+ 已装插件热插拔 ——
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

private fun seedCatalog(): List<Community.CatalogPlugin> = listOf(
    Community.CatalogPlugin("tavily", "tavily", "网页搜索，摘要省 token", "http", "https://mcp.tavily.com/mcp", "none", "Authorization", "", listOf("search")),
    Community.CatalogPlugin("brave-search", "brave-search", "Brave 搜索（需 Key）", "sse", "https://mcp.brave.com/sse", "header", "Authorization", "Bearer <你的 Brave Key>", listOf("search")),
)

@Composable
private fun MarketSection(tick: Int, onChanged: () -> Unit) {
    var servers by remember { mutableStateOf<List<McpServerUi>>(emptyList()) }
    var catalog by remember { mutableStateOf<List<Community.CatalogPlugin>>(emptyList()) }
    var catalogOnline by remember { mutableStateOf(false) }
    var toolCount by remember { mutableStateOf(0) }
    var busy by remember { mutableStateOf(false) }
    var note by remember { mutableStateOf("") }
    var showManual by remember { mutableStateOf(false) }
    var mName by remember { mutableStateOf("") }
    var mEndpoint by remember { mutableStateOf("") }
    var mHeader by remember { mutableStateOf("") }
    var pendingAuthFor by remember { mutableStateOf<Community.CatalogPlugin?>(null) }
    var pendingAuthVal by remember { mutableStateOf("") }

    fun refreshServers() {
        busy = true
        Bg.run({ Mobilebind.mcpServers() }) { raw ->
            servers = parseMcpServers(raw)
            Bg.run({ Mobilebind.mcpTools() }) { t -> toolCount = try { JSONArray(t).length() } catch (e: Exception) { 0 } }
            busy = false
        }
    }
    LaunchedEffect(tick) {
        refreshServers()
        Bg.async({ Community.fetchCatalog() }) { r ->
            @Suppress("UNCHECKED_CAST") val cat = r as? List<Community.CatalogPlugin>
            if (cat == null) { catalogOnline = false; catalog = seedCatalog() } else { catalogOnline = true; catalog = cat }
        }
    }

    fun install(p: Community.CatalogPlugin, authValue: String) {
        if (p.transport == "stdio") { note = "${p.name} 是 stdio 型，仅桌面可用，安卓跳过"; return }
        val header = if (p.authType == "header" && authValue.isNotBlank()) "{\"${p.authHeader}\":\"${authValue.replace("\"", "\\\"")}\"}" else ""
        note = "安装中…"
        Bg.run({ Mobilebind.mcpAddServer(p.name, p.endpoint, header) }) { r ->
            note = if (r == "OK") { onChanged(); "已安装 ${p.name}" } else r
            pendingAuthFor = null; pendingAuthVal = ""
        }
    }

    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text("目录源：" + (if (catalogOnline) "已连社区仓库" else "内置种子（可在 设置‑进阶 填社区源 URL）") + " · 已装 ${servers.size} 插件 / 可用 $toolCount 工具",
            fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)

        Text("可安装（市场目录）", fontWeight = FontWeight.Bold, fontSize = 14.sp)
        catalog.forEach { p ->
            Card(Modifier.fillMaxWidth()) {
                Column(Modifier.padding(12.dp)) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Column(Modifier.weight(1f)) {
                            Text(p.name, fontWeight = FontWeight.Bold, fontSize = 14.sp)
                            Text(p.description, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            Text("${p.transport} · " + (if (p.authType == "header") "需 Key" else "免 Key") +
                                if (p.transport == "stdio") " · 仅桌面" else "", fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                        Button(onClick = {
                            if (p.transport == "stdio") { note = "${p.name} 仅桌面可用"; return@Button }
                            if (p.authType == "header") { pendingAuthFor = p; pendingAuthVal = "" } else install(p, "")
                        }, enabled = !busy) { Text("安装") }
                    }
                    if (pendingAuthFor?.id == p.id) {
                        OutlinedTextField(pendingAuthVal, { pendingAuthVal = it }, label = { Text(p.authHint.ifBlank { "Key 值" }) }, modifier = Modifier.fillMaxWidth(), visualTransformation = PasswordVisualTransformation())
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            Button(onClick = { install(p, pendingAuthVal) }) { Text("确认安装") }
                            TextButton(onClick = { pendingAuthFor = null }) { Text("取消") }
                        }
                    }
                }
            }
        }

        Text("已装插件（开关=热插拔，不重启）", fontWeight = FontWeight.Bold, fontSize = 14.sp, modifier = Modifier.padding(top = 6.dp))
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
                            fontSize = 11.sp, color = if (s.err.isNotBlank()) Color.Red else MaterialTheme.colorScheme.primary,
                        )
                    }
                    Switch(checked = s.enabled, onCheckedChange = { v -> Bg.run({ Mobilebind.mcpSetEnabled(s.name, v) }) { onChanged() } })
                    TextButton(onClick = { Bg.run({ Mobilebind.mcpRemoveServer(s.name) }) { onChanged() } }) { Text("🗑") }
                }
            }
        }
        if (servers.isEmpty()) Text("尚未安装插件。", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)

        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(onClick = { showManual = !showManual }) { Text("＋ 手动添加") }
            OutlinedButton(onClick = { onChanged() }) { Text(if (busy) "刷新中…" else "刷新") }
        }
        if (showManual) {
            OutlinedTextField(mName, { mName = it }, label = { Text("名称") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
            OutlinedTextField(mEndpoint, { mEndpoint = it }, label = { Text("MCP 端点 URL（https，HTTP/SSE）") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
            OutlinedTextField(mHeader, { mHeader = it }, label = { Text("请求头 JSON（可选）") }, modifier = Modifier.fillMaxWidth(), visualTransformation = PasswordVisualTransformation())
            Button(onClick = {
                if (mName.trim().isEmpty() || mEndpoint.trim().isEmpty()) { note = "名称/端点必填"; return@Button }
                Bg.run({ Mobilebind.mcpAddServer(mName.trim(), mEndpoint.trim(), mHeader.trim()) }) { r ->
                    note = if (r == "OK") { mName = ""; mEndpoint = ""; mHeader = ""; showManual = false; onChanged(); "已安装" } else r
                }
            }) { Text("安装并连接") }
        }
        if (note.isNotEmpty()) Text(note, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Text("说明：插件运行在远端 MCP server，本体不在仓库；Git 仓库只放目录（endpoint）。stdio 型仅桌面。",
            fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

// —— 生态接入（三分档说明 + 用 Git 仓库当后端的说明） ——
@Composable
private fun EcosystemSection() {
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Text("生态接入", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold)
        Text("以 MCP 为边界吃生态；插件一律外部进程、可选可关可沙盒。", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        EcoTier("① MCP 原生 — 今天就能用", "任何以 MCP server 发布的能力直接挂，零代码。", true)
        EcoTier("② 数据/格式导入 — 已支持", "角色卡 V2 / 世界书 / 预设 / 主题包（数据非代码），本地导入。", true)
        EcoTier("③ 兼容服务器 — 需逐框架写，桌面侧", "给 AstrBot(Py)/SillyTavern(JS) 写独立进程兼容层，不进安卓内核。", false)
        EcoTier("④ 源码移植 — 人工重写", "要对齐具体能力就重写，最稳。", false)
        Card(Modifier.fillMaxWidth()) {
            Column(Modifier.padding(12.dp)) {
                Text("社区后端 = Git 仓库（当前形态）", fontWeight = FontWeight.Bold, fontSize = 13.sp)
                Text("market/index.json（目录）+ forum/posts.json（帖子）放仓库，客户端 HTTPS 拉取。零服务器、PR 审核即上架、可版本化。以后换真后端，返回同样 JSON 即可平滑切换。见 docs/community-ecosystem-contract.md。",
                    fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
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
