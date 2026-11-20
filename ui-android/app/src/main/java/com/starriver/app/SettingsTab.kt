package com.starriver.app

/*
 * SettingsTab.kt（重写）
 * 变更：
 *  1) 端点区只留 base / API Key / 模型 id；人设 system 输入框已移除（保存时原样透传 config.system）。
 *  2) 校验端点 / 拉取模型：点击即禁用并显示进行中；结果在顶部固定状态区 + Toast；模型列表渲染为可点胶囊。
 *  3) 失败原因红字加粗，并提示 base 与 key 需属于同一家服务商。
 *  4) 去除全部 emoji，改为本文件内 Canvas 线性图标（StIcon，可迁入 AppIcons.kt）。
 *  5) 三层结构与 Permission/Report/Safety/Panels/Diag 子屏调用保持不变。
 *
 * i18n：文案一律经 tr(key, fallback) → AppI18n.t(key)。
 *  下方每个 tr("settings.x.*", "...") 的 key 都是待补到 kernel/i18n zh.json / en.json 的新 key，
 *  第二个参数即 zh 值；en 值需另补。真源补齐后可删除 fallback。
 */

import android.widget.Toast
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import mobilebind.Mobilebind
import org.json.JSONArray
import org.json.JSONObject

/* ───────────────────────── i18n 辅助 ───────────────────────── */

@Composable
private fun tr(key: String, fallback: String): String {
    val v = AppI18n.t(key)
    return if (v.isBlank() || v == key) fallback else v
}

/* ───────────────────────── Canvas 线性图标 ───────────────────────── */

private enum class StGlyph { CHECK, CROSS, SHIELD, CHART, PULSE, CHEVRON, LOCK, FLAG }

@Composable
private fun StIcon(
    glyph: StGlyph,
    modifier: Modifier = Modifier.size(20.dp),
    tint: Color = MaterialTheme.colorScheme.onSurfaceVariant,
) {
    Canvas(modifier) {
        val w = size.minDimension
        val st = Stroke(width = w * 0.09f, cap = StrokeCap.Round, join = StrokeJoin.Round)
        fun line(x1: Float, y1: Float, x2: Float, y2: Float) {
            drawLine(tint, Offset(x1 * w, y1 * w), Offset(x2 * w, y2 * w), strokeWidth = st.width, cap = StrokeCap.Round)
        }
        fun poly(close: Boolean, vararg p: Float) {
            val path = Path()
            path.moveTo(p[0] * w, p[1] * w)
            var i = 2
            while (i + 1 < p.size) {
                path.lineTo(p[i] * w, p[i + 1] * w)
                i += 2
            }
            if (close) path.close()
            drawPath(path = path, color = tint, style = st)
        }
        when (glyph) {
            StGlyph.CHECK -> poly(false, 0.18f, 0.52f, 0.42f, 0.74f, 0.82f, 0.28f)
            StGlyph.CROSS -> {
                line(0.24f, 0.24f, 0.76f, 0.76f)
                line(0.76f, 0.24f, 0.24f, 0.76f)
            }
            StGlyph.SHIELD -> {
                poly(true, 0.5f, 0.1f, 0.84f, 0.24f, 0.84f, 0.5f, 0.74f, 0.72f, 0.5f, 0.9f, 0.26f, 0.72f, 0.16f, 0.5f, 0.16f, 0.24f)
                poly(false, 0.36f, 0.5f, 0.47f, 0.61f, 0.66f, 0.4f)
            }
            StGlyph.CHART -> {
                line(0.14f, 0.86f, 0.86f, 0.86f)
                line(0.28f, 0.7f, 0.28f, 0.54f)
                line(0.5f, 0.7f, 0.5f, 0.2f)
                line(0.72f, 0.7f, 0.72f, 0.4f)
            }
            StGlyph.PULSE -> poly(false, 0.08f, 0.55f, 0.3f, 0.55f, 0.4f, 0.25f, 0.54f, 0.8f, 0.64f, 0.55f, 0.92f, 0.55f)
            StGlyph.CHEVRON -> poly(false, 0.38f, 0.22f, 0.66f, 0.5f, 0.38f, 0.78f)
            StGlyph.LOCK -> {
                drawRoundRect(
                    color = tint,
                    topLeft = Offset(0.2f * w, 0.46f * w),
                    size = Size(0.6f * w, 0.42f * w),
                    cornerRadius = CornerRadius(0.08f * w, 0.08f * w),
                    style = st,
                )
                drawArc(
                    color = tint,
                    startAngle = 180f,
                    sweepAngle = 180f,
                    useCenter = false,
                    topLeft = Offset(0.32f * w, 0.12f * w),
                    size = Size(0.36f * w, 0.68f * w),
                    style = st,
                )
            }
            StGlyph.FLAG -> {
                line(0.24f, 0.12f, 0.24f, 0.9f)
                poly(false, 0.24f, 0.16f, 0.8f, 0.16f, 0.66f, 0.34f, 0.8f, 0.52f, 0.24f, 0.52f)
            }
        }
    }
}

/* ───────────────────────── 通用小组件 ───────────────────────── */

@Composable
private fun SectionTitle(text: String) {
    Text(
        text,
        fontWeight = FontWeight.Bold,
        fontSize = 15.sp,
        color = MaterialTheme.colorScheme.onSurface,
        modifier = Modifier.padding(top = 8.dp),
    )
}

@Composable
private fun Hint(text: String) {
    Text(text, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
}

@Composable
private fun ChoiceChip(label: String, selected: Boolean, onClick: () -> Unit) {
    val cs = MaterialTheme.colorScheme
    val shape = RoundedCornerShape(LocalCornerRadius.current)
    Box(
        Modifier
            .clip(shape)
            .background(if (selected) cs.primaryContainer else cs.surface)
            .border(1.dp, if (selected) cs.primary else cs.outline, shape)
            .clickable(onClick = onClick)
            .defaultMinSize(minHeight = 38.dp)
            .padding(horizontal = 14.dp, vertical = 8.dp),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            label,
            fontSize = 13.sp,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            fontWeight = if (selected) FontWeight.Bold else FontWeight.Normal,
            color = if (selected) cs.onPrimaryContainer else cs.onSurface,
        )
    }
}

@Composable
private fun TierSeg(label: String, selected: Boolean, modifier: Modifier, onClick: () -> Unit) {
    val cs = MaterialTheme.colorScheme
    val shape = RoundedCornerShape(LocalCornerRadius.current)
    Box(
        modifier
            .clip(shape)
            .background(if (selected) cs.primary else cs.surfaceVariant)
            .clickable(onClick = onClick)
            .padding(vertical = 10.dp),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            label,
            fontSize = 14.sp,
            maxLines = 1,
            fontWeight = if (selected) FontWeight.Bold else FontWeight.Normal,
            color = if (selected) cs.onPrimary else cs.onSurfaceVariant,
        )
    }
}

@Composable
private fun Field(
    value: String,
    onChange: (String) -> Unit,
    label: String,
    modifier: Modifier = Modifier.fillMaxWidth(),
    secret: Boolean = false,
) {
    OutlinedTextField(
        value = value,
        onValueChange = onChange,
        label = { Text(label, maxLines = 1, overflow = TextOverflow.Ellipsis) },
        modifier = modifier,
        singleLine = true,
        shape = RoundedCornerShape(LocalCornerRadius.current),
        visualTransformation = if (secret) PasswordVisualTransformation() else VisualTransformation.None,
    )
}

@Composable
private fun SwitchRow(title: String, sub: String?, checked: Boolean, strong: Boolean, onChange: (Boolean) -> Unit) {
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f)) {
            Text(
                title,
                fontSize = if (strong) 15.sp else 13.sp,
                fontWeight = if (strong) FontWeight.Bold else FontWeight.Normal,
                color = MaterialTheme.colorScheme.onSurface,
            )
            if (!sub.isNullOrEmpty()) Hint(sub)
        }
        Switch(checked = checked, onCheckedChange = onChange)
    }
}

/** 带进行中态的按钮：busy 时禁用并显示转圈 + 进行中文案。 */
@Composable
private fun ActionButton(
    label: String,
    busyLabel: String,
    busy: Boolean,
    enabled: Boolean,
    primary: Boolean = false,
    modifier: Modifier = Modifier,
    onClick: () -> Unit,
) {
    val shape = RoundedCornerShape(LocalCornerRadius.current)
    val content: @Composable () -> Unit = {
        if (busy) {
            CircularProgressIndicator(modifier = Modifier.size(14.dp), strokeWidth = 2.dp)
            Spacer(Modifier.width(6.dp))
        }
        Text(if (busy) busyLabel else label, maxLines = 1)
    }
    if (primary) {
        Button(onClick = onClick, enabled = enabled && !busy, shape = shape, modifier = modifier) { content() }
    } else {
        OutlinedButton(onClick = onClick, enabled = enabled && !busy, shape = shape, modifier = modifier) { content() }
    }
}

@Composable
private fun NavRow(glyph: StGlyph, label: String, onClick: () -> Unit) {
    val cs = MaterialTheme.colorScheme
    val shape = RoundedCornerShape(LocalCornerRadius.current)
    Row(
        Modifier
            .fillMaxWidth()
            .clip(shape)
            .background(cs.surfaceVariant)
            .clickable(onClick = onClick)
            .padding(horizontal = 14.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        StIcon(glyph, Modifier.size(22.dp), cs.primary)
        Text(label, modifier = Modifier.weight(1f), fontSize = 14.sp, color = cs.onSurface)
        StIcon(StGlyph.CHEVRON, Modifier.size(16.dp), cs.onSurfaceVariant)
    }
}

/** 固定状态区：空闲提示 / 进行中 / 成功 / 失败（红字 + 同一家提示）。kind: 0 无 1 成功 2 失败 */
@Composable
private fun StatusBanner(busy: Boolean, kind: Int, title: String, detail: String, idleText: String, failHint: String) {
    val cs = MaterialTheme.colorScheme
    val shape = RoundedCornerShape(LocalCornerRadius.current)
    val failed = !busy && kind == 2
    val ok = !busy && kind == 1
    val bg = if (ok) cs.primaryContainer else if (failed) cs.surface else cs.surfaceVariant
    val fg = if (ok) cs.onPrimaryContainer else if (failed) cs.error else cs.onSurfaceVariant
    Row(
        Modifier
            .fillMaxWidth()
            .clip(shape)
            .background(bg)
            .then(if (failed) Modifier.border(2.dp, cs.error, shape) else Modifier)
            .padding(12.dp),
        verticalAlignment = Alignment.Top,
        horizontalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        when {
            busy -> CircularProgressIndicator(modifier = Modifier.size(18.dp), strokeWidth = 2.dp)
            ok -> StIcon(StGlyph.CHECK, Modifier.size(20.dp), fg)
            failed -> StIcon(StGlyph.CROSS, Modifier.size(20.dp), fg)
            else -> StIcon(StGlyph.PULSE, Modifier.size(20.dp), fg)
        }
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(
                if (!busy && kind == 0) idleText else title,
                fontSize = if (failed) 15.sp else 13.sp,
                fontWeight = if (ok || failed) FontWeight.Bold else FontWeight.Normal,
                color = fg,
            )
            if (!busy && detail.isNotEmpty()) {
                Text(detail, fontSize = 12.sp, color = fg, maxLines = 5, overflow = TextOverflow.Ellipsis)
            }
            if (failed) {
                Text(failHint, fontSize = 12.sp, fontWeight = FontWeight.Bold, color = cs.error)
            }
        }
    }
}

private fun parseModelIds(raw: String): List<String>? = try {
    val t = raw.trim()
    val arr = if (t.startsWith("[")) JSONArray(t) else (JSONObject(t).optJSONArray("data") ?: JSONArray())
    val out = ArrayList<String>()
    for (i in 0 until arr.length()) {
        val id = arr.optJSONObject(i)?.optString("id") ?: arr.optString(i)
        if (!id.isNullOrBlank()) out.add(id)
    }
    out.distinct()
} catch (e: Exception) {
    null
}

/* ───────────────────────── 设置主屏 ───────────────────────── */

/** 设置三层：新手（端点）/ 进阶（主题·动效·搜索·语音·原生搜索）/ 开发者（合规·诊断·探测）。 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun SettingsTab(
    config: ChatConfig,
    theme: ThemeConfig,
    reduceMotion: Boolean,
    lang: String,
    followSystem: Boolean,
    onThemeChange: (ThemeConfig) -> Unit,
    onReduceMotionChange: (Boolean) -> Unit,
    onLangChange: (String) -> Unit,
    onFollowSystem: () -> Unit,
    onSave: (ChatConfig) -> Unit,
) {
    var tier by remember { mutableStateOf(0) }
    var screen by remember { mutableStateOf(0) }
    if (screen == 1) { PermissionScreen { screen = 0 }; return }
    if (screen == 2) { ReportScreen { screen = 0 }; return }
    if (screen == 3) { SafetyScreen { screen = 0 }; return }
    if (screen == 4) { PanelsScreen { screen = 0 }; return }
    if (screen == 5) { DiagScreen { screen = 0 }; return }

    val cs = MaterialTheme.colorScheme
    Column(Modifier.fillMaxSize().padding(horizontal = 16.dp, vertical = 12.dp)) {
        Text(
            AppI18n.t("settings.title"),
            style = MaterialTheme.typography.headlineSmall,
            fontWeight = FontWeight.Bold,
            color = cs.onBackground,
        )
        Spacer(Modifier.height(8.dp))
        Text(AppI18n.t("settings.language"), fontSize = 12.sp, color = cs.onSurfaceVariant)
        Spacer(Modifier.height(4.dp))
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            ChoiceChip(tr("settings.x.lang_follow", "跟随系统"), followSystem) { onFollowSystem() }
            // 语言名为各语言自称，不参与翻译
            ChoiceChip("中文", !followSystem && lang == "zh") { onLangChange("zh") }
            ChoiceChip("English", !followSystem && lang == "en") { onLangChange("en") }
        }
        Spacer(Modifier.height(12.dp))
        Row(
            Modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(LocalCornerRadius.current))
                .background(cs.surfaceVariant)
                .padding(4.dp),
            horizontalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            TierSeg(AppI18n.t("settings.tier.novice"), tier == 0, Modifier.weight(1f)) { tier = 0 }
            TierSeg(AppI18n.t("settings.tier.advanced"), tier == 1, Modifier.weight(1f)) { tier = 1 }
            TierSeg(AppI18n.t("settings.tier.developer"), tier == 2, Modifier.weight(1f)) { tier = 2 }
        }
        Spacer(Modifier.height(12.dp))
        Box(Modifier.weight(1f).fillMaxWidth()) {
            when (tier) {
                0 -> NoviceSettings(config, onSave)
                1 -> AdvancedSettings(theme, reduceMotion, onThemeChange, onReduceMotionChange)
                else -> DeveloperSettings(
                    config,
                    onOpenPermission = { screen = 1 },
                    onOpenReport = { screen = 2 },
                    onOpenSafety = { screen = 3 },
                    onOpenPanels = { screen = 4 },
                    onOpenDiag = { screen = 5 },
                )
            }
        }
    }
}

/* ───────────────────────── 新手：端点 / Key / 模型 ───────────────────────── */

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun NoviceSettings(config: ChatConfig, onSave: (ChatConfig) -> Unit) {
    val ctx = LocalContext.current
    var base by remember { mutableStateOf(config.baseUrl) }
    var key by remember { mutableStateOf(config.apiKey) }
    var model by remember { mutableStateOf(config.model) }
    var busy by remember { mutableStateOf(0) }      // 0 空闲 1 校验中 2 拉模型中
    var kind by remember { mutableStateOf(0) }      // 0 无 1 成功 2 失败
    var title by remember { mutableStateOf("") }
    var detail by remember { mutableStateOf("") }
    var filter by remember { mutableStateOf("") }
    val models = remember { mutableStateListOf<String>() }

    val sIdle = tr("settings.x.endpoint_idle", "填好端点与 Key 后，可先校验端点或拉取模型列表；结果显示在这里。")
    val sProbing = tr("settings.x.probing", "校验中…")
    val sProbe = tr("settings.x.probe", "校验端点")
    val sProbeOk = tr("settings.x.probe_ok", "端点可用")
    val sProbeFail = tr("settings.x.probe_fail", "校验失败")
    val sListing = tr("settings.x.listing", "拉取中…")
    val sList = tr("settings.x.list_models", "拉取模型")
    val sListOk = tr("settings.x.list_ok", "已拉取模型，点选下方胶囊填入")
    val sListFail = tr("settings.x.list_fail", "拉取模型失败")
    val sListEmpty = tr("settings.x.list_empty", "端点未返回任何模型")
    val sFailHint = tr("settings.x.same_vendor_hint", "请确认端点 base 与 API Key 属于同一家服务商。")
    val sSaved = tr("settings.x.saved", "已保存")
    val sModelLabel = AppI18n.t("settings.model")
    val errLabels = mapOf(
        "network" to AppI18n.t("error.network"),
        "timeout" to AppI18n.t("error.timeout"),
        "auth" to AppI18n.t("error.auth"),
        "rate_limit" to AppI18n.t("error.rate_limit"),
        "server" to AppI18n.t("error.server"),
        "protocol" to AppI18n.t("error.protocol"),
    )

    fun toast(msg: String) {
        Toast.makeText(ctx, msg, Toast.LENGTH_SHORT).show()
    }

    fun probe() {
        if (busy != 0) return
        val b = base.trim(); val k = key.trim(); val m = model.trim()
        busy = 1; kind = 0; title = sProbing; detail = ""
        Bg.run({ Mobilebind.probeEndpoint(b, k, m) }) { raw ->
            busy = 0
            try {
                val o = JSONObject(raw)
                if (o.optBoolean("ok")) {
                    kind = 1; title = sProbeOk
                    detail = sModelLabel + " " + o.optString("model").ifBlank { m }
                } else {
                    val cat = o.optString("category")
                    val err = o.optString("error")
                    kind = 2; title = sProbeFail
                    detail = listOf(errLabels[cat] ?: cat, err).filter { it.isNotBlank() }.joinToString("\n")
                }
            } catch (e: Exception) {
                kind = 2; title = sProbeFail; detail = raw.removePrefix("ERR:")
            }
            toast(title)
        }
    }

    fun listModels() {
        if (busy != 0) return
        val b = base.trim(); val k = key.trim()
        busy = 2; kind = 0; title = sListing; detail = ""
        Bg.run({ Mobilebind.listModels(b, k) }) { raw ->
            busy = 0
            val ids = if (raw.startsWith("ERR:")) null else parseModelIds(raw)
            if (ids == null) {
                kind = 2; title = sListFail; detail = raw.removePrefix("ERR:")
            } else if (ids.isEmpty()) {
                kind = 2; title = sListEmpty; detail = ""
            } else {
                models.clear(); models.addAll(ids); filter = ""
                kind = 1; title = sListOk; detail = "× " + ids.size
            }
            toast(title)
        }
    }

    Column(Modifier.fillMaxSize(), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        // 固定状态区：不随滚动，永远在按钮上方，不被底部导航/键盘遮挡
        StatusBanner(busy != 0, kind, title, detail, sIdle, sFailHint)

        Column(
            Modifier.weight(1f).fillMaxWidth().verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Hint(tr("settings.x.endpoint_hint", "端点、Key、模型均由你自行填写；Key 经 Android Keystore 加密，不落明文。"))
            Field(base, { base = it }, AppI18n.t("settings.endpoint"))
            Field(key, { key = it }, AppI18n.t("settings.apikey"), secret = true)
            Field(model, { model = it }, sModelLabel)

            FlowRow(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalArrangement = Arrangement.spacedBy(4.dp),
            ) {
                ActionButton(
                    label = AppI18n.t("common.save"),
                    busyLabel = AppI18n.t("common.save"),
                    busy = false,
                    enabled = base.isNotBlank() && key.isNotBlank() && model.isNotBlank(),
                    primary = true,
                ) {
                    // 人设不属于端点页：system 原样透传，不在此编辑
                    onSave(ChatConfig(base.trim(), key.trim(), model.trim(), config.system))
                    toast(sSaved)
                }
                ActionButton(
                    label = sProbe,
                    busyLabel = sProbing,
                    busy = busy == 1,
                    enabled = busy == 0 && base.isNotBlank() && model.isNotBlank(),
                ) { probe() }
                ActionButton(
                    label = sList,
                    busyLabel = sListing,
                    busy = busy == 2,
                    enabled = busy == 0 && base.isNotBlank(),
                ) { listModels() }
            }

            if (models.isNotEmpty()) {
                SectionTitle(tr("settings.x.models_title", "可用模型（点选填入）") + " · " + models.size)
                if (models.size > 12) {
                    Field(filter, { filter = it }, tr("settings.x.models_filter", "筛选模型"))
                }
                val q = filter.trim()
                val shown = if (q.isEmpty()) models.toList() else models.filter { it.contains(q, ignoreCase = true) }
                FlowRow(
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    shown.forEach { id ->
                        ChoiceChip(id, id == model.trim()) { model = id }
                    }
                }
            }
            Spacer(Modifier.height(16.dp))
        }
    }
}

/* ───────────────────────── 进阶 ───────────────────────── */

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun AdvancedSettings(
    theme: ThemeConfig,
    reduceMotion: Boolean,
    onThemeChange: (ThemeConfig) -> Unit,
    onReduceMotionChange: (Boolean) -> Unit,
) {
    val ctx = LocalContext.current
    val cs = MaterialTheme.colorScheme
    var provider by remember { mutableStateOf(Prefs.searchProvider(ctx)) }
    var searxng by remember { mutableStateOf(Prefs.searxngBase(ctx)) }
    var braveKey by remember { mutableStateOf(SecretStore.get(ctx, "search.brave_key") ?: "") }
    var ttsVoice by remember { mutableStateOf(Prefs.ttsVoice(ctx)) }
    var ttsFormat by remember { mutableStateOf(Prefs.ttsFormat(ctx)) }
    var nativeSearch by remember { mutableStateOf(Prefs.nativeSearch(ctx)) }
    var communityBase by remember { mutableStateOf(CommunityPrefs.base(ctx)) }
    var personalTag by remember { mutableStateOf(SysPrefs.personalTag(ctx)) }
    var imgBase by remember { mutableStateOf(ImagePrefs.base(ctx)) }
    var imgModel by remember { mutableStateOf(ImagePrefs.model(ctx)) }
    var imgKey by remember { mutableStateOf(ImagePrefs.apiKey(ctx)) }
    var imgEditPath by remember { mutableStateOf(ImagePrefs.editPath(ctx)) }
    var embBase by remember { mutableStateOf(EmbedPrefs.base(ctx)) }
    var embModel by remember { mutableStateOf(EmbedPrefs.model(ctx)) }
    var embKey by remember { mutableStateOf(EmbedPrefs.key(ctx)) }
    var bkBase by remember { mutableStateOf(BackupPrefs.base(ctx)) }
    var bkModel by remember { mutableStateOf(BackupPrefs.model(ctx)) }
    var bkKey by remember { mutableStateOf(BackupPrefs.key(ctx)) }
    var bkProvider by remember { mutableStateOf(BackupPrefs.provider(ctx)) }
    var chatProto by remember { mutableStateOf(ProtoPrefs.provider(ctx)) }
    var proStatus by remember { mutableStateOf(ProactiveUi.status()) }
    var backupNote by remember { mutableStateOf("") }
    var backupFailed by remember { mutableStateOf(false) }
    var saved by remember { mutableStateOf("") }
    var savedFailed by remember { mutableStateOf(false) }

    val sExportEmpty = tr("settings.x.backup_export_empty", "导出为空")
    val sReadFail = tr("settings.x.backup_read_fail", "读取失败")
    val sExported = tr("settings.x.backup_exported", "已导出备份")
    val sImported = tr("settings.x.backup_imported", "已导入备份（重启 App 生效更稳妥）")
    val sTicked = tr("settings.x.proactive_ticked", "已触发一次主动评估")
    val sSavedImgMissing = tr("settings.x.adv_saved_img_missing", "已保存（画图模型未配全）")
    val sSavedImgFail = tr("settings.x.adv_saved_img_fail", "已保存，画图初始化失败：")
    val sSavedImgReady = tr("settings.x.adv_saved_img_ready", "已保存，画图模型就绪")
    val sProtoOpenAi = tr("settings.x.proto_openai", "OpenAI 兼容")

    fun toast(msg: String) {
        Toast.makeText(ctx, msg, Toast.LENGTH_SHORT).show()
    }

    val exportLauncher = rememberLauncherForActivityResult(ActivityResultContracts.CreateDocument("application/zip")) { uri ->
        if (uri != null) Bg.run({
            val data = Mobilebind.backupExport()
            if (data == null || data.isEmpty()) return@run "ERR:$sExportEmpty"
            try {
                ctx.contentResolver.openOutputStream(uri)?.use { it.write(data) }; "OK"
            } catch (e: Exception) {
                "ERR:" + (e.message ?: "?")
            }
        }) { r ->
            backupFailed = r != "OK"
            backupNote = if (r == "OK") sExported else r.removePrefix("ERR:")
            toast(backupNote)
        }
    }
    val importLauncher = rememberLauncherForActivityResult(ActivityResultContracts.GetContent()) { uri ->
        if (uri != null) Bg.run({
            val data = try {
                ctx.contentResolver.openInputStream(uri)?.use { it.readBytes() }
            } catch (e: Exception) {
                null
            }
            if (data == null) return@run "ERR:$sReadFail"
            Mobilebind.backupImport(data)
        }) { r ->
            backupFailed = !r.startsWith("OK:")
            backupNote = if (r.startsWith("OK:")) sImported else r.removePrefix("ERR:")
            toast(backupNote)
        }
    }

    Column(
        Modifier.fillMaxSize().verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        // 聊天协议
        SectionTitle(tr("settings.x.proto_title", "聊天协议（模型厂商）"))
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            ChoiceChip(sProtoOpenAi, chatProto == "openai") { chatProto = "openai" }
            ChoiceChip("Claude", chatProto == "anthropic") { chatProto = "anthropic" }
            ChoiceChip("Gemini", chatProto == "gemini") { chatProto = "gemini" }
        }
        Hint(tr("settings.x.proto_hint", "选 Claude/Gemini 时，端点/模型按对应官方或中转地址填。"))

        // 主动消息
        SectionTitle(AppI18n.t("proactive.title"))
        SwitchRow(
            tr("settings.x.proactive_enable", "启用主动消息"),
            tr("settings.x.proactive_enable_hint", "可随时撤回的权限，默认关闭"),
            proStatus.enabled, strong = false,
        ) { v -> Bg.run({ Mobilebind.proactiveSetEnabled(v); "OK" }) { proStatus = ProactiveUi.status() } }
        if (proStatus.enabled) {
            SwitchRow(tr("settings.x.proactive_mem", "记忆复习到期"), null, proStatus.memReview, strong = false) { v ->
                Bg.run({ Mobilebind.proactiveSetSource("memory_review", v); "OK" }) { proStatus = ProactiveUi.status() }
            }
            SwitchRow(tr("settings.x.proactive_schedule", "人设日程"), null, proStatus.schedule, strong = false) { v ->
                Bg.run({ Mobilebind.proactiveSetSource("schedule", v); "OK" }) { proStatus = ProactiveUi.status() }
            }
            SwitchRow(tr("settings.x.proactive_emotion", "情绪驱动"), null, proStatus.emotion, strong = false) { v ->
                Bg.run({ Mobilebind.proactiveSetSource("emotion", v); "OK" }) { proStatus = ProactiveUi.status() }
            }
            OutlinedButton(
                onClick = { Bg.run({ ProactiveUi.tickAndNotify(ctx).toString() }) { toast(sTicked) } },
                shape = RoundedCornerShape(LocalCornerRadius.current),
            ) { Text(tr("settings.x.proactive_tick", "立即评估一次")) }
        }

        // 主题
        SectionTitle(AppI18n.t("settings.theme"))
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            ChoiceChip(tr("settings.x.theme_day", "卡其日间"), theme.mode == ThemeMode.DAY) {
                onThemeChange(theme.copy(mode = ThemeMode.DAY))
            }
            ChoiceChip(tr("settings.x.theme_night", "夜间流星"), theme.mode == ThemeMode.NIGHT) {
                onThemeChange(theme.copy(mode = ThemeMode.NIGHT))
            }
        }
        Hint(tr("settings.x.accent", "自定义强调色"))
        FlowRow(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            AccentPresets.forEach { (argb, _) ->
                val c = if (argb == null) cs.primary else Color(argb)
                val selected = theme.accent == argb
                Box(
                    Modifier
                        .size(36.dp)
                        .clip(CircleShape)
                        .background(c, CircleShape)
                        .border(if (selected) 3.dp else 1.dp, if (selected) cs.onSurface else cs.outline, CircleShape)
                        .clickable { onThemeChange(theme.copy(accent = argb)) },
                    contentAlignment = Alignment.Center,
                ) {
                    if (selected) StIcon(StGlyph.CHECK, Modifier.size(16.dp), cs.onPrimary)
                }
            }
        }
        Hint(tr("settings.x.radius", "圆角档"))
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            RadiusPresets.forEach { (dpv, name) ->
                ChoiceChip(name, theme.radiusDp == dpv) { onThemeChange(theme.copy(radiusDp = dpv)) }
            }
        }

        SwitchRow(
            tr("settings.x.reduce_motion", "减少动态 / 关闭氛围动效"),
            tr("settings.x.reduce_motion_hint", "夜间流星背景静止为静态星空（省电、无障碍）"),
            reduceMotion, strong = true, onChange = onReduceMotionChange,
        )

        // 搜索
        SectionTitle(tr("settings.x.search_title", "网页搜索源"))
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            ChoiceChip(tr("settings.x.search_searxng", "SearXNG 自建"), provider == "searxng") { provider = "searxng" }
            ChoiceChip("Brave", provider == "brave") { provider = "brave" }
        }
        if (provider == "searxng") {
            Field(searxng, { searxng = it }, tr("settings.x.search_searxng_base", "SearXNG base（自建实例）"))
        } else {
            Field(braveKey, { braveKey = it }, tr("settings.x.search_brave_key", "Brave API Key（加密存储）"), secret = true)
        }

        // 语音
        SectionTitle(tr("settings.x.tts_title", "语音（TTS 朗读）"))
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Field(ttsVoice, { ttsVoice = it }, tr("settings.x.tts_voice", "音色 voice"), Modifier.weight(1f))
            Field(ttsFormat, { ttsFormat = it }, tr("settings.x.tts_format", "格式（如 mp3）"), Modifier.weight(1f))
        }

        SwitchRow(
            tr("settings.x.native_search", "原生搜索（按模型）"),
            tr("settings.x.native_search_hint", "开：模型自带 web_search；关：走外部搜索源"),
            nativeSearch, strong = true,
        ) { nativeSearch = it }

        // 社区源
        SectionTitle(tr("settings.x.community_title", "社区源（Git 仓库 / 自建 API，同一契约）"))
        Field(communityBase, { communityBase = it }, tr("settings.x.community_base", "社区源 base（raw 地址或 API 根）"))
        Hint(tr("settings.x.community_hint", "留空=用内置种子。见 docs/community-ecosystem-contract.md"))

        // 请求头
        SectionTitle(tr("settings.x.tag_title", "应用标识请求头（随请求发给你自配端点）"))
        Field(personalTag, { personalTag = it }, tr("settings.x.tag_label", "X-Star-River-Tag（可选）"))
        Hint(tr("settings.x.tag_hint", "默认头 X-Star-River-Client 恒带。"))

        // 画图
        SectionTitle(tr("settings.x.img_title", "画图模型（相册/自拍：人设图垫图）"))
        Field(imgBase, { imgBase = it }, tr("settings.x.img_base", "画图端点 base"))
        Field(imgModel, { imgModel = it }, tr("settings.x.img_model", "画图模型 id（需支持图生图）"))
        Field(imgKey, { imgKey = it }, tr("settings.x.img_key", "画图 API Key（加密存储）"), secret = true)
        Field(imgEditPath, { imgEditPath = it }, tr("settings.x.img_edit_path", "图生图路径（默认 /images/edits，可留空）"))
        Hint(tr("settings.x.img_hint", "垫图=人设图；有垫图走图生图端点保角色一致性。生成图片强制带 AI 水印。"))

        // embedding
        SectionTitle(tr("settings.x.emb_title", "语义记忆（embedding，可选）"))
        Hint(tr("settings.x.emb_hint", "配了才有向量召回，否则走关键词。"))
        Field(embBase, { embBase = it }, tr("settings.x.emb_base", "embedding 端点 base（可同聊天端点）"))
        Field(embModel, { embModel = it }, tr("settings.x.emb_model", "embedding 模型 id"))
        Field(embKey, { embKey = it }, tr("settings.x.emb_key", "embedding API Key（加密存储）"), secret = true)

        // 灾备
        SectionTitle(tr("settings.x.bk_title", "灾备备用端点（可选：主端点失败自动切）"))
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            listOf("openai" to sProtoOpenAi, "anthropic" to "Claude", "gemini" to "Gemini").forEach { (k, label) ->
                ChoiceChip(label, bkProvider == k) { bkProvider = k }
            }
        }
        Field(bkBase, { bkBase = it }, tr("settings.x.bk_base", "备用端点 base（留空=不使用灾备）"))
        Field(bkModel, { bkModel = it }, tr("settings.x.bk_model", "备用模型 id"))
        Field(bkKey, { bkKey = it }, tr("settings.x.bk_key", "备用 API Key（加密存储）"), secret = true)

        // 数据备份
        SectionTitle(tr("settings.x.backup_title", "数据备份（换机/防丢）"))
        if (backupNote.isNotEmpty()) {
            Text(
                backupNote,
                fontSize = 12.sp,
                fontWeight = if (backupFailed) FontWeight.Bold else FontWeight.Normal,
                color = if (backupFailed) cs.error else cs.primary,
            )
        }
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            OutlinedButton(
                onClick = { exportLauncher.launch("star-river-backup.zip") },
                shape = RoundedCornerShape(LocalCornerRadius.current),
            ) { Text(tr("settings.x.backup_export", "导出备份")) }
            OutlinedButton(
                onClick = { importLauncher.launch("application/zip") },
                shape = RoundedCornerShape(LocalCornerRadius.current),
            ) { Text(tr("settings.x.backup_import", "导入备份")) }
        }

        // 保存：结果在按钮上方 + Toast
        if (saved.isNotEmpty()) {
            Text(
                saved,
                fontSize = 12.sp,
                fontWeight = if (savedFailed) FontWeight.Bold else FontWeight.Normal,
                color = if (savedFailed) cs.error else cs.primary,
                modifier = Modifier.padding(top = 6.dp),
            )
        }
        Button(
            onClick = {
                Prefs.setSearchProvider(ctx, provider)
                Prefs.setSearxngBase(ctx, searxng.trim())
                if (braveKey.isNotBlank()) SecretStore.put(ctx, "search.brave_key", braveKey)
                Prefs.setTtsVoice(ctx, ttsVoice.trim().ifBlank { "alloy" })
                Prefs.setTtsFormat(ctx, ttsFormat.trim().ifBlank { "mp3" })
                Prefs.setNativeSearch(ctx, nativeSearch)
                CommunityPrefs.setBase(ctx, communityBase.trim())
                SysPrefs.setPersonalTag(ctx, personalTag.trim())
                HeaderHelp.apply(ctx)
                ProtoPrefs.setProvider(ctx, chatProto); ProtoPrefs.apply(ctx)
                ImagePrefs.setBase(ctx, imgBase.trim()); ImagePrefs.setModel(ctx, imgModel.trim())
                ImagePrefs.setEditPath(ctx, imgEditPath.trim()); ImagePrefs.setApiKey(ctx, imgKey.trim())
                EmbedPrefs.setBase(ctx, embBase.trim()); EmbedPrefs.setModel(ctx, embModel.trim()); EmbedPrefs.setKey(ctx, embKey.trim())
                EmbedPrefs.apply(ctx)
                BackupPrefs.setBase(ctx, bkBase.trim()); BackupPrefs.setModel(ctx, bkModel.trim())
                BackupPrefs.setKey(ctx, bkKey.trim()); BackupPrefs.setProvider(ctx, bkProvider)
                BackupPrefs.apply(ctx)
                val vr = VisualCfg.initIfConfigured(ctx)
                savedFailed = vr.startsWith("ERR")
                saved = if (vr == "not-configured") sSavedImgMissing
                else if (vr.startsWith("ERR")) sSavedImgFail + vr.removePrefix("ERR:")
                else sSavedImgReady
                toast(saved)
            },
            shape = RoundedCornerShape(LocalCornerRadius.current),
            modifier = Modifier.fillMaxWidth(),
        ) { Text(tr("settings.x.adv_save", "保存进阶设置")) }
        Spacer(Modifier.height(16.dp))
    }
}

/* ───────────────────────── 开发者 ───────────────────────── */

@Composable
private fun DeveloperSettings(
    config: ChatConfig,
    onOpenPermission: () -> Unit,
    onOpenReport: () -> Unit,
    onOpenSafety: () -> Unit,
    onOpenPanels: () -> Unit,
    onOpenDiag: () -> Unit,
) {
    val cs = MaterialTheme.colorScheme
    var info by remember { mutableStateOf("") }
    var infoBusy by remember { mutableStateOf(false) }
    var probe by remember { mutableStateOf("") }
    var probeFailed by remember { mutableStateOf(false) }
    var probeBusy by remember { mutableStateOf(false) }
    var compliance by remember { mutableStateOf("") }
    LaunchedEffect(Unit) {
        Bg.run({ Mobilebind.compliancePolicy() }) { compliance = it }
    }

    val sNotConfigured = tr("settings.x.dev_not_configured", "端点/模型未配置")
    val sLoading = tr("settings.x.loading", "读取中…")
    val sProbing = tr("settings.x.dev_probing", "探测中…")
    val shape = RoundedCornerShape(LocalCornerRadius.current)

    Column(
        Modifier.fillMaxSize().verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        SectionTitle(tr("settings.x.dev_compliance", "合规 / 系统"))
        if (compliance.isNotEmpty()) {
            Hint(tr("settings.x.dev_policy", "内容策略：") + compliance)
        }
        NavRow(StGlyph.LOCK, tr("settings.x.dev_permission", "权限管理"), onOpenPermission)
        NavRow(StGlyph.FLAG, tr("settings.x.dev_report", "举报与反馈"), onOpenReport)
        NavRow(StGlyph.SHIELD, tr("settings.x.dev_safety", "系统 · 安全 · 自愈板块"), onOpenSafety)
        NavRow(StGlyph.CHART, tr("settings.x.dev_panels", "开发者三面板（情绪/记忆/上下文）"), onOpenPanels)
        NavRow(StGlyph.PULSE, tr("settings.x.dev_diag", "崩溃诊断 · 隐私边界"), onOpenDiag)

        SectionTitle(tr("settings.x.dev_kernel", "内核信息"))
        ActionButton(
            label = tr("settings.x.dev_kernel_read", "读取内核版本 / 设备档"),
            busyLabel = sLoading,
            busy = infoBusy,
            enabled = true,
            primary = true,
        ) {
            infoBusy = true
            Bg.run({ "version=" + Mobilebind.version() + "\n\nDeviceProfile=\n" + Mobilebind.deviceProfile() }) {
                info = it; infoBusy = false
            }
        }
        if (info.isNotEmpty()) {
            Text(
                info,
                fontSize = 11.sp,
                color = cs.onSurfaceVariant,
                modifier = Modifier.fillMaxWidth().clip(shape).background(cs.surfaceVariant).padding(10.dp),
            )
        }

        SectionTitle(tr("settings.x.dev_probe_title", "模型实测探测"))
        Hint(tr("settings.x.dev_probe_hint", "有真实调用成本，仅在选中模型时点。"))
        ActionButton(
            label = tr("settings.x.dev_probe", "探测当前模型能力"),
            busyLabel = sProbing,
            busy = probeBusy,
            enabled = true,
            primary = true,
        ) {
            if (config.baseUrl.isBlank() || config.model.isBlank()) {
                probe = sNotConfigured; probeFailed = true
            } else {
                probeBusy = true; probeFailed = false
                Bg.run({ Mobilebind.probeModel(config.baseUrl, config.apiKey, config.model) }) { r ->
                    probeFailed = r.startsWith("ERR:")
                    probe = r.removePrefix("ERR:")
                    probeBusy = false
                }
            }
        }
        if (probe.isNotEmpty()) {
            Text(
                probe,
                fontSize = 11.sp,
                fontWeight = if (probeFailed) FontWeight.Bold else FontWeight.Normal,
                color = if (probeFailed) cs.error else cs.onSurfaceVariant,
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(shape)
                    .background(cs.surfaceVariant)
                    .then(if (probeFailed) Modifier.border(1.dp, cs.error, shape) else Modifier)
                    .padding(10.dp),
            )
        }
        Spacer(Modifier.height(16.dp))
    }
}
