package com.starriver.app

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * Android Keystore 密钥持有（红线 4）。
 *
 * 主密钥由 AndroidKeyStore 生成并常驻安全硬件，不可导出；数据用 AES/GCM 加解密后
 * 以 Base64 存在本应用私有 SharedPreferences（密文可落盘，密钥不可导出）。
 *
 * 对应内核 secret.Vault 的 Android 后端：内核只依赖 secret.Delegate 接口，
 * 平台加解密留在此 Kotlin 层（见 kernel/secret/vault_delegate.go 的注释约定）。
 */
object SecretStore {
    private const val ANDROID_KEYSTORE = "AndroidKeyStore"
    private const val KEY_ALIAS = "star_river_master_key"
    private const val TRANSFORMATION = "AES/GCM/NoPadding"
    private const val GCM_TAG_BITS = 128
    private const val PREF = "star_river_secrets"

    private fun keyStore(): KeyStore =
        KeyStore.getInstance(ANDROID_KEYSTORE).apply { load(null) }

    /** 取或生成 Keystore 主密钥（不可导出）。 */
    private fun masterKey(): SecretKey {
        val ks = keyStore()
        (ks.getEntry(KEY_ALIAS, null) as? KeyStore.SecretKeyEntry)?.let { return it.secretKey }
        val kg = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, ANDROID_KEYSTORE)
        kg.init(
            KeyGenParameterSpec.Builder(
                KEY_ALIAS,
                KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
            )
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .build(),
        )
        return kg.generateKey()
    }

    /** 加密并持久化（存储格式：Base64(iv) + ":" + Base64(ciphertext)）。 */
    fun put(ctx: android.content.Context, id: String, plain: String) {
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.ENCRYPT_MODE, masterKey())
        val ct = cipher.doFinal(plain.toByteArray(Charsets.UTF_8))
        val iv = cipher.iv
        val stored = Base64.encodeToString(iv, Base64.NO_WRAP) + ":" +
            Base64.encodeToString(ct, Base64.NO_WRAP)
        ctx.getSharedPreferences(PREF, android.content.Context.MODE_PRIVATE)
            .edit().putString(id, stored).apply()
    }

    /** 取回明文；不存在返回 null。 */
    fun get(ctx: android.content.Context, id: String): String? {
        val stored = ctx.getSharedPreferences(PREF, android.content.Context.MODE_PRIVATE)
            .getString(id, null) ?: return null
        val parts = stored.split(":")
        if (parts.size != 2) return null
        return try {
            val iv = Base64.decode(parts[0], Base64.NO_WRAP)
            val ct = Base64.decode(parts[1], Base64.NO_WRAP)
            val cipher = Cipher.getInstance(TRANSFORMATION)
            cipher.init(Cipher.DECRYPT_MODE, masterKey(), GCMParameterSpec(GCM_TAG_BITS, iv))
            String(cipher.doFinal(ct), Charsets.UTF_8)
        } catch (t: Throwable) {
            null
        }
    }

    fun delete(ctx: android.content.Context, id: String) {
        ctx.getSharedPreferences(PREF, android.content.Context.MODE_PRIVATE)
            .edit().remove(id).apply()
    }
}
