package com.starriver.app

import android.os.Bundle
import android.os.Handler
import android.os.Looper
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
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
import mobilebind.ChatSink
import mobilebind.Mobilebind
import java.util.concurrent.atomic.AtomicLong

/** UI 消息（不可变；流式更新靠替换列表元素触发重组）。 */
data class UiMessage(
    val id: Long,
    val role: String, // "user" | "assistant"
    val text: String,
    val streaming: Boolean = false,
)

/** 会话（内存态；消息持久化接入留后续任务）。 */
class UiSession(val id: Long, var title: String) {
    val messages = mutableStateListOf<UiMessage>()
}

private val idGen = AtomicLong(1L)

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val initial = ConfigStore.load(this)
        setContent {
            MaterialTheme {
                Surface(modifier = Modifier.fillMaxSize()) {
                    App(initialConfig = initial) { cfg -> ConfigStore.save(this, cfg) }
                }
            }
        }
    }
}

@Composable
private fun App(initialConfig: ChatConfig, persist: (ChatConfig) -> Unit) {
    var config by remember { mutableStateOf(initialConfig) }
    var editing by remember { mutableStateOf(!initialConfig.isConfigured) }
    // 会话状态提升到顶层：切到设置页再回来，聊天记录不丢。
    val sessions = remember { mutableStateListOf(UiSession(idGen.getAndIncrement(), "对话 1")) }
    var currentId by remember { mutableStateOf(sessions.first().id) }
    var busy by remember { mutableStateOf(false) }
    var input by remember { mutableStateOf("") }

    if (editing) {
        SettingsScreen(
            initial = config,
            canCancel = config.isConfigured,
            onCancel = { editing = false },
            onSave = { cfg ->
                config = cfg
                persist(cfg)
                editing = false
            },
        )
    } else {
        ChatScreen(
            config = config,
            sessions = sessions,
            currentId = currentId,
            onSelect = { currentId = it },
            busy = busy,
            setBusy = { busy = it },
            input = input,
            setInput = { input = it },
            onOpenSettings = { editing = true },
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SettingsScreen(
    initial: ChatConfig,
    canCancel: Boolean,
    onCancel: () -> Unit,
    onSave: (ChatConfig) -> Unit,
) {
    var base by remember { mutableStateOf(initial.baseUrl) }
    var key by remember { mutableStateOf(initial.apiKey) }
    var model by remember { mutableStateOf(initial.model) }
    var system by remember { mutableStateOf(initial.system) }

    Column(modifier = Modifier.fillMaxSize().padding(16.dp)) {
        Text("设置", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
        Text("填入中转/端点（OpenAI 兼容）即可聊天", fontSize = 12.sp, color = Color.Gray)
        Column(modifier = Modifier.padding(vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            OutlinedTextField(value = base, onValueChange = { base = it }, label = { Text("端点 base（如 https://api.siliconflow.cn/v1）") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
            OutlinedTextField(value = key, onValueChange = { key = it }, label = { Text("API Key") }, modifier = Modifier.fillMaxWidth(), singleLine = true, visualTransformation = PasswordVisualTransformation())
            OutlinedTextField(value = model, onValueChange = { model = it }, label = { Text("模型 id") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
            OutlinedTextField(value = system, onValueChange = { system = it }, label = { Text("人设 system") }, modifier = Modifier.fillMaxWidth(), minLines = 3)
        }
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            Button(onClick = {
                onSave(ChatConfig(base.trim(), key.trim(), model.trim(), system))
            }, enabled = base.isNotBlank() && key.isNotBlank() && model.isNotBlank()) {
                Text("保存并开始")
            }
            if (canCancel) {
                TextButton(onClick = onCancel) { Text("取消") }
            }
        }
    }
}

@Composable
private fun ChatScreen(
    config: ChatConfig,
    sessions: MutableList<UiSession>,
    currentId: Long,
    onSelect: (Long) -> Unit,
    busy: Boolean,
    setBusy: (Boolean) -> Unit,
    input: String,
    setInput: (String) -> Unit,
    onOpenSettings: () -> Unit,
) {
    val uiHandler = remember { Handler(Looper.getMainLooper()) }
    val current = sessions.firstOrNull { it.id == currentId } ?: sessions.first()

    fun send() {
        val text = input.trim()
        if (text.isEmpty() || busy) return
        setInput("")
        setBusy(true)
        val msgs = current.messages
        msgs.add(UiMessage(idGen.getAndIncrement(), "user", text))
        val placeholder = UiMessage(idGen.getAndIncrement(), "assistant", "", streaming = true)
        msgs.add(placeholder)
        val idx = msgs.size - 1

        // 打字机节奏（拟真延迟）：网络再快也按节奏吐字，短回复顺、长回复追得上。
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
                msgs[idx] = msgs[idx].copy(text = e, streaming = false)
                setBusy(false)
                stopped = true
                return@Runnable
            }
            val full = target.toString()
            if (revealed < full.length) {
                val backlog = full.length - revealed
                val step = if (backlog > 80) 4 else if (backlog > 30) 2 else 1
                revealed = minOf(full.length, revealed + step)
                msgs[idx] = msgs[idx].copy(text = full.substring(0, revealed), streaming = true)
                uiHandler.postDelayed(tick, 18)
            } else if (finished) {
                msgs[idx] = msgs[idx].copy(text = full, streaming = false)
                setBusy(false)
                stopped = true
            } else {
                uiHandler.postDelayed(tick, 18)
            }
        }

        val sink = object : ChatSink {
            override fun onDelta(s: String) {
                uiHandler.post { target.append(s) }
            }
            override fun onDone(model: String) {
                uiHandler.post { finished = true }
            }
            override fun onError(msg: String) {
                uiHandler.post { errMsg = "[错误] " + msg }
            }
        }
        uiHandler.post(tick)
        Thread {
            try {
                Mobilebind.chatStream(config.baseUrl, config.apiKey, config.model, config.system, text, sink)
            } catch (t: Throwable) {
                uiHandler.post { errMsg = "[异常] " + (t.message ?: "unknown") }
            }
        }.start()
    }

    Column(modifier = Modifier.fillMaxSize().imePadding()) {
        Row(
            modifier = Modifier.fillMaxWidth().background(Color(0xFF1E88E5)).padding(horizontal = 12.dp, vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text("示例角色", color = Color.White, fontWeight = FontWeight.Bold, fontSize = 18.sp, modifier = Modifier.weight(1f))
            TextButton(onClick = {
                val s = UiSession(idGen.getAndIncrement(), "对话 ${sessions.size + 1}")
                sessions.add(s)
                onSelect(s.id)
            }) { Text("＋新对话", color = Color.White) }
            TextButton(onClick = onOpenSettings) { Text("设置", color = Color.White) }
        }

        if (sessions.size > 1) {
            Row(modifier = Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 4.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                sessions.forEach { s ->
                    val selected = s.id == currentId
                    TextButton(onClick = { onSelect(s.id) }) {
                        Text(s.title, fontWeight = if (selected) FontWeight.Bold else FontWeight.Normal, fontSize = 12.sp)
                    }
                }
            }
        }

        LazyColumn(
            modifier = Modifier.weight(1f).fillMaxWidth().padding(horizontal = 12.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            items(current.messages, key = { it.id }) { m -> MessageBubble(m) }
        }

        Row(
            modifier = Modifier.fillMaxWidth().padding(8.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            OutlinedTextField(
                value = input, onValueChange = setInput,
                modifier = Modifier.weight(1f), label = { Text("说点什么…") }, maxLines = 4,
            )
            Button(onClick = { send() }, enabled = input.isNotBlank() && !busy) {
                Text(if (busy) "…" else "发送")
            }
        }
    }
}

@Composable
private fun MessageBubble(m: UiMessage) {
    val isUser = m.role == "user"
    val bubbleColor = if (isUser) Color(0xFF1E88E5) else Color(0xFFEEEEEE)
    val textColor = if (isUser) Color.White else Color.Black
    val shown = if (m.streaming && m.text.isEmpty()) "…" else m.text
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = if (isUser) Arrangement.End else Arrangement.Start,
    ) {
        Box(
            modifier = Modifier
                .widthIn(max = 300.dp)
                .background(bubbleColor, RoundedCornerShape(12.dp))
                .padding(horizontal = 12.dp, vertical = 8.dp),
        ) {
            Text(shown, color = textColor)
        }
    }
}
