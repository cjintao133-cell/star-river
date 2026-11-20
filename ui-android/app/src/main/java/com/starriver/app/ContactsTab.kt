package com.starriver.app

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import mobilebind.Mobilebind
import org.json.JSONArray
import org.json.JSONObject

/** 联系人：角色列表 → 点角色按人设新建会话；＋ 分离「创建角色 / 导入角色卡」两入口。 */
@Composable
fun ContactsTab(config: ChatConfig, app: AppState, onOpenMessages: () -> Unit) {
    val ctx = LocalContext.current
    var list by remember { mutableStateOf<List<CharUi>>(emptyList()) }
    var err by remember { mutableStateOf("") }
    var tick by remember { mutableStateOf(0) }
    var showForm by remember { mutableStateOf(false) }
    var formTarget by remember { mutableStateOf<CharUi?>(null) }
    var formIsNew by remember { mutableStateOf(true) }
    var showAlbum by remember { mutableStateOf(false) }
    if (showAlbum) { AlbumScreen(app) { showAlbum = false }; return }

    fun refresh() {
        Bg.run({ Mobilebind.listCharacters() }) { raw ->
            if (raw.startsWith("ERR:")) err = raw else { err = ""; list = parseChars(raw) }
        }
    }
    androidx.compose.runtime.LaunchedEffect(tick) { refresh() }

    val importLauncher = rememberLauncherForActivityResult(ActivityResultContracts.GetContent()) { uri ->
        if (uri != null) {
            try {
                val text = ctx.contentResolver.openInputStream(uri)!!.bufferedReader().use { it.readText() }
                val c = parseCardJson(text)
                if (c != null) { formTarget = c; formIsNew = true; showForm = true } else err = "角色卡解析失败（需 Character Card V2 JSON）"
            } catch (t: Throwable) { err = "读取失败：" + (t.message ?: "?") }
        }
    }

    if (showForm) {
        CharacterFormScreen(
            initial = formTarget, isNew = formIsNew, config = config,
            onCancel = { showForm = false; formTarget = null },
            onSaved = { showForm = false; formTarget = null; tick++ },
        )
        return
    }

    Column(Modifier.fillMaxSize().padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text("联系人（角色）", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(onClick = { formTarget = null; formIsNew = true; showForm = true }) { Text("＋ 创建角色") }
            OutlinedButton(onClick = { importLauncher.launch("application/json") }) { Text("导入角色卡") }
            OutlinedButton(onClick = { importLauncher.launch("*/*") }) { Text("更多格式") }
            OutlinedButton(onClick = { showAlbum = true }) { IconCamera(MaterialTheme.colorScheme.onSurface, 18.dp) }
        }
        if (err.isNotEmpty()) Text(err, color = Color.Red, fontSize = 12.sp)
        if (list.isEmpty()) {
            Box(Modifier.weight(1f).fillMaxWidth(), contentAlignment = Alignment.Center) {
                Text("还没有角色。点「＋ 创建角色」或「导入角色卡」", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 13.sp)
            }
        } else {
            LazyColumn(Modifier.weight(1f).fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                items(list, key = { it.id }) { c ->
                    Card(Modifier.fillMaxWidth().clickable {
                        Bg.run({
                            val sys = buildCharacterSystem(c)
                            val sid = app.newSession(c.id, c.name, config.model, sys)
                            if (sid != null && c.firstMes.isNotBlank()) Mobilebind.appendMessage(sid, "assistant", c.firstMes)
                            if (sid != null) "OK" else "ERR:session"
                        }) { r -> if (!r.startsWith("ERR:")) onOpenMessages() }
                    }) {
                        Row(Modifier.padding(12.dp), verticalAlignment = Alignment.CenterVertically) {
                            AvatarCircle(c.name.firstOrNull()?.toString() ?: "?", 42)
                            Column(Modifier.weight(1f).padding(start = 10.dp)) {
                                Text(c.name.ifBlank { "未命名" }, fontWeight = FontWeight.Bold, fontSize = 15.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                val sub = c.description.ifBlank { c.personality }
                                if (sub.isNotBlank()) Text(sub, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2, overflow = TextOverflow.Ellipsis)
                            }
                            TextButton(onClick = { formTarget = c; formIsNew = false; showForm = true }, modifier = Modifier.semantics { contentDescription = "编辑角色 ${c.name}" }) { Text("✏") }
                            TextButton(onClick = { Bg.run({ Mobilebind.deleteCharacter(c.id) }) { tick++ } }, modifier = Modifier.semantics { contentDescription = "删除角色 ${c.name}" }) { Text("🗑") }
                        }
                    }
                }
            }
        }
    }
}

/** 角色卡 V2 JSON → CharUi（支持顶层或 data 包裹；生成新 id）。 */
private fun parseCardJson(text: String): CharUi? {
    return try {
        var o = JSONObject(text)
        if (o.has("data") && o.optJSONObject("data") != null) o = o.getJSONObject("data")
        val name = o.optString("name", "").ifBlank { return null }
        val alt = mutableListOf<String>()
        val ag = o.optJSONArray("alternate_greetings")
        if (ag != null) for (i in 0 until ag.length()) alt.add(ag.optString(i))
        val book = o.opt("character_book")
        val bookStr = when (book) {
            is JSONObject -> book.toString()
            is String -> book
            else -> ""
        }
        CharUi(
            id = "c-" + System.currentTimeMillis(),
            name = name,
            description = o.optString("description", ""),
            personality = o.optString("personality", ""),
            scenario = o.optString("scenario", ""),
            firstMes = o.optString("first_mes", ""),
            alternateGreetings = alt,
            mesExample = o.optString("mes_example", ""),
            systemPrompt = o.optString("system_prompt", ""),
            postHistory = o.optString("post_history_instructions", ""),
            characterBook = bookStr,
        )
    } catch (e: Exception) { null }
}

/** 角色表单：全字段 + 头像/人设图上传。新建用 generate id，编辑用原 id。 */
@Composable
private fun CharacterFormScreen(
    initial: CharUi?,
    isNew: Boolean,
    config: ChatConfig,
    onCancel: () -> Unit,
    onSaved: () -> Unit,
) {
    val ctx = LocalContext.current
    var name by remember { mutableStateOf(initial?.name ?: "") }
    var description by remember { mutableStateOf(initial?.description ?: "") }
    var personality by remember { mutableStateOf(initial?.personality ?: "") }
    var scenario by remember { mutableStateOf(initial?.scenario ?: "") }
    var firstMes by remember { mutableStateOf(initial?.firstMes ?: "") }
    var altGreet by remember { mutableStateOf(initial?.alternateGreetings?.joinToString("\n") ?: "") }
    var mesExample by remember { mutableStateOf(initial?.mesExample ?: "") }
    var systemPrompt by remember { mutableStateOf(initial?.systemPrompt ?: "") }
    var postHistory by remember { mutableStateOf(initial?.postHistory ?: "") }
    var characterBook by remember { mutableStateOf(initial?.characterBook ?: "") }
    var avatarBytes by remember { mutableStateOf<ByteArray?>(null) }
    var personaBytes by remember { mutableStateOf<ByteArray?>(null) }
    var status by remember { mutableStateOf("") }
    val id = remember { initial?.id?.ifBlank { "c-" + System.currentTimeMillis() } ?: ("c-" + System.currentTimeMillis()) }

    val avatarPicker = rememberLauncherForActivityResult(ActivityResultContracts.GetContent()) { uri ->
        if (uri != null) try { avatarBytes = ctx.contentResolver.openInputStream(uri)!!.use { it.readBytes() }; status = "已选头像（保存后生效）" } catch (t: Throwable) { status = "头像读取失败" }
    }
    val personaPicker = rememberLauncherForActivityResult(ActivityResultContracts.GetContent()) { uri ->
        if (uri != null) try { personaBytes = ctx.contentResolver.openInputStream(uri)!!.use { it.readBytes() }; status = "已选人设图（保存后生效）" } catch (t: Throwable) { status = "人设图读取失败" }
    }

    fun save() {
        if (name.isBlank()) { status = "名字必填"; return }
        val o = JSONObject()
        o.put("id", id)
        o.put("name", name.trim())
        o.put("description", description)
        o.put("personality", personality)
        o.put("scenario", scenario)
        o.put("first_mes", firstMes)
        val altList = altGreet.split("\n").map { it.trim() }.filter { it.isNotEmpty() }
        if (altList.isNotEmpty()) o.put("alternate_greetings", JSONArray(altList))
        o.put("mes_example", mesExample)
        o.put("system_prompt", systemPrompt)
        o.put("post_history_instructions", postHistory)
        if (characterBook.isNotBlank()) o.put("character_book", characterBook)
        val js = o.toString()
        status = "保存中…"
        Bg.run({
            val r = if (isNew) Mobilebind.createCharacter(js) else Mobilebind.updateCharacter(js)
            if (r != "OK") return@run r
            avatarBytes?.let { Mobilebind.setCharacterAvatar(id, it) }
            personaBytes?.let { Mobilebind.setCharacterPersona(id, it) }
            "OK"
        }) { r ->
            if (r == "OK") onSaved() else status = r
        }
    }

    Column(Modifier.fillMaxSize().padding(12.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(if (isNew) "创建角色" else "编辑角色", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
            TextButton(onClick = onCancel) { Text("取消") }
        }
        AvatarCircle(name.firstOrNull()?.toString() ?: "?", 56)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(onClick = { avatarPicker.launch("image/*") }) { Text(if (avatarBytes != null) "头像 ✓" else "上传头像") }
            OutlinedButton(onClick = { personaPicker.launch("image/*") }) { Text(if (personaBytes != null) "人设图 ✓" else "上传人设图") }
        }
        Field("名字 *", name) { name = it }
        Field("设定 description", description, min = 3) { description = it }
        Field("性格 personality", personality, min = 2) { personality = it }
        Field("场景 scenario", scenario, min = 2) { scenario = it }
        Field("开场白 first_mes", firstMes, min = 2) { firstMes = it }
        Field("备选开场白（每行一条）", altGreet, min = 2) { altGreet = it }
        Field("对话示例 mes_example", mesExample, min = 2) { mesExample = it }
        Field("系统提示 system_prompt", systemPrompt, min = 2) { systemPrompt = it }
        Field("历史后置 post_history", postHistory, min = 2) { postHistory = it }
        Field("世界书 character_book (JSON)", characterBook, min = 2) { characterBook = it }
        if (status.isNotEmpty()) Text(status, fontSize = 12.sp, color = if (status.startsWith("ERR") || status.contains("失败") || status.contains("必填")) Color.Red else MaterialTheme.colorScheme.primary)
        Button(onClick = { save() }, modifier = Modifier.fillMaxWidth()) { Text("保存角色") }
    }
}

@Composable
private fun Field(label: String, value: String, min: Int = 1, onChange: (String) -> Unit) {
    OutlinedTextField(
        value = value, onValueChange = onChange, label = { Text(label) },
        modifier = Modifier.fillMaxWidth(), minLines = min, maxLines = 6,
    )
}
