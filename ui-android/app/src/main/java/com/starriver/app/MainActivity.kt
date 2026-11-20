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
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/**
 * 星海容器 · Android 入口。
 * 阶段 B：四栏血肉。阶段 C：流星动效背景/主题/语言/权限/举报/系统安全自愈板块。
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
    val ctx = LocalContext.current
    var config by remember { mutableStateOf(initialConfig) }
    var tab by remember { mutableStateOf(0) }
    var theme by remember { mutableStateOf(ThemePrefs.load(ctx)) }
    var reduceMotion by remember { mutableStateOf(UiPrefs.reduceMotion(ctx)) }
    var lang by remember { mutableStateOf(LangPrefs.lang(ctx)) }
    AppI18n.load(ctx, lang)
    val app = remember { AppState() }
    LaunchedEffect(Unit) { app.reload() }

    AppTheme(theme) {
        key(lang, theme.mode, theme.accent, theme.radiusDp) {
            val body: @Composable () -> Unit = {
                Column(Modifier.fillMaxSize()) {
                    Box(Modifier.weight(1f).fillMaxWidth()) {
                        when (tab) {
                            0 -> MessageTab(config, app) { tab = 3 }
                            1 -> ContactsTab(config, app) { tab = 0 }
                            2 -> CommunityTab()
                            else -> SettingsTab(
                                config = config,
                                theme = theme,
                                reduceMotion = reduceMotion,
                                lang = lang,
                                onThemeChange = { theme = it; ThemePrefs.save(ctx, it) },
                                onReduceMotionChange = { reduceMotion = it; UiPrefs.setReduceMotion(ctx, it) },
                                onLangChange = { lang = it; LangPrefs.setLang(ctx, it) },
                                onSave = { cfg -> config = cfg; persist(cfg) },
                            )
                        }
                    }
                    BottomBar(tab) { tab = it }
                }
            }
            if (theme.mode == ThemeMode.NIGHT) {
                StarryBackdrop(reduceMotion = reduceMotion) { body() }
            } else {
                Box(Modifier.fillMaxSize().background(MaterialTheme.colorScheme.background)) { body() }
            }
        }
    }
}

@Composable
private fun BottomBar(cur: Int, onSel: (Int) -> Unit) {
    val items = listOf(
        "💬 " + AppI18n.t("nav.messages"),
        "👤 " + AppI18n.t("nav.contacts"),
        "🌐 " + AppI18n.t("nav.community"),
        "⚙ " + AppI18n.t("nav.settings"),
    )
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
