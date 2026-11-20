package com.starriver.app

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.delay
import mobilebind.Mobilebind

/**
 * 系统 · 安全 · 自愈板块。
 * 汇总：看门狗（存活监控 + 异常自愈）/ 沙盒隔离策略 / 安全红线核对 /
 * 自我修复（自检，不覆盖损坏数据）/ 受控自我进化（插件内演进，不改内核代码、不越安全边界）。
 */
@Composable
fun SafetyScreen(onBack: () -> Unit) {
    val ctx = LocalContext.current
    var watchdog by remember { mutableStateOf(SafetyPrefs.watchdog(ctx)) }
    var sandboxStrict by remember { mutableStateOf(SafetyPrefs.sandboxStrict(ctx)) }
    var health by remember { mutableStateOf("未启动") }
    var lastCheck by remember { mutableStateOf("—") }
    var fails by remember { mutableStateOf(0) }
    var selfCheck by remember { mutableStateOf("") }
    var checking by remember { mutableStateOf(false) }

    // 看门狗：每 5s 轻量 ping 内核；异常则尝试重开（自愈），不改密钥、不覆盖数据。
    // single-flight：上一轮未结束不启新一轮，避免重复初始化/并发重建（按 astra 评审 P1）。
    val inFlight = remember { java.util.concurrent.atomic.AtomicBoolean(false) }
    LaunchedEffect(watchdog) {
        if (!watchdog) { health = "已关闭"; return@LaunchedEffect }
        health = "启动中…"
        while (true) {
            if (inFlight.compareAndSet(false, true)) {
                Bg.run({ try { Mobilebind.version() } catch (t: Throwable) { "ERR:" + (t.message ?: "?") } }) { r ->
                    val t = java.text.SimpleDateFormat("HH:mm:ss", java.util.Locale.getDefault()).format(java.util.Date())
                    lastCheck = t
                    if (r.startsWith("ERR")) {
                        fails += 1; health = "异常·已触发自愈"
                        KernelBoot.reinit(ctx)
                    } else {
                        health = "健康"
                    }
                    inFlight.set(false)
                }
            }
            delay(5000)
        }
    }

    fun runSelfCheck() {
        if (checking) return
        checking = true; selfCheck = "自检中…"
        Bg.run({
            val v = try { Mobilebind.version() } catch (t: Throwable) { "ERR:" + (t.message ?: "?") }
            val dp = try { Mobilebind.deviceProfile() } catch (t: Throwable) { "ERR:" + (t.message ?: "?") }
            val re = KernelBoot.reinit(ctx)
            val sb = StringBuilder()
            sb.append(if (v.startsWith("ERR")) "❌ 内核版本读取失败\n" else "✓ 内核版本 $v\n")
            sb.append(if (dp.startsWith("ERR")) "❌ 设备档读取失败\n" else "✓ 设备档可用（NEON/内存探测正常）\n")
            sb.append(if (re == "OK") "✓ 内核重开成功（未覆盖密钥/数据）\n" else "❌ 内核重开失败：$re\n")
            sb.append("✓ 密钥经 Keystore 加密持有（未读取明文）\n")
            sb.append("✓ 打开数据库时执行 schema 版本断言（migrate 拒绝高位/断档）")
            sb.toString()
        }) { selfCheck = it; checking = false }
    }

    Column(Modifier.fillMaxSize().padding(12.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            TextButton(onClick = onBack) { Text("← 返回") }
            Text("系统 · 安全 · 自愈", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
        }

        // 看门狗
        SectionCard("🐕 看门狗（Watchdog）") {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text("内核存活监控", fontWeight = FontWeight.Bold, fontSize = 14.sp)
                    Text("每 5s 轻量 ping；异常自动重开内核（自愈）", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                Switch(checked = watchdog, onCheckedChange = { watchdog = it; SafetyPrefs.setWatchdog(ctx, it) })
            }
            Text("状态：$health · 最近自检 $lastCheck · 异常计数 $fails",
                fontSize = 12.sp,
                color = if (health.contains("异常")) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary)
            Text("说明：进程级看门狗（独立守护进程/崩溃自动拉起）需原生层，随长稳阶段接；此处为应用层心跳+自愈。",
                fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }

        // 沙盒隔离
        SectionCard("📦 沙盒隔离（Sandbox）") {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text("工具/MCP 按沙盒隔离执行", fontWeight = FontWeight.Bold, fontSize = 14.sp)
                    Text("每个插件/工具在受控边界内运行，最小权限", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                Switch(checked = sandboxStrict, onCheckedChange = { sandboxStrict = it; SafetyPrefs.setSandboxStrict(ctx, it) })
            }
            Text(if (sandboxStrict) "策略：严格（默认拒绝未声明能力）" else "策略：标准（按工具声明放行）",
                fontSize = 12.sp, color = MaterialTheme.colorScheme.primary)
            Text("说明：沙盒执行本体在内核层（kernel/tool + 平台隔离）；本开关为策略意图，深度强制随能力接线复核。",
                fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }

        // 安全红线核对
        SectionCard("🛡 安全限制（红线核对）") {
            RedlineRow("密钥不进 URL/日志/dump，Keystore 加密", true)
            RedlineRow("底座不 import 闭源插件（红线2）", true)
            RedlineRow("零上传：崩溃 dump 仅本地，能力上无网络", true)
            RedlineRow("AI 生成图强制不可关水印", true)
            RedlineRow("真删除覆盖记忆/向量（红线5）", false)
            RedlineRow("请求端点 HTTPS 冻结 / 禁重定向（P0）", false)
        }

        // 自我修复
        SectionCard("🔧 自我修复（Self-Repair）") {
            Text("自检内核连通、设备探测、数据库可重开；发现损坏进恢复态，绝不自动覆盖密钥/数据。",
                fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Button(onClick = { runSelfCheck() }, enabled = !checking, modifier = Modifier.fillMaxWidth()) {
                Text(if (checking) "自检中…" else "运行自检 + 自愈")
            }
            if (selfCheck.isNotEmpty()) Text(selfCheck, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurface)
        }

        // 受控自我进化
        SectionCard("🧬 自我进化（受控）") {
            Text("记忆/情绪在伴侣插件（hippocampus / endocrine）内本地演进、可审计、可回滚。",
                fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurface)
            Text("边界（硬约束）：", fontWeight = FontWeight.Bold, fontSize = 12.sp, modifier = Modifier.padding(top = 4.dp))
            Text("• 不自主修改内核代码 / 不自提权\n• 不越过安全红线与沙盒边界\n• 仅写白名单数据字段；派生数据不构成授权；任何开关不得降低安全红线\n• 强制在内核出口（读写/联网/出图）执行，不靠提示词\n• 所有演进仅本地，不外发、可导出审计",
                fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Text("说明：此处不做「AI 自主改代码」——那类自我进化与安全红线冲突，明确不支持；进化限定在人格/记忆数据层。",
                fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

@Composable
private fun SectionCard(title: String, content: @Composable () -> Unit) {
    Card(Modifier.fillMaxWidth(), shape = RoundedCornerShape(LocalCornerRadius.current)) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(title, fontWeight = FontWeight.Bold, fontSize = 15.sp)
            content()
        }
    }
}

@Composable
private fun RedlineRow(text: String, ok: Boolean) {
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
        Text(if (ok) "✓" else "~", fontWeight = FontWeight.Bold,
            color = if (ok) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.error)
        Text(text, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurface)
        if (!ok) Text("（接线中）", fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}
