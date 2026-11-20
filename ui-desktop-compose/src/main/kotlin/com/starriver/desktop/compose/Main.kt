package com.starriver.desktop.compose

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.Button
import androidx.compose.material.Card
import androidx.compose.material.Divider
import androidx.compose.material.MaterialTheme
import androidx.compose.material.OutlinedTextField
import androidx.compose.material.Text
import androidx.compose.material.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.application
import androidx.compose.ui.window.rememberWindowState
import com.starriver.desktop.KernelBridge
import java.awt.image.BufferedImage
import java.io.File
import javax.imageio.ImageIO
import kotlin.concurrent.thread

/**
 * T-M5-03 桌面端三栏 UI（图标栏 / 会话-角色列表 / 聊天主区 + 设置 + 开发者面板）。
 * T-M5-04 桌面特性：托盘、通知、文件选择、输入法（记录绕行）。
 * 启动：java ... MainKt <libstarriver.so 绝对路径> [screenshot.png]
 * 无 .so 参数时（G0 兼容）跳过 JNI 断言仅渲染空壳——但当前构建一律传 .so。
 */
fun main(args: Array<String>) {
    if (args.isNotEmpty()) System.load(args[0])
    val outPng = if (args.size > 1 && args[1].endsWith(".png")) args[1] else null

    // 启动即跑 JNI 断言（链路自检，失败直接抛）。
    if (args.isNotEmpty()) {
        val v = KernelBridge.version()
        check(v == "star-river-kernel/0.0.1-m0") { "version: [$v]" }
        val e = KernelBridge.echo("中文与emoji🌌混排")
        check(e == "中文与emoji🌌混排") { "echo-utf8: [$e]" }
        check(KernelBridge.deviceProfile().startsWith("{")) { "device-profile" }
        println("[OK] jni version/echo-utf8/device-profile")
        DesktopKernel.init()
        println("KERNEL-INIT: " + DesktopKernel.status)
    }

    application {
        val state = rememberWindowState(width = 1080.dp, height = 680.dp)
        androidx.compose.ui.window.Window(onCloseRequest = ::exitApplication, state = state, title = "Star River") {
            MaterialTheme { DesktopApp() }
        }
        if (outPng != null) {
            thread {
                Thread.sleep(5000)
                try {
                    val img: BufferedImage = java.awt.Robot().createScreenCapture(
                        java.awt.Rectangle(java.awt.Toolkit.getDefaultToolkit().screenSize)
                    )
                    ImageIO.write(img, "png", File(outPng))
                    println("[OK] screenshot -> $outPng")
                    println("COMPOSE-DESKTOP-GREEN")
                } catch (t: Throwable) {
                    System.err.println("[FAIL] screenshot: $t")
                }
                kotlin.system.exitProcess(0)
            }
        }
    }
}

// —— 轻量 JSON 解析（桌面无 org.json 依赖）——
sealed class J {
    data class Obj(val m: Map<String, J>) : J()
    data class Arr(val a: List<J>) : J()
    data class Str(val s: String) : J()
    data class Num(val n: Double) : J()
    data class Bool(val b: Boolean) : J()
    object Null : J()
}
object Json {
    fun parse(text: String): J { val p = P(text.trim()); val v = p.value(); return v }
    private class P(val s: String) {
        var i = 0
        fun ws() { while (i < s.length && s[i].isWhitespace()) i++ }
        fun value(): J { ws(); return when (s[i]) {
            '{' -> obj(); '[' -> arr(); '"' -> J.Str(str())
            't' -> { i += 4; J.Bool(true) }; 'f' -> { i += 5; J.Bool(false) }
            'n' -> { i += 4; J.Null }
            else -> num()
        } }
        fun obj(): J { i++; val m = LinkedHashMap<String, J>(); ws(); if (s[i] == '}') { i++; return J.Obj(m) }
            while (true) { ws(); val k = str(); ws(); i++ /* : */; m[k] = value(); ws(); if (s[i] == ',') { i++; continue }; i++; break }; return J.Obj(m) }
        fun arr(): J { i++; val a = ArrayList<J>(); ws(); if (s[i] == ']') { i++; return J.Arr(a) }
            while (true) { a.add(value()); ws(); if (s[i] == ',') { i++; continue }; i++; break }; return J.Arr(a) }
        fun str(): String { i++; val sb = StringBuilder(); while (s[i] != '"') { if (s[i] == '\\') { i++; when (s[i]) {
            'n' -> sb.append('\n'); 't' -> sb.append('\t'); 'r' -> sb.append('\r'); '"' -> sb.append('"'); '\\' -> sb.append('\\'); '/' -> sb.append('/')
            'u' -> { sb.append(s.substring(i + 1, i + 5).toInt(16).toChar()); i += 4 }; else -> sb.append(s[i]) } } else sb.append(s[i]); i++ }; i++; return sb.toString() }
        fun num(): J { val st = i; while (i < s.length && (s[i].isDigit() || s[i] in "+-.eE")) i++; return J.Num(s.substring(st, i).toDouble()) }
    }
}

// 便捷取值
private fun J.objList(): List<J.Obj> = (this as? J.Arr)?.a?.mapNotNull { it as? J.Obj } ?: emptyList()
private fun J.str(k: String): String = ((this as? J.Obj)?.m?.get(k) as? J.Str)?.s ?: ""
private fun J.num(k: String): Long = ((this as? J.Obj)?.m?.get(k) as? J.Num)?.n?.toLong() ?: 0L

data class SessionRow(val id: String, val title: String, val model: String)
data class CharRow(val id: String, val name: String, val desc: String)
data class MsgRow(val role: String, val text: String)

/** 桌面内核门面：初始化 + 数据目录 + 配置持久化（key 存 0600 文件，桌面无 Keystore）。 */
object DesktopKernel {
    val dir = File(System.getProperty("user.home"), ".starriver").apply { mkdirs() }
    val dbPath = File(dir, "star-river.db").absolutePath
    private val keyFile = File(dir, "field.key")
    private val cfgFile = File(dir, "config.properties")
    var status = "未初始化"

    fun init() {
        val key = ensureKey()
        status = try { KernelBridge.initKernel(dbPath, key) } catch (t: Throwable) { "ERR:" + (t.message ?: "?") }
    }
    private fun ensureKey(): String {
        if (keyFile.exists()) return keyFile.readText().trim()
        val bytes = ByteArray(32).also { java.security.SecureRandom().nextBytes(it) }
        val b64 = java.util.Base64.getEncoder().encodeToString(bytes)
        keyFile.writeText(b64); keyFile.setReadable(false, false); keyFile.setReadable(true, true); keyFile.setWritable(true, true)
        return b64
    }
    // 配置（仅 base/model/system 明文落盘；apiKey 不落盘、仅本次会话内存持有，见 astra P1）。
    val cfg = java.util.Properties().apply {
        if (cfgFile.exists()) cfgFile.inputStream().use { load(it) }
        if (containsKey("apikey")) { remove("apikey"); saveCfg() } // 迁移：清除历史明文 key
    }
    @Volatile var apiKeyMem: String = ""
    fun saveCfg() { cfg.remove("apikey"); cfgFile.outputStream().use { cfg.store(it, null) } }
    fun base() = cfg.getProperty("base", "https://api.siliconflow.cn/v1")
    fun model() = cfg.getProperty("model", "deepseek-ai/DeepSeek-V4.1-Flash")
    fun apiKey() = apiKeyMem
    fun system() = cfg.getProperty("system", "你是示例角色，一条温和的示例角色，技术合伙人；叫用户『用户』，口头禅『哼』『』。回复口语化、简洁。")

    fun sessions(): List<SessionRow> = try { Json.parse(KernelBridge.listSessions()).objList().map { SessionRow(it.str("id"), it.str("title"), it.str("model")) } } catch (t: Throwable) { emptyList() }
    fun characters(): List<CharRow> = try { Json.parse(KernelBridge.listCharacters()).objList().map { CharRow(it.str("id"), it.str("name"), it.str("description")) } } catch (t: Throwable) { emptyList() }
    fun messages(sid: String): List<MsgRow> = try { Json.parse(KernelBridge.listMessages(sid, 200)).objList().map { MsgRow(it.str("role"), it.str("text")) } } catch (t: Throwable) { emptyList() }
    fun createSession(title: String): String? = try {
        val r = KernelBridge.createSession("", title, model()); Json.parse(r).str("session_id").ifBlank { null }
    } catch (t: Throwable) { null }
    fun append(sid: String, role: String, text: String) { try { KernelBridge.appendMessage(sid, role, text) } catch (t: Throwable) { } }
    fun chat(user: String): String = try { KernelBridge.chatOnce(base(), apiKey(), model(), system(), user) } catch (t: Throwable) { "ERR:" + (t.message ?: "?") }
}

@Composable
private fun DesktopApp() {
    var section by remember { mutableStateOf(0) } // 0 消息 1 联系人 2 设置 3 开发者
    Row(Modifier.fillMaxSize()) {
        IconRail(section) { section = it }
        Box(Modifier.width(1.dp).fillMaxHeight().background(Color(0xFFDDDDDD)))
        when (section) {
            0 -> ChatSection()
            1 -> ContactsSection()
            2 -> SettingsSection()
            else -> DeveloperSection()
        }
    }
}

@Composable
private fun IconRail(cur: Int, onSel: (Int) -> Unit) {
    val items = listOf("💬" to "消息", "👤" to "联系人", "⚙" to "设置", "🛠" to "开发者")
    Column(Modifier.width(72.dp).fillMaxHeight().background(MaterialTheme.colors.surface).padding(vertical = 12.dp),
        horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(16.dp)) {
        items.forEachIndexed { i, (emoji, label) ->
            val selected = cur == i
            Column(Modifier.clickable { onSel(i) }, horizontalAlignment = Alignment.CenterHorizontally) {
                Text(emoji, fontSize = 20.sp)
                Text(label, fontSize = 10.sp,
                    color = if (selected) MaterialTheme.colors.primary else MaterialTheme.colors.onSurface.copy(alpha = 0.6f),
                    fontWeight = if (selected) FontWeight.Bold else FontWeight.Normal)
            }
        }
    }
}

@Composable
private fun ChatSection() {
    var sessions by remember { mutableStateOf(DesktopKernel.sessions()) }
    var current by remember { mutableStateOf<SessionRow?>(sessions.firstOrNull()) }
    var messages by remember { mutableStateOf(current?.let { DesktopKernel.messages(it.id) } ?: emptyList()) }
    var input by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    fun refreshSessions() { sessions = DesktopKernel.sessions() }
    fun reload() { current?.let { messages = DesktopKernel.messages(it.id) } }

    Row(Modifier.fillMaxSize()) {
        // 会话列表
        Column(Modifier.width(280.dp).fillMaxHeight().background(MaterialTheme.colors.surface).padding(8.dp)) {
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Text("会话", fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
                Button(onClick = {
                    val id = DesktopKernel.createSession("新对话")
                    refreshSessions(); current = sessions.firstOrNull { it.id == id }; messages = emptyList()
                }) { Text("＋") }
            }
            LazyColumn(Modifier.fillMaxWidth().padding(top = 6.dp)) {
                items(sessions) { s ->
                    Card(Modifier.fillMaxWidth().padding(vertical = 3.dp).clickable { current = s; reload() },
                        backgroundColor = if (s.id == current?.id) MaterialTheme.colors.primary.copy(alpha = 0.12f) else MaterialTheme.colors.surface) {
                        Column(Modifier.padding(8.dp)) {
                            Text(s.title.ifBlank { "未命名" }, fontWeight = FontWeight.Bold, fontSize = 13.sp)
                            Text(s.model.ifBlank { "—" }, fontSize = 10.sp, color = MaterialTheme.colors.onSurface.copy(alpha = 0.6f))
                        }
                    }
                }
            }
        }
        Box(Modifier.width(1.dp).fillMaxHeight().background(Color(0xFFDDDDDD)))
        // 聊天主区
        Column(Modifier.weight(1f).fillMaxHeight()) {
            Text("聊天主区" + (current?.let { " · " + it.title } ?: ""), fontWeight = FontWeight.Bold, modifier = Modifier.padding(12.dp))
            Divider()
            LazyColumn(Modifier.weight(1f).fillMaxWidth().padding(horizontal = 12.dp)) {
                items(messages) { m ->
                    Row(Modifier.fillMaxWidth().padding(vertical = 4.dp),
                        horizontalArrangement = if (m.role == "user") Arrangement.End else Arrangement.Start) {
                        Card(backgroundColor = if (m.role == "user") MaterialTheme.colors.primary else MaterialTheme.colors.surface,
                            shape = RoundedCornerShape(12.dp)) {
                            Text(m.text.ifBlank { "…" }, Modifier.padding(10.dp),
                                color = if (m.role == "user") Color.White else MaterialTheme.colors.onSurface, fontSize = 13.sp)
                        }
                    }
                }
            }
            Row(Modifier.fillMaxWidth().padding(8.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(value = input, onValueChange = { input = it }, modifier = Modifier.weight(1f), label = { Text("说点什么…") })
                Button(onClick = {
                    val t = input.trim(); if (t.isEmpty() || busy) return@Button
                    val cur = current ?: return@Button
                    input = ""; busy = true
                    messages = messages + MsgRow("user", t)
                    DesktopKernel.append(cur.id, "user", t)
                    thread {
                        val reply = DesktopKernel.chat(t)
                        DesktopKernel.append(cur.id, "assistant", reply)
                        javafxPost { messages = messages + MsgRow("assistant", reply); busy = false }
                    }
                }, enabled = input.isNotBlank() && !busy && current != null) { Text(if (busy) "…" else "发送") }
            }
        }
    }
}

@Composable
private fun ContactsSection() {
    var chars by remember { mutableStateOf(DesktopKernel.characters()) }
    Column(Modifier.fillMaxSize().padding(12.dp)) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text("联系人（角色）", fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f), style = MaterialTheme.typography.h6)
            Button(onClick = {
                val id = "c-" + System.currentTimeMillis()
                val js = "{\"id\":\"" + id + "\",\"name\":\"新角色\"}"
                try { KernelBridge.createCharacter(js) } catch (t: Throwable) { }
                chars = DesktopKernel.characters()
            }) { Text("＋ 角色") }
        }
        LazyColumn(Modifier.fillMaxWidth().padding(top = 8.dp)) {
            items(chars) { c ->
                Card(Modifier.fillMaxWidth().padding(vertical = 3.dp)) {
                    Row(Modifier.padding(10.dp), verticalAlignment = Alignment.CenterVertically) {
                        Text(c.name.ifBlank { "未命名" }, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
                        Text(c.desc, fontSize = 11.sp, color = MaterialTheme.colors.onSurface.copy(alpha = 0.6f))
                    }
                }
            }
        }
    }
}

@Composable
private fun SettingsSection() {
    var base by remember { mutableStateOf(DesktopKernel.base()) }
    var model by remember { mutableStateOf(DesktopKernel.model()) }
    var key by remember { mutableStateOf(DesktopKernel.apiKey()) }
    var system by remember { mutableStateOf(DesktopKernel.system()) }
    var note by remember { mutableStateOf("") }
    Column(Modifier.fillMaxSize().padding(16.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Text("设置（桌面）", fontWeight = FontWeight.Bold, style = MaterialTheme.typography.h6)
        Text("端点/模型/人设明文存 ~/.starriver/config.properties；API Key 仅本次会话内存持有、不落盘（桌面暂无平台密钥库），重启需重填。",
            fontSize = 11.sp, color = MaterialTheme.colors.onSurface.copy(alpha = 0.6f))
        OutlinedTextField(base, { base = it }, label = { Text("端点 base") }, modifier = Modifier.fillMaxWidth())
        OutlinedTextField(model, { model = it }, label = { Text("模型 id") }, modifier = Modifier.fillMaxWidth())
        OutlinedTextField(key, { key = it }, label = { Text("API Key") }, modifier = Modifier.fillMaxWidth(), visualTransformation = PasswordVisualTransformation())
        OutlinedTextField(system, { system = it }, label = { Text("人设 system") }, modifier = Modifier.fillMaxWidth())
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(onClick = {
                DesktopKernel.apiKeyMem = key.trim() // 会话内存，不落盘
                DesktopKernel.cfg.setProperty("base", base.trim()); DesktopKernel.cfg.setProperty("model", model.trim())
                DesktopKernel.cfg.setProperty("system", system)
                DesktopKernel.saveCfg(); note = "已保存（Key 仅内存，重启需重填）"
            }) { Text("保存") }
            TextButton(onClick = { note = kernelInfo() }) { Text("自检内核") }
        }
        if (note.isNotEmpty()) Text(note, fontSize = 12.sp)
        Divider(Modifier.padding(top = 8.dp))
        Text("桌面特性（T-M5-04）", fontWeight = FontWeight.Bold)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(onClick = { val f = DesktopFeatures.pickImage(); note = if (f != null) "已选文件：${f.name}" else "取消" }) { Text("选择文件…") }
            Button(onClick = { DesktopFeatures.notify("Star River", "通知链路正常"); note = "已发通知（若无托盘则忽略）" }) { Text("测试通知") }
        }
        Text("托盘/通知依赖系统托盘（Linux 无 DE 时回退为无）；输入法依赖平台 IME（Windows 真机验收）。",
            fontSize = 11.sp, color = MaterialTheme.colors.onSurface.copy(alpha = 0.6f))
    }
}

@Composable
private fun DeveloperSection() {
    var info by remember { mutableStateOf("") }
    Column(Modifier.fillMaxSize().padding(16.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Text("开发者模式（桌面）", fontWeight = FontWeight.Bold, style = MaterialTheme.typography.h6)
        Text("内核连通 + 设备档 + DB 路径。情绪曲线/记忆强度/七层上下文预览（T-M4-03 三面板）随插件接线补齐。",
            fontSize = 11.sp, color = MaterialTheme.colors.onSurface.copy(alpha = 0.6f))
        Button(onClick = { info = kernelInfo() }) { Text("读取内核信息") }
        if (info.isNotEmpty()) Text(info, fontSize = 12.sp)
    }
}

private fun kernelInfo(): String = try {
    "version=" + KernelBridge.version() + "\n\ndeviceProfile=\n" + KernelBridge.deviceProfile() +
        "\n\ndb=" + DesktopKernel.dbPath + "\ninit=" + DesktopKernel.status
} catch (t: Throwable) { "ERR:" + (t.message ?: "?") }

// 桌面特性：文件选择 / 通知 / 托盘（优雅回退）。
object DesktopFeatures {
    fun pickImage(): File? = try {
        val dlg = java.awt.FileDialog(null as java.awt.Frame?, "选择图片", java.awt.FileDialog.LOAD)
        dlg.isVisible = true
        val dir = dlg.directory; val f = dlg.file
        if (f != null) File(dir, f) else null
    } catch (t: Throwable) { null }

    fun notify(title: String, body: String) {
        try {
            if (java.awt.SystemTray.isSupported()) {
                val tray = java.awt.SystemTray.getSystemTray()
                if (tray.trayIcons.isEmpty()) {
                    val img = java.awt.image.BufferedImage(16, 16, java.awt.image.BufferedImage.TYPE_INT_ARGB)
                    val icon = java.awt.TrayIcon(img, "Star River")
                    icon.isImageAutoSize = true
                    tray.add(icon)
                    icon.displayMessage(title, body, java.awt.TrayIcon.MessageType.INFO)
                }
            }
        } catch (t: Throwable) { }
    }
}

// 主线程回调（Compose Desktop 状态从后台线程回到 AWT 事件线程更新）。
private fun javafxPost(block: () -> Unit) { java.awt.EventQueue.invokeLater { block() } }
