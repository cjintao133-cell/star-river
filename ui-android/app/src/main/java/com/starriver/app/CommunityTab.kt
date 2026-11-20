package com.starriver.app

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/** 社区：论坛体（帖子/回复） + MCP 插件集市。当前为本地骨相，联网/发帖接后端留阶段 C。 */
@Composable
fun CommunityTab() {
    var section by remember { mutableStateOf(0) }
    Column(Modifier.fillMaxSize().padding(12.dp)) {
        Text("社区", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
        Row(horizontalArrangement = Arrangement.spacedBy(6.dp), modifier = Modifier.padding(vertical = 8.dp)) {
            OutlinedButton(onClick = { section = 0 }) { Text("圈内论坛", fontWeight = if (section == 0) FontWeight.Bold else FontWeight.Normal) }
            OutlinedButton(onClick = { section = 1 }) { Text("MCP 集市", fontWeight = if (section == 1) FontWeight.Bold else FontWeight.Normal) }
        }
        if (section == 0) ForumSection() else MarketSection()
    }
}

private data class Post(val title: String, val author: String, val replies: Int, val snippet: String)

@Composable
private fun ForumSection() {
    val posts = remember {
        listOf(
            Post("示例角色伴侣上线三天，感觉她真会记仇 ", "用户", 12, "上次说错话，她隔天翻出来戳我…"),
            Post("求助：2GB 安卓跑动效背景怎么降级不卡", "新人阿柴", 7, "开了流星背景，切后台再回来就掉帧"),
            Post("角色卡 V2 世界书触发深度怎么设", "示例鱼罐头", 23, "递归到 4 层就开始灌水，大家一般设多少"),
            Post("自拍改头像效果分享（附参数）", "星河观察者", 31, "ComfyUI 走云 GPU，出图挺稳"),
        )
    }
    Column(Modifier.fillMaxWidth()) {
        Card(Modifier.fillMaxWidth()) {
            Row(Modifier.padding(12.dp), verticalAlignment = Alignment.CenterVertically) {
                Text("论坛为占位数据，联网发帖/回复待阶段 C 接入", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f))
                TextButton(onClick = { }) { Text("＋ 发帖") }
            }
        }
        LazyColumn(Modifier.fillMaxWidth().padding(top = 8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            items(posts) { p ->
                Card(Modifier.fillMaxWidth()) {
                    Column(Modifier.padding(12.dp)) {
                        Text(p.title, fontWeight = FontWeight.Bold, fontSize = 14.sp, maxLines = 2, overflow = TextOverflow.Ellipsis)
                        Text(p.snippet, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 2.dp))
                        Text("${p.author} · 💬 ${p.replies}", fontSize = 11.sp, color = MaterialTheme.colorScheme.primary, modifier = Modifier.padding(top = 4.dp))
                    }
                }
            }
        }
    }
}

private data class McpPlugin(val id: String, val name: String, val desc: String)

@Composable
private fun MarketSection() {
    val ctx = androidx.compose.ui.platform.LocalContext.current
    val plugins = remember {
        listOf(
            McpPlugin("filesystem", "文件系统 MCP", "本地文件读写，沙盒受控"),
            McpPlugin("brave-search", "Brave 搜索 MCP", "复用 kernel/tool，零代码接入"),
            McpPlugin("tavily", "Tavily 搜索", "摘要省 token，适合日常"),
            McpPlugin("comfyui", "ComfyUI 绘图", "云 GPU 出图，接自拍/生图"),
            McpPlugin("memory", "长期记忆", "伴侣插件包 hippocampus"),
        )
    }
    val installed = remember { mutableStateMapOf<String, Boolean>() }
    Column(Modifier.fillMaxWidth()) {
        Text("插件 key 在「设置‑进阶」填；启用走 MCP 热插拔（内核已备，前端接线待阶段 C）",
            fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(bottom = 8.dp))
        LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            items(plugins) { p ->
                Card(Modifier.fillMaxWidth()) {
                    Row(Modifier.padding(12.dp), verticalAlignment = Alignment.CenterVertically) {
                        Column(Modifier.weight(1f)) {
                            Text(p.name, fontWeight = FontWeight.Bold, fontSize = 14.sp)
                            Text(p.desc, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                        Switch(checked = installed[p.id] == true, onCheckedChange = { installed[p.id] = it })
                    }
                }
            }
        }
    }
}
