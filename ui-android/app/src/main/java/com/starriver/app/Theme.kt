package com.starriver.app

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.luminance
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp

/** 主题模式：卡其日间 / 夜间流星（见 docs/ui-spec.md §8）。 */
enum class ThemeMode { DAY, NIGHT }

/** 主题配置（T-M4-04 主题 v2）：基础模式 + 自定义强调色(ARGB, null=默认) + 圆角档(dp)。 */
data class ThemeConfig(
    val mode: ThemeMode = ThemeMode.DAY,
    val accent: Int? = null,
    val radiusDp: Int = 16,
)

/** 全应用圆角令牌：组件读它而非硬编码，主题切换/自定义即时生效。 */
val LocalCornerRadius = staticCompositionLocalOf<Dp> { 16.dp }

// 卡其日间：暖橙(primary)+砖红(secondary)，卡其底。
private val DayColors = lightColorScheme(
    primary = Color(0xFFD9762F),
    onPrimary = Color.White,
    secondary = Color(0xFFB5432E),
    onSecondary = Color.White,
    background = Color(0xFFE8DCC8),
    onBackground = Color(0xFF3A2E24),
    surface = Color(0xFFF5EFE3),
    onSurface = Color(0xFF3A2E24),
    surfaceVariant = Color(0xFFEFE6D4),
    onSurfaceVariant = Color(0xFF8A7A67),
    outline = Color(0xFFD9CCB6),
)

// 夜间流星：流星青蓝(primary)+星辉浅蓝(secondary)，夜空深底。
private val NightColors = darkColorScheme(
    primary = Color(0xFF38BDF8),
    onPrimary = Color(0xFF081018),
    secondary = Color(0xFFBFE6FF),
    onSecondary = Color(0xFF081018),
    background = Color(0xFF0B1026),
    onBackground = Color(0xFFE6EEF8),
    surface = Color(0xFF131A33),
    onSurface = Color(0xFFE6EEF8),
    surfaceVariant = Color(0xFF1B2540),
    onSurfaceVariant = Color(0xFF93A4C0),
    outline = Color(0xFF2A3352),
)

/** 依据强调色算可读的前景色（深色底配浅字，浅色底配深字）。 */
private fun onColorFor(c: Color): Color = if (c.luminance() > 0.5f) Color(0xFF1A130C) else Color.White

/** 应用主题：按配置选基色板 + 覆盖强调色 + 提供圆角令牌。 */
@Composable
fun AppTheme(config: ThemeConfig, content: @Composable () -> Unit) {
    var colors = when (config.mode) {
        ThemeMode.DAY -> DayColors
        ThemeMode.NIGHT -> NightColors
    }
    config.accent?.let { argb ->
        val c = Color(argb)
        colors = colors.copy(primary = c, onPrimary = onColorFor(c))
    }
    CompositionLocalProvider(LocalCornerRadius provides config.radiusDp.dp) {
        MaterialTheme(colorScheme = colors, content = content)
    }
}

/** 圆角档位预设（紧凑/标准/宽松）。 */
val RadiusPresets = listOf(8 to "紧凑", 16 to "标准", 24 to "宽松")

/** 强调色预设（null = 跟随主题默认）。 */
val AccentPresets: List<Pair<Int?, String>> = listOf(
    null to "默认",
    0xFFD9762F.toInt() to "暖橙",
    0xFFB5432E.toInt() to "砖红",
    0xFF38BDF8.toInt() to "青蓝",
    0xFFA855F7.toInt() to "星紫",
    0xFF22C55E.toInt() to "青绿",
    0xFFEC4899.toInt() to "樱粉",
)
