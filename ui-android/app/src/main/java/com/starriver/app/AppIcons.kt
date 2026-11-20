package com.starriver.app

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.size
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp

/**
 * 干净矢量图标（Canvas 手绘线性，无文字/无 emoji/无第三方图标库）。
 * 24dp 网格、圆头端点、统一线宽；描边取 tint 色。风格素净。
 */
private fun stroke(color: Color, w: Float) = Stroke(width = w, cap = StrokeCap.Round)

@Composable
fun IconMic(tint: Color, sizeDp: Dp = 24.dp) {
    Canvas(Modifier.size(sizeDp)) {
        val w = size.width; val h = size.height; val u = w / 24f
        val sw = 1.9f * u
        // 麦克风胶囊
        drawRoundRect(color = tint, topLeft = Offset(9 * u, 3 * u), size = Size(6 * u, 11 * u),
            cornerRadius = androidx.compose.ui.geometry.CornerRadius(3 * u, 3 * u), style = stroke(tint, sw))
        // 下方弧形托
        drawArc(color = tint, startAngle = 20f, sweepAngle = 140f, useCenter = false,
            topLeft = Offset(6 * u, 6 * u), size = Size(12 * u, 12 * u), style = stroke(tint, sw))
        // 支杆
        drawLine(tint, Offset(12 * u, 18 * u), Offset(12 * u, 21 * u), strokeWidth = sw, cap = StrokeCap.Round)
    }
}

@Composable
fun IconCamera(tint: Color, sizeDp: Dp = 24.dp) {
    Canvas(Modifier.size(sizeDp)) {
        val u = size.width / 24f; val sw = 1.9f * u
        // 机身
        drawRoundRect(color = tint, topLeft = Offset(3 * u, 7 * u), size = Size(18 * u, 12 * u),
            cornerRadius = androidx.compose.ui.geometry.CornerRadius(2.5f * u, 2.5f * u), style = stroke(tint, sw))
        // 顶部取景凸起
        drawLine(tint, Offset(9 * u, 7 * u), Offset(10 * u, 5 * u), strokeWidth = sw, cap = StrokeCap.Round)
        drawLine(tint, Offset(10 * u, 5 * u), Offset(14 * u, 5 * u), strokeWidth = sw, cap = StrokeCap.Round)
        drawLine(tint, Offset(14 * u, 5 * u), Offset(15 * u, 7 * u), strokeWidth = sw, cap = StrokeCap.Round)
        // 镜头
        drawCircle(tint, radius = 3.4f * u, center = Offset(12 * u, 13 * u), style = stroke(tint, sw))
    }
}

@Composable
fun IconPhoto(tint: Color, sizeDp: Dp = 24.dp) {
    Canvas(Modifier.size(sizeDp)) {
        val u = size.width / 24f; val sw = 1.9f * u
        drawRoundRect(color = tint, topLeft = Offset(3 * u, 4 * u), size = Size(18 * u, 16 * u),
            cornerRadius = androidx.compose.ui.geometry.CornerRadius(2.5f * u, 2.5f * u), style = stroke(tint, sw))
        // 太阳
        drawCircle(tint, radius = 1.8f * u, center = Offset(8.5f * u, 9 * u), style = stroke(tint, sw))
        // 山线
        drawLine(tint, Offset(4.5f * u, 17 * u), Offset(10 * u, 11 * u), strokeWidth = sw, cap = StrokeCap.Round)
        drawLine(tint, Offset(10 * u, 11 * u), Offset(15 * u, 17 * u), strokeWidth = sw, cap = StrokeCap.Round)
    }
}

@Composable
fun IconSend(tint: Color, sizeDp: Dp = 24.dp) {
    Canvas(Modifier.size(sizeDp)) {
        val u = size.width / 24f; val sw = 1.9f * u
        // 纸飞机
        drawLine(tint, Offset(4 * u, 12 * u), Offset(20 * u, 5 * u), strokeWidth = sw, cap = StrokeCap.Round)
        drawLine(tint, Offset(20 * u, 5 * u), Offset(13 * u, 20 * u), strokeWidth = sw, cap = StrokeCap.Round)
        drawLine(tint, Offset(13 * u, 20 * u), Offset(11 * u, 13 * u), strokeWidth = sw, cap = StrokeCap.Round)
        drawLine(tint, Offset(11 * u, 13 * u), Offset(4 * u, 12 * u), strokeWidth = sw, cap = StrokeCap.Round)
    }
}

@Composable
fun IconStop(tint: Color, sizeDp: Dp = 24.dp) {
    Canvas(Modifier.size(sizeDp)) {
        val u = size.width / 24f
        drawRoundRect(color = tint, topLeft = Offset(7 * u, 7 * u), size = Size(10 * u, 10 * u),
            cornerRadius = androidx.compose.ui.geometry.CornerRadius(2 * u, 2 * u))
    }
}

@Composable
fun IconBack(tint: Color, sizeDp: Dp = 24.dp) {
    Canvas(Modifier.size(sizeDp)) {
        val u = size.width / 24f; val sw = 1.9f * u
        drawLine(tint, Offset(14 * u, 6 * u), Offset(8 * u, 12 * u), strokeWidth = sw, cap = StrokeCap.Round)
        drawLine(tint, Offset(8 * u, 12 * u), Offset(14 * u, 18 * u), strokeWidth = sw, cap = StrokeCap.Round)
    }
}

@Composable
fun IconTrash(tint: Color, sizeDp: Dp = 20.dp) {
    Canvas(Modifier.size(sizeDp)) {
        val u = size.width / 24f; val sw = 1.8f * u
        drawLine(tint, Offset(5 * u, 7 * u), Offset(19 * u, 7 * u), strokeWidth = sw, cap = StrokeCap.Round)
        drawRoundRect(color = tint, topLeft = Offset(7 * u, 7 * u), size = Size(10 * u, 12 * u),
            cornerRadius = androidx.compose.ui.geometry.CornerRadius(2 * u, 2 * u), style = stroke(tint, sw))
        drawLine(tint, Offset(10 * u, 4 * u), Offset(14 * u, 4 * u), strokeWidth = sw, cap = StrokeCap.Round)
    }
}


@Composable
fun IconSearch(tint: Color, sizeDp: Dp = 20.dp) {
    Canvas(Modifier.size(sizeDp)) {
        val u = size.width / 24f; val sw = 1.9f * u
        drawCircle(color = tint, radius = 6.5f * u, center = Offset(10.5f * u, 10.5f * u), style = stroke(tint, sw))
        drawLine(tint, Offset(15.5f * u, 15.5f * u), Offset(20f * u, 20f * u), strokeWidth = sw, cap = StrokeCap.Round)
    }
}

@Composable
fun IconChat(tint: Color, sizeDp: Dp = 22.dp) {
    Canvas(Modifier.size(sizeDp)) {
        val u = size.width / 24f; val sw = 1.9f * u
        // 圆角气泡
        drawRoundRect(color = tint, topLeft = Offset(4 * u, 5 * u), size = Size(16 * u, 12 * u),
            cornerRadius = androidx.compose.ui.geometry.CornerRadius(3 * u, 3 * u), style = stroke(tint, sw))
        // 底部小尾巴
        drawLine(tint, Offset(8 * u, 17 * u), Offset(7 * u, 20.5f * u), strokeWidth = sw, cap = StrokeCap.Round)
        drawLine(tint, Offset(7 * u, 20.5f * u), Offset(11 * u, 17 * u), strokeWidth = sw, cap = StrokeCap.Round)
        // 内部三点
        val r = 1.1f * u
        for (cx in listOf(9f, 12f, 15f)) drawCircle(tint, r, Offset(cx * u, 11 * u))
    }
}
