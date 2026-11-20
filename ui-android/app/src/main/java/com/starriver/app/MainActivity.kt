package com.starriver.app

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/**
 * 星海容器 · Android 入口。
 * 阶段 B：四栏血肉 —— 消息（会话列表+持久气泡+流式）、联系人（角色卡全字段+导入+头像）、
 * 设置（新手/进阶/开发者三层）、社区（论坛体+MCP 集市）。内核自启见 KernelBoot。
 */
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        KernelBoot.ensure(this)
        val initial = ConfigStore.load(this)
        setContent {
            StarRiverApp(initialConfig = initial) { cfg -> ConfigStore.save(this, cfg) }
        }
    }
}

@Composable
private fun StarRiverApp(initialConfig: ChatConfig, persist: (ChatConfig) -> Unit) {
    var config by remember { mutableStateOf(initialConfig) }
    var tab by remember { mutableStateOf(0) }
    var themeMode by remember { mutableStateOf(ThemeMode.DAY) }
    val app = remember { AppState() }
    LaunchedEffect(Unit) { app.reload() }

    AppTheme(themeMode) {
        Column(Modifier.fillMaxSize()) {
            Box(Modifier.weight(1f).fillMaxWidth()) {
                when (tab) {
                    0 -> MessageTab(config, app) { tab = 3 }
                    1 -> ContactsTab(config, app) { tab = 0 }
                    2 -> CommunityTab()
                    else -> SettingsTab(
                        config = config,
                        themeMode = themeMode,
                        onThemeChange = { themeMode = it },
                        onSave = { cfg -> config = cfg; persist(cfg) },
                    )
                }
            }
            BottomBar(tab) { tab = it }
        }
    }
}

@Composable
private fun BottomBar(cur: Int, onSel: (Int) -> Unit) {
    val items = listOf("💬 消息", "👤 联系人", "🌐 社区", "⚙ 设置")
    Row(
        Modifier.fillMaxWidth().background(MaterialTheme.colorScheme.surface).padding(vertical = 4.dp),
        horizontalArrangement = Arrangement.SpaceEvenly,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        items.forEachIndexed { idx, label ->
            TextButton(onClick = { onSel(idx) }) {
                Text(
                    label, fontSize = 12.sp,
                    fontWeight = if (cur == idx) FontWeight.Bold else FontWeight.Normal,
                    color = if (cur == idx) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}
