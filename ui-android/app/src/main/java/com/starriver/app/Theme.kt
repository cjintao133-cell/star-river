package com.starriver.app

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

/** 主题模式：卡其日间 / 夜间流星（见 docs/ui-spec.md §8）。 */
enum class ThemeMode { DAY, NIGHT }

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

/** 应用主题：按 mode 选色板，套 Material3。 */
@Composable
fun AppTheme(mode: ThemeMode, content: @Composable () -> Unit) {
    val colors = when (mode) {
        ThemeMode.DAY -> DayColors
        ThemeMode.NIGHT -> NightColors
    }
    MaterialTheme(colorScheme = colors, content = content)
}
