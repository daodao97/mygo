package dev.mygo

import android.app.KeyguardManager
import android.content.Context
import android.content.pm.PackageManager
import android.hardware.biometrics.BiometricPrompt
import android.os.Build
import android.os.CancellationSignal
import org.json.JSONObject

class BiometricsPlugin : MyGoPlugin {
    override fun call(method: String, args: JSONObject, reply: PluginReply) {
        val ctx = PluginHost.context
        val keyguard = ctx.getSystemService(Context.KEYGUARD_SERVICE) as KeyguardManager
        when (method) {
            "available" -> {
                val pm = ctx.packageManager
                val kind = when {
                    Build.VERSION.SDK_INT < 28 -> ""
                    pm.hasSystemFeature(PackageManager.FEATURE_FINGERPRINT) && keyguard.isDeviceSecure -> "fingerprint"
                    pm.hasSystemFeature(PackageManager.FEATURE_FACE) && keyguard.isDeviceSecure -> "face"
                    pm.hasSystemFeature(PackageManager.FEATURE_IRIS) && keyguard.isDeviceSecure -> "iris"
                    keyguard.isDeviceSecure -> "passcode"
                    else -> ""
                }
                reply.success(JSONObject().put("kind", kind))
            }
            "authenticate" -> {
                if (Build.VERSION.SDK_INT < 28 || PluginHost.activity == null) {
                    reply.failure(PluginFailure("Failed", "needs Android 9"))
                    return
                }
                val b = BiometricPrompt.Builder(PluginHost.activity!!)
                    .setTitle(args.optString("reason").ifEmpty { " " })
                if (Build.VERSION.SDK_INT >= 30) {
                    b.setAllowedAuthenticators(android.hardware.biometrics.BiometricManager.Authenticators.BIOMETRIC_WEAK or
                        android.hardware.biometrics.BiometricManager.Authenticators.DEVICE_CREDENTIAL)
                } else if (Build.VERSION.SDK_INT >= 29) {
                    @Suppress("DEPRECATION")
                    b.setDeviceCredentialAllowed(true)
                } else {
                    b.setNegativeButton(ctx.getString(android.R.string.cancel), ctx.mainExecutor) { _, _ -> reply.failure(PluginFailure("Canceled")) }
                }
                b.build().authenticate(CancellationSignal(), ctx.mainExecutor, object : BiometricPrompt.AuthenticationCallback() {
                    override fun onAuthenticationSucceeded(r: BiometricPrompt.AuthenticationResult) = reply.success(null)
                    override fun onAuthenticationError(code: Int, msg: CharSequence) {
                        val canceled = code == BiometricPrompt.BIOMETRIC_ERROR_USER_CANCELED || code == BiometricPrompt.BIOMETRIC_ERROR_CANCELED
                        reply.failure(if (canceled) PluginFailure("Canceled") else PluginFailure("Failed", msg.toString()))
                    }
                })
            }
            else -> reply.failure(PluginFailure("NoMethod", method))
        }
    }
}
