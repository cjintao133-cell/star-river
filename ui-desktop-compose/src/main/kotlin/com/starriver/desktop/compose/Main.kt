package com.starriver.desktop.compose

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.padding
import androidx.compose.material.MaterialTheme
import androidx.compose.material.Text
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Window
import androidx.compose.ui.window.application
import androidx.compose.ui.window.rememberWindowState
import com.starriver.desktop.KernelBridge
import java.awt.image.BufferedImage
import java.io.File
import javax.imageio.ImageIO

/**
 * G0-PREP：最小 Compose 桌面 + JNI 补验（Xvfb 无头）。
 * 用法：xvfb-run java ... com.starriver.desktop.compose.MainKt <libstarriver.so 绝对路径> <截图输出.png>
 * 断言失败即抛异常（exit!=0）；成功截图后 exitProcess(0)。
 * 截图用 java.awt.Robot 抓全屏（Xvfb 有真实 X screen；无 WM 时窗口位置不可控，不抓局部）。
 */
fun main(args: Array<String>) {
    System.load(args[0])
    val outPng = args[1]

    val version = KernelBridge.version()
    check(version == "star-river-kernel/0.0.1-m0") { "version: [$version]" }
    val echo = KernelBridge.echo("中文与emoji🌌混排")
    check(echo == "中文与emoji🌌混排") { "echo-utf8: [$echo]" }
    val profile = KernelBridge.deviceProfile()
    check(profile.startsWith("{") && profile.endsWith("}") &&
        profile.contains("\"neon\"") && profile.contains("\"avx2\"") &&
        profile.contains("\"avail_mem_bytes\"")) { "device-profile: [$profile]" }
    println("[OK] jni version/echo-utf8/device-profile")
    println("DEVICE-PROFILE: $profile")

    application {
        Window(
            onCloseRequest = ::exitApplication,
            state = rememberWindowState(width = 640.dp, height = 360.dp),
            title = "Star River G0-PREP"
        ) {
            MaterialTheme {
                Column(Modifier.padding(16.dp)) {
                    Text("VERSION: $version")
                    Text("ECHO: $echo")
                    Text("PROFILE: $profile")
                }
            }
        }
        Thread {
            Thread.sleep(4000) // 等 skiko 首帧（Xvfb 软渲染，放宽）
            try {
                val robot = java.awt.Robot()
                val screen = java.awt.Toolkit.getDefaultToolkit().screenSize
                val img: BufferedImage = robot.createScreenCapture(java.awt.Rectangle(screen))
                ImageIO.write(img, "png", File(outPng))
                println("[OK] screenshot -> $outPng")
                println("COMPOSE-JNI-GREEN")
                kotlin.system.exitProcess(0)
            } catch (t: Throwable) {
                System.err.println("[FAIL] screenshot: $t")
                kotlin.system.exitProcess(1)
            }
        }.start()
    }
}
