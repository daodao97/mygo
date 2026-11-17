package dev.mygo

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * Secrets, encrypted with an AES key of Android's keystore, which never
 * leaves it, and kept in the app's shared preferences.
 */
object Secrets {
    private const val KEY = "mygo.secrets"

    private fun key(): SecretKey {
        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (ks.getEntry(KEY, null) as? KeyStore.SecretKeyEntry)?.let { return it.secretKey }
        val gen = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
        gen.init(
            KeyGenParameterSpec.Builder(KEY, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .build()
        )
        return gen.generateKey()
    }

    private fun prefs(c: Context) = c.getSharedPreferences(KEY, Context.MODE_PRIVATE)
    private fun name(service: String, account: String) = service + "\u0000" + account

    fun get(c: Context, call: SecretGetCall): SecretGetReply {
        val r = SecretGetReply()
        val stored = prefs(c).getString(name(call.service, call.account), null) ?: return r
        try {
            val data = Base64.decode(stored, Base64.NO_WRAP)
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, data, 0, 12))
            r.value = cipher.doFinal(data, 12, data.size - 12)
            r.found = true
        } catch (e: Exception) {
            r.error = "mygo: keystore: ${e.message}"
        }
        return r
    }

    fun set(c: Context, call: SecretSetCall): SecretSetReply {
        val r = SecretSetReply()
        try {
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.ENCRYPT_MODE, key())
            val data = cipher.iv + cipher.doFinal(call.value)
            prefs(c).edit().putString(name(call.service, call.account), Base64.encodeToString(data, Base64.NO_WRAP)).apply()
        } catch (e: Exception) {
            r.error = "mygo: keystore: ${e.message}"
        }
        return r
    }

    fun delete(c: Context, call: SecretDeleteCall): SecretDeleteReply {
        prefs(c).edit().remove(name(call.service, call.account)).apply()
        return SecretDeleteReply()
    }
}
