package com.starriver.app

import android.content.Context
import android.media.MediaRecorder
import java.io.File

/** 极简录音器：MediaRecorder 录 AAC(m4a) 到缓存文件，停止后返回字节。供 STT 转写。 */
object Stt {
    private var rec: MediaRecorder? = null
    private var file: File? = null

    /** 开始录音，返回 "OK"/"ERR:..."。 */
    @Suppress("DEPRECATION")
    fun start(ctx: Context): String = try {
        val f = File(ctx.cacheDir, "stt_${System.currentTimeMillis()}.m4a")
        val r = if (android.os.Build.VERSION.SDK_INT >= 31) MediaRecorder(ctx) else MediaRecorder()
        r.setAudioSource(MediaRecorder.AudioSource.MIC)
        r.setOutputFormat(MediaRecorder.OutputFormat.MPEG_4)
        r.setAudioEncoder(MediaRecorder.AudioEncoder.AAC)
        r.setAudioSamplingRate(16000)
        r.setOutputFile(f.absolutePath)
        r.prepare()
        r.start()
        rec = r; file = f
        "OK"
    } catch (t: Throwable) {
        try { rec?.release() } catch (e: Throwable) { }
        rec = null; file = null
        "ERR:" + (t.message ?: "record start failed")
    }

    /** 停止录音并返回音频字节（无则 null）。 */
    fun stop(): ByteArray? {
        val r = rec; val f = file
        rec = null; file = null
        if (r == null || f == null) return null
        return try {
            r.stop(); r.release()
            val bytes = if (f.exists()) f.readBytes() else null
            f.delete()
            bytes
        } catch (t: Throwable) {
            try { r.release() } catch (e: Throwable) { }
            try { f.delete() } catch (e: Throwable) { }
            null
        }
    }
}
