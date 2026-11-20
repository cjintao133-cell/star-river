package com.starriver.app

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.os.Build
import mobilebind.Mobilebind
import org.json.JSONArray
import org.json.JSONObject

/** 主动能力（proactive）UI 辅助：状态读取、触发一次并本地通知。 */
object ProactiveUi {
    data class Status(val enabled: Boolean, val memReview: Boolean, val schedule: Boolean, val emotion: Boolean)

    fun status(): Status = try {
        val o = JSONObject(Mobilebind.proactiveStatus())
        val s = o.optJSONObject("sources")
        Status(
            enabled = o.optBoolean("enabled", false),
            memReview = s?.optBoolean("memory_review", true) ?: true,
            schedule = s?.optBoolean("schedule", true) ?: true,
            emotion = s?.optBoolean("emotion", true) ?: true,
        )
    } catch (t: Throwable) { Status(false, true, true, true) }

    private const val CHANNEL_ID = "star_river_proactive"

    private fun ensureChannel(ctx: Context) {
        if (Build.VERSION.SDK_INT >= 26) {
            val nm = ctx.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
            if (nm.getNotificationChannel(CHANNEL_ID) == null) {
                nm.createNotificationChannel(NotificationChannel(CHANNEL_ID, "主动消息", NotificationManager.IMPORTANCE_DEFAULT))
            }
        }
    }

    /** 评估一次；有 nudge 则本地通知（标题=TA 的心声，正文=消息）。返回 nudge 数。 */
    fun tickAndNotify(ctx: Context): Int = try {
        val arr = JSONArray(Mobilebind.proactiveTick())
        if (arr.length() > 0) {
            ensureChannel(ctx)
            val nm = ctx.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
            for (i in 0 until arr.length()) {
                val o = arr.optJSONObject(i) ?: continue
                val title = o.optString("title", "来自 TA 的消息")
                val body = o.optString("body", "")
                val b = if (Build.VERSION.SDK_INT >= 26) Notification.Builder(ctx, CHANNEL_ID) else @Suppress("DEPRECATION") Notification.Builder(ctx)
                b.setContentTitle(title).setContentText(body).setSmallIcon(android.R.drawable.ic_dialog_info).setAutoCancel(true)
                nm.notify(1000 + i, b.build())
            }
        }
        arr.length()
    } catch (t: Throwable) { 0 }
}
