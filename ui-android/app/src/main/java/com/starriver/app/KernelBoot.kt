package com.starriver.app

import android.content.Context
import android.util.Base64
import mobilebind.Mobilebind
import java.security.SecureRandom

/**
 * 内核自启：生成/复用一个 32 字节字段加密钥（经 SecretStore/Keystore 加密持有），
 * 打开本地 SQLite 内核（mobilebind.initKernel）。幂等。
 */
object KernelBoot {
    private const val KEY_ID = "kernel.field_key"

    @Volatile
    private var inited = false

    fun ensure(ctx: Context): String {
        if (inited) return "OK"
        val b64 = SecretStore.get(ctx, KEY_ID) ?: run {
            val key = ByteArray(32)
            SecureRandom().nextBytes(key)
            val s = Base64.encodeToString(key, Base64.NO_WRAP)
            SecretStore.put(ctx, KEY_ID, s)
            s
        }
        val r = try {
            Mobilebind.initKernel(ctx.filesDir.absolutePath + "/star-river.db", b64)
        } catch (t: Throwable) {
            "ERR:" + (t.message ?: "init failed")
        }
        if (r == "OK") inited = true
        return r
    }

    /** 自愈/看门狗用：重新打开内核（幂等）。不改密钥、不覆盖损坏数据。 */
    fun reinit(ctx: Context): String {
        inited = false
        return ensure(ctx)
    }

    fun isInited(): Boolean = inited
}
