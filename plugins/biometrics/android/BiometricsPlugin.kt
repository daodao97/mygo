package dev.mygo

import android.app.KeyguardManager
import android.content.Context
import android.content.pm.PackageManager
import android.hardware.biometrics.BiometricPrompt
import android.os.Build
import android.os.CancellationSignal
import org.json.JSONObject

class BiometricsPlugin : MyGoPlugin {
    // busy is set while a prompt shows; main thread only.
    private var busy = false

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
                if (Build.VERSION.SDK_INT < 28) {
                    reply.failure(PluginFailure("Unavailable", "needs Android 9"))
                    return
                }
                ctx.mainExecutor.execute { authenticate(ctx, keyguard, args, reply) }
            }
            else -> reply.failure(PluginFailure("NoMethod", method))
        }
    }

    private fun authenticate(ctx: Context, keyguard: KeyguardManager, args: JSONObject, reply: PluginReply) {
        val activity = PluginHost.activity
        when {
            busy -> { reply.failure(PluginFailure("Busy")); return }
            activity == null || activity.isFinishing || !activity.hasWindowFocus() -> { reply.failure(PluginFailure("Inactive")); return }
            !keyguard.isDeviceSecure -> { reply.failure(PluginFailure("PasscodeNotSet")); return }
        }
        busy = true
        val done = { f: PluginFailure? ->
            busy = false
            if (f == null) reply.success(null) else reply.failure(f)
        }
        val b = BiometricPrompt.Builder(activity!!)
            .setTitle(args.optString("reason").ifEmpty { " " })
        if (Build.VERSION.SDK_INT >= 30) {
            b.setAllowedAuthenticators(android.hardware.biometrics.BiometricManager.Authenticators.BIOMETRIC_WEAK or
                android.hardware.biometrics.BiometricManager.Authenticators.DEVICE_CREDENTIAL)
        } else if (Build.VERSION.SDK_INT >= 29) {
            @Suppress("DEPRECATION")
            b.setDeviceCredentialAllowed(true)
        } else {
            b.setNegativeButton(ctx.getString(android.R.string.cancel), ctx.mainExecutor) { _, _ -> done(PluginFailure("Canceled")) }
        }
        b.build().authenticate(CancellationSignal(), ctx.mainExecutor, object : BiometricPrompt.AuthenticationCallback() {
            override fun onAuthenticationSucceeded(r: BiometricPrompt.AuthenticationResult) = done(null)
            override fun onAuthenticationError(code: Int, msg: CharSequence) = done(failure(code, msg.toString()))
        })
    }

    private fun failure(code: Int, msg: String): PluginFailure = when (code) {
        BiometricPrompt.BIOMETRIC_ERROR_USER_CANCELED, BiometricPrompt.BIOMETRIC_ERROR_CANCELED -> PluginFailure("Canceled")
        BiometricPrompt.BIOMETRIC_ERROR_HW_UNAVAILABLE, BiometricPrompt.BIOMETRIC_ERROR_HW_NOT_PRESENT -> PluginFailure("Unavailable", msg)
        BiometricPrompt.BIOMETRIC_ERROR_NO_BIOMETRICS -> PluginFailure("NotEnrolled", msg)
        BiometricPrompt.BIOMETRIC_ERROR_LOCKOUT, BiometricPrompt.BIOMETRIC_ERROR_LOCKOUT_PERMANENT -> PluginFailure("LockedOut", msg)
        BiometricPrompt.BIOMETRIC_ERROR_NO_DEVICE_CREDENTIAL -> PluginFailure("PasscodeNotSet", msg)
        else -> PluginFailure("Failed", msg)
    }
}
