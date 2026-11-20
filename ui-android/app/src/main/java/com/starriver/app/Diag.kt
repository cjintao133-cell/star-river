package com.starriver.app

import android.content.Context
import mobilebind.Mobilebind
import java.io.PrintWriter
import java.io.StringWriter

/** 崩溃诊断：全局未捕获异常 → 本地 dump（绝不上传）；隐私边界文本。 */
object Diag {
    /** 安装全局未捕获异常处理器：记录本地 dump 后交回系统默认处理（仍会崩，但留下记录）。 */
    fun install(ctx: Context) {
        val prev = Thread.getDefaultUncaughtExceptionHandler()
        Thread.setDefaultUncaughtExceptionHandler { thread, ex ->
            try {
                val sw = StringWriter()
                ex.printStackTrace(PrintWriter(sw))
                Mobilebind.diagCrashDump(sw.toString())
            } catch (t: Throwable) { }
            prev?.uncaughtException(thread, ex)
        }
    }

    fun privacyBoundary(): String = try { Mobilebind.diagPrivacyBoundary() } catch (t: Throwable) { "" }
    fun listDumps(): String = try { Mobilebind.diagListDumps() } catch (t: Throwable) { "[]" }
    fun readDump(name: String): String = try { Mobilebind.diagReadDump(name) } catch (t: Throwable) { "" }
}
