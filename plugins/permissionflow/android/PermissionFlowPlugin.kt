package dev.mygo

import android.Manifest
import android.app.NotificationManager
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.provider.Settings
import org.json.JSONObject

class PermissionFlowPlugin : MyGoPlugin {
    /** The manifest's names of a permission, which an app declares (android.permissions). */
    private fun names(p: String): Array<String>? = when (p) {
        "camera" -> arrayOf(Manifest.permission.CAMERA)
        "microphone" -> arrayOf(Manifest.permission.RECORD_AUDIO)
        "photos" -> if (Build.VERSION.SDK_INT >= 33) arrayOf(Manifest.permission.READ_MEDIA_IMAGES) else arrayOf(Manifest.permission.READ_EXTERNAL_STORAGE)
        "contacts" -> arrayOf(Manifest.permission.READ_CONTACTS)
        "location" -> arrayOf(Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION)
        "bluetooth" -> if (Build.VERSION.SDK_INT >= 31) arrayOf(Manifest.permission.BLUETOOTH_CONNECT) else emptyArray()
        "calendars" -> arrayOf(Manifest.permission.READ_CALENDAR, Manifest.permission.WRITE_CALENDAR)
        "notifications" -> if (Build.VERSION.SDK_INT >= 33) arrayOf(Manifest.permission.POST_NOTIFICATIONS) else emptyArray()
        else -> null
    }

    private val asked get() = PluginHost.context.getSharedPreferences("mygo.permissions", Context.MODE_PRIVATE)

    private fun status(p: String, perms: Array<String>): String {
        val ctx = PluginHost.context
        if (p == "notifications" && !(ctx.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager).areNotificationsEnabled()) {
            return if (asked.getBoolean(p, false) || perms.isEmpty()) "denied" else "notDetermined"
        }
        if (perms.all { ctx.checkSelfPermission(it) == PackageManager.PERMISSION_GRANTED }) return "granted"
        return if (asked.getBoolean(p, false)) "denied" else "notDetermined"
    }

    /** Whether the app's manifest names them: Android turns down others without a word. */
    private fun declared(perms: Array<String>): Boolean {
        val ctx = PluginHost.context
        val info = ctx.packageManager.getPackageInfo(ctx.packageName, PackageManager.GET_PERMISSIONS)
        val listed = info.requestedPermissions ?: emptyArray()
        return perms.all { it in listed }
    }

    override fun call(method: String, args: JSONObject, reply: PluginReply) {
        val p = args.optString("permission")
        val perms = names(p)
        when (method) {
            "check" -> if (perms == null) reply.failure(PluginFailure("Unsupported")) else reply.success(JSONObject().put("status", status(p, perms)))
            "request" -> {
                if (perms == null) {
                    reply.failure(PluginFailure("Unsupported"))
                    return
                }
                if (!declared(perms)) {
                    reply.failure(PluginFailure("NoUsageDescription", "android.permissions lacks ${perms.joinToString()}"))
                    return
                }
                PluginHost.requestPermissions(perms) {
                    asked.edit().putBoolean(p, true).apply()
                    reply.success(JSONObject().put("status", status(p, perms)))
                }
            }
            "openSettings" -> {
                val ctx = PluginHost.context
                val i = Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.parse("package:" + ctx.packageName))
                (PluginHost.activity ?: ctx).startActivity(i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
                reply.success(null)
            }
            else -> reply.failure(PluginFailure("NoMethod", method))
        }
    }
}
