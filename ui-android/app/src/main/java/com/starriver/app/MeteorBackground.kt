package com.starriver.app

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import kotlinx.coroutines.delay
import kotlin.math.floor
import kotlin.math.sin
import kotlin.random.Random

// 夜间流星色板（docs/ui-spec.md §8.2）
private val SkyTop = Color(0xFF0B1026)
private val SkyBottom = Color(0xFF131A33)
private val Skyline = Color(0xFF060A18)
private val MeteorBlue = Color(0xFF38BDF8)
private val GlowBlue = Color(0xFFBFE6FF)

private data class Star(val x: Float, val y: Float, val r: Float, val ph: Float)
private data class Meteor(
    val x: Float, val y: Float, val vx: Float, val vy: Float,
    val cycle: Float, val phase: Float, val len: Float,
)

/**
 * 夜间流星氛围背景（§8.6 性能约束）：
 * - Compose Canvas（Skia/GPU）绘制，~25fps（delay 40ms），不做重模糊。
 * - 3~8 条流星（此处 6 条）简单光带；64 颗静态星点轻微闪烁。
 * - 切后台/锁屏（ON_STOP/ON_PAUSE）立即暂停；回前台恢复。
 * - 低端机（可用内存 < ~1.5GB）自动降级为静态星空。
 * - reduceMotion（用户「减少动态/关闭氛围动效」开关）→ 不跑动画，仅静态。
 */
@Composable
fun StarryBackdrop(reduceMotion: Boolean, content: @Composable BoxScope.() -> Unit) {
    val lowEnd = remember { isLowEndDevice() }
    val animate = !reduceMotion && !lowEnd
    var running by remember { mutableStateOf(true) }
    val lifecycleOwner = LocalLifecycleOwner.current
    DisposableEffect(lifecycleOwner) {
        val obs = LifecycleEventObserver { _, e ->
            when (e) {
                Lifecycle.Event.ON_START, Lifecycle.Event.ON_RESUME -> running = true
                Lifecycle.Event.ON_STOP, Lifecycle.Event.ON_PAUSE -> running = false
                else -> {}
            }
        }
        lifecycleOwner.lifecycle.addObserver(obs)
        onDispose { lifecycleOwner.lifecycle.removeObserver(obs) }
    }
    var time by remember { mutableStateOf(0f) }
    LaunchedEffect(animate, running) {
        if (!animate) return@LaunchedEffect
        while (true) {
            if (running) time += 40f
            delay(40) // ~25fps
        }
    }
    val stars = remember {
        val r = Random(42)
        List(64) { Star(r.nextFloat(), r.nextFloat() * 0.88f, 0.6f + r.nextFloat() * 1.6f, r.nextFloat() * 6.28f) }
    }
    val meteors = remember {
        val r = Random(7)
        List(6) {
            val ang = -40f + r.nextFloat() * 28f
            val rad = Math.toRadians(ang.toDouble())
            Meteor(
                x = r.nextFloat(),
                y = -0.2f - r.nextFloat() * 0.35f,
                vx = (Math.cos(rad)).toFloat() * 1.6f,
                vy = (Math.sin(rad)).toFloat() * 1.4f + 1.3f,
                cycle = 3.5f + r.nextFloat() * 3f,
                phase = r.nextFloat(),
                len = 0.10f + r.nextFloat() * 0.08f,
            )
        }
    }
    Box(Modifier.fillMaxSize()) {
        Canvas(Modifier.fillMaxSize()) {
            val w = size.width
            val h = size.height
            drawRect(Brush.verticalGradient(listOf(SkyTop, SkyBottom)))
            // 星点
            stars.forEach { s ->
                val a = if (animate) 0.30f + 0.45f * (0.5f + 0.5f * sin(time / 700f + s.ph)) else 0.55f
                drawCircle(GlowBlue.copy(alpha = a * 0.7f), radius = s.r, center = Offset(s.x * w, s.y * h))
            }
            // 流星（仅动效开启时）
            if (animate) {
                meteors.forEach { m ->
                    val tt = (time / 1000f) / m.cycle + m.phase
                    val prog = tt - floor(tt)
                    if (prog < 0.45f) {
                        val p = prog / 0.45f
                        val headX = (m.x + m.vx * p * 0.6f) * w
                        val headY = (m.y + m.vy * p * 0.6f) * h
                        val tailX = headX - m.vx * w * m.len
                        val tailY = headY - m.vy * h * m.len
                        val taper = sin(p * Math.PI.toFloat())
                        drawLine(
                            Brush.linearGradient(
                                listOf(MeteorBlue.copy(alpha = 0f), MeteorBlue.copy(alpha = 0.35f * taper)),
                                start = Offset(tailX, tailY), end = Offset(headX, headY),
                            ),
                            Offset(tailX, tailY), Offset(headX, headY), strokeWidth = 5f, cap = StrokeCap.Round,
                        )
                        drawLine(
                            Brush.linearGradient(
                                listOf(GlowBlue.copy(alpha = 0f), GlowBlue.copy(alpha = 0.95f * taper)),
                                start = Offset(tailX, tailY), end = Offset(headX, headY),
                            ),
                            Offset(tailX, tailY), Offset(headX, headY), strokeWidth = 2f, cap = StrokeCap.Round,
                        )
                    }
                }
            }
            // 城市天际线剪影 + 零星窗光
            val groundY = h * 0.86f
            val rnd = Random(99)
            var x = 0f
            while (x < w) {
                val bw = w * (0.06f + rnd.nextFloat() * 0.09f)
                val bh = h * (0.05f + rnd.nextFloat() * 0.12f)
                drawRect(Skyline, topLeft = Offset(x, groundY - bh), size = Size(bw, bh + h))
                var wx = x + 5f
                while (wx < x + bw - 5f) {
                    var yy = groundY - bh + 6f
                    while (yy < groundY - 6f) {
                        if (rnd.nextFloat() > 0.62f) {
                            drawRect(GlowBlue.copy(alpha = 0.16f), topLeft = Offset(wx, yy), size = Size(2f, 3f))
                        }
                        yy += 9f
                    }
                    wx += 9f
                }
                x += bw + 2f
            }
            drawRect(Skyline, topLeft = Offset(0f, groundY), size = Size(w, h - groundY))
        }
        content()
    }
}

/** 低端机探测：可用内存 < ~1.5GB 视为低端，动效降级静态。失败时按非低端处理。 */
private fun isLowEndDevice(): Boolean = try {
    val o = org.json.JSONObject(mobilebind.Mobilebind.deviceProfile())
    val mem = o.optLong("avail_mem_bytes", Long.MAX_VALUE)
    mem in 1..1_500_000_000L
} catch (t: Throwable) {
    false
}
