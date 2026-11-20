package com.starriver.app

import android.content.Intent
import android.net.Uri
import android.os.Handler
import android.os.Looper
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import mobilebind.ChatSink
import mobilebind.Mobilebind
import org.json.JSONArray
import org.json.JSONObject

@Composable
fun MessageTab(config: ChatConfig, app: AppState, onOpenSettings: () -> Unit) {
    var searchOpen by remember { mutableStateOf(false) }
    if (searchOpen) {
        SearchScreen(config) { searchOpen = false }
        return
    }
    val sid = app.currentSessionId
    if (sid == null) SessionList(config, app) { searchOpen = true } else ChatView(config, app, sid)
}

/** QQ 式会话列表：新建 / 搜索 / 点进 / 删除。 */
@Composable
private fun SessionList(config: ChatConfig, app: AppState, onSearch: () -> Unit) {
    var tick by remember { mutableStateOf(0) }
    LaunchedEffect(tick) { app.reload() }
    Column(Modifier.fillMaxSize().padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text("消息", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
            TextButton(onClick = onSearch) { Text("🔍 搜索") }
            Button(onClick = {
                Bg.run({ (app.newSession("", "新对话", config.model, config.system)?.let { "OK" }) ?: "ERR:create" }) { tick++ }
            }) { Text("＋ 新对话") }
        }
        if (app.sessions.isEmpty()) {
            Box(Modifier.weight(1f).fillMaxWidth(), contentAlignment = Alignment.Center) {
                Text("还没有会话，点右上「＋ 新对话」，或去联系人点角色开始", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 13.sp)
            }
        } else {
            LazyColumn(Modifier.weight(1f).fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                items(app.sessions, key = { it.id }) { s ->
                    SessionRow(s,
                        onOpen = { app.currentSessionId = s.id },
                        onDelete = { Bg.run({ app.delete(s.id); "OK" }) { tick++ } },
                    )
                }
            }
        }
    }
}

@Composable
private fun SessionRow(s: SessionUi, onOpen: () -> Unit, onDelete: () -> Unit) {
    Card(Modifier.fillMaxWidth().clickable { onOpen() }) {
        Row(Modifier.padding(12.dp), verticalAlignment = Alignment.CenterVertically) {
            AvatarCircle(s.title.firstOrNull()?.toString() ?: "?", 40)
            Column(Modifier.weight(1f).padding(start = 10.dp)) {
                Text(s.title.ifBlank { "未命名" }, fontWeight = FontWeight.Bold, fontSize = 15.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text(s.model.ifBlank { "—" }, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
            TextButton(onClick = onDelete, modifier = Modifier.semantics { contentDescription = "删除会话 ${s.title}" }) { Text("🗑", fontSize = 16.sp) }
        }
    }
}

/** 进聊：顶栏返回 / 气泡流 / 输入栏。消息持久化走 appendMessage/listMessages。 */
@Composable
private fun ChatView(config: ChatConfig, app: AppState, sid: String) {
    val msgs = remember(sid) { mutableStateListOf<MsgUi>() }
    LaunchedEffect(sid) { msgs.clear(); msgs.addAll(loadMessages(sid)) }
    var input by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    val h = remember { Handler(Looper.getMainLooper()) }
    val listState = rememberLazyListState()
    val title = app.sessions.firstOrNull { it.id == sid }?.title ?: "对话"
    val system = app.sessionSystem[sid] ?: config.system

    LaunchedEffect(msgs.size) {
        if (msgs.isNotEmpty()) listState.animateScrollToItem(msgs.size - 1)
    }

    fun send() {
        val text = input.trim()
        if (text.isEmpty() || busy) return
        input = ""
        busy = true
        msgs.add(MsgUi("mu-" + System.nanoTime(), "user", text, false))
        Bg.run({ Mobilebind.appendMessage(sid, "user", text) }) {}
        msgs.add(MsgUi("ma-" + System.nanoTime(), "assistant", "", true))
        val idx = msgs.size - 1
        val target = StringBuilder()
        var revealed = 0
        var finished = false
        var errMsg: String? = null
        var stopped = false
        lateinit var tick: Runnable
        tick = Runnable {
            if (stopped) return@Runnable
            val e = errMsg
            if (e != null) {
                msgs[idx] = msgs[idx].copy(text = e, streaming = false); busy = false; stopped = true; return@Runnable
            }
            val full = target.toString()
            if (revealed < full.length) {
                val backlog = full.length - revealed
                val step = if (backlog > 80) 4 else if (backlog > 30) 2 else 1
                revealed = minOf(full.length, revealed + step)
                msgs[idx] = msgs[idx].copy(text = full.substring(0, revealed), streaming = true)
                h.postDelayed(tick, 18)
            } else if (finished) {
                msgs[idx] = msgs[idx].copy(text = full, streaming = false)
                busy = false; stopped = true
                Bg.run({ Mobilebind.appendMessage(sid, "assistant", full) }) {}
            } else {
                h.postDelayed(tick, 18)
            }
        }
        val sink = object : ChatSink {
            override fun onDelta(s: String) { h.post { target.append(s) } }
            override fun onDone(model: String) { h.post { finished = true } }
            override fun onError(msg: String) { h.post { errMsg = "[错误] " + msg } }
        }
        Bg.run({ Mobilebind.chatStream(config.baseUrl, config.apiKey, config.model, system, text, sink); "OK" }) {}
        h.postDelayed(tick, 18)
    }

    Column(Modifier.fillMaxSize()) {
        Row(Modifier.fillMaxWidth().background(MaterialTheme.colorScheme.surface).padding(horizontal = 6.dp, vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
            TextButton(onClick = { app.currentSessionId = null }) { Text("← 返回") }
            Text(title.ifBlank { "对话" }, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        LazyColumn(
            modifier = Modifier.weight(1f).fillMaxWidth().padding(horizontal = 12.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
            state = listState,
        ) {
            items(msgs.size) { i -> MessageBubble(msgs[i], config) }
        }
        Row(Modifier.fillMaxWidth().padding(8.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            Text("🎤", fontSize = 20.sp, modifier = Modifier.semantics { contentDescription = "语音输入（待接线）" }.clickable { })
            OutlinedTextField(
                value = input, onValueChange = { input = it },
                modifier = Modifier.weight(1f), label = { Text("说点什么…") }, maxLines = 4,
            )
            Button(onClick = { send() }, enabled = input.isNotBlank() && !busy) { Text(if (busy) "…" else "发送") }
        }
    }
}

@Composable
private fun MessageBubble(m: MsgUi, config: ChatConfig) {
    val ctx = LocalContext.current
    val isUser = m.role == "user"
    val bubbleColor = if (isUser) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.surfaceVariant
    val textColor = if (isUser) MaterialTheme.colorScheme.onPrimary else MaterialTheme.colorScheme.onSurface
    val shown = if (m.streaming && m.text.isEmpty()) "…" else m.text
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = if (isUser) Arrangement.End else Arrangement.Start,
    ) {
        if (!isUser) AvatarCircle("溟", 30)
        Box(
            modifier = Modifier
                .widthIn(max = 300.dp)
                .padding(horizontal = if (isUser) 0.dp else 6.dp)
                .background(bubbleColor, RoundedCornerShape(12.dp))
                .padding(horizontal = 12.dp, vertical = 8.dp),
        ) {
            Column {
                Text(shown, color = textColor)
                if (!isUser && !m.streaming && m.text.isNotBlank()) {
                    Text(
                        "🔊 朗读", fontSize = 11.sp, color = MaterialTheme.colorScheme.primary,
                        modifier = Modifier.padding(top = 4.dp).clickable { speak(ctx, config, m.text) },
                    )
                }
            }
        }
    }
}

/** TTS 朗读（真实调用 ttsSynthesize，写入缓存临时文件用 MediaPlayer 播放）。 */
private fun speak(ctx: android.content.Context, config: ChatConfig, text: String) {
    if (config.baseUrl.isBlank() || config.apiKey.isBlank()) return
    val voice = Prefs.ttsVoice(ctx)
    val fmt = Prefs.ttsFormat(ctx)
    Bg.run({ Mobilebind.ttsSynthesize(config.baseUrl, config.apiKey, text, voice, fmt) }) { raw ->
        if (raw.startsWith("ERR:")) return@run
        try {
            val o = JSONObject(raw)
            val b64 = o.optString("audio_base64")
            val bytes = android.util.Base64.decode(b64, android.util.Base64.DEFAULT)
            val f = java.io.File(ctx.cacheDir, "tts_" + System.currentTimeMillis() + "." + fmt)
            f.writeBytes(bytes)
            val mp = android.media.MediaPlayer()
            mp.setDataSource(f.absolutePath)
            mp.setOnCompletionListener { it.release(); f.delete() }
            mp.prepare()
            mp.start()
        } catch (t: Throwable) { }
    }
}

private data class SearchHit(val title: String, val url: String, val snippet: String, val thumb: String)

/** 类百度网页搜索：接 webSearch（SearXNG/Brave，配置在设置‑进阶）。 */
@Composable
private fun SearchScreen(config: ChatConfig, onBack: () -> Unit) {
    val ctx = LocalContext.current
    var query by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    var note by remember { mutableStateOf("") }
    var hits by remember { mutableStateOf<List<SearchHit>>(emptyList()) }

    fun runSearch() {
        val q = query.trim()
        if (q.isEmpty() || busy) return
        val provider = Prefs.searchProvider(ctx)
        val base = Prefs.searxngBase(ctx)
        val braveKey = SecretStore.get(ctx, "search.brave_key") ?: ""
        if (provider == "searxng" && base.isBlank()) { note = "请先在 设置‑进阶 填 SearXNG base"; return }
        if (provider == "brave" && braveKey.isBlank()) { note = "请先在 设置‑进阶 填 Brave Key"; return }
        busy = true; note = "搜索中…"; hits = emptyList()
        Bg.run({ Mobilebind.webSearch(provider, base, braveKey, q, 8L, "") }) { raw ->
            if (raw.startsWith("ERR:")) { note = "失败：" + raw.removePrefix("ERR:") }
            else {
                val out = mutableListOf<SearchHit>()
                try {
                    val arr = JSONArray(raw)
                    for (i in 0 until arr.length()) {
                        val o = arr.optJSONObject(i) ?: continue
                        out.add(SearchHit(o.optString("title"), o.optString("url"), o.optString("snippet"), o.optString("thumbnail")))
                    }
                } catch (e: Exception) { }
                hits = out
                note = if (out.isEmpty()) "无结果" else "共 ${out.size} 条"
            }
            busy = false
        }
    }

    Column(Modifier.fillMaxSize().padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            TextButton(onClick = onBack) { Text("← 返回") }
            Text("网页搜索", fontWeight = FontWeight.Bold, fontSize = 18.sp)
        }
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedTextField(value = query, onValueChange = { query = it }, modifier = Modifier.weight(1f), label = { Text("搜点什么…") }, singleLine = true)
            Button(onClick = { runSearch() }, enabled = !busy && query.isNotBlank()) { Text(if (busy) "…" else "搜索") }
        }
        Text("源：" + Prefs.searchProvider(ctx) + "（设置‑进阶可改）", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        if (note.isNotEmpty()) Text(note, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            items(hits) { h ->
                Card(Modifier.fillMaxWidth().clickable {
                    try { ctx.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(h.url))) } catch (t: Throwable) { }
                }) {
                    Column(Modifier.padding(12.dp)) {
                        Text(h.title.ifBlank { h.url }, fontWeight = FontWeight.Bold, fontSize = 14.sp, maxLines = 2, overflow = TextOverflow.Ellipsis)
                        Text(h.url, fontSize = 11.sp, color = MaterialTheme.colorScheme.primary, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        if (h.snippet.isNotBlank()) Text(h.snippet, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 3, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 3.dp))
                    }
                }
            }
        }
    }
}

@Composable
fun AvatarCircle(letter: String, size: Int) {
    Box(
        Modifier
            .widthIn(min = size.dp, max = size.dp)
            .background(MaterialTheme.colorScheme.secondary, RoundedCornerShape(50))
            .padding(vertical = 8.dp),
        contentAlignment = Alignment.Center,
    ) {
        Text(letter, color = MaterialTheme.colorScheme.onSecondary, fontWeight = FontWeight.Bold, fontSize = (size / 2.4).sp)
    }
}
