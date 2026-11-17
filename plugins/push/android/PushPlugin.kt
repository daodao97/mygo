package dev.mygo

import android.Manifest
import android.os.Build
import com.google.firebase.FirebaseApp
import com.google.firebase.FirebaseOptions
import com.google.firebase.messaging.FirebaseMessaging
import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage
import org.json.JSONObject

class PushPlugin : MyGoPlugin {
    private var listening = false

    init {
        PluginHost.onIntent = { if (listening) opened(it) }
    }

    private fun opened(i: android.content.Intent) {
        val extras = i.extras ?: return
        if (!extras.containsKey("google.message_id")) return
        val o = JSONObject()
        for (k in extras.keySet()) extras.getString(k)?.let { o.put(k, it) }
        i.removeExtra("google.message_id") // once, as an Activity made again has it still
        PluginHost.emit("push", "opened", o)
    }

    override fun call(method: String, args: JSONObject, reply: PluginReply) {
        when (method) {
            "configure" -> {
                if (FirebaseApp.getApps(PluginHost.context).isEmpty()) {
                    FirebaseApp.initializeApp(PluginHost.context, FirebaseOptions.Builder()
                        .setProjectId(args.optString("projectId"))
                        .setApplicationId(args.optString("appId"))
                        .setApiKey(args.optString("apiKey"))
                        .setGcmSenderId(args.optString("senderId"))
                        .build())
                }
                reply.success(null)
            }
            "register" -> {
                val perms = if (Build.VERSION.SDK_INT >= 33) arrayOf(Manifest.permission.POST_NOTIFICATIONS) else emptyArray()
                PluginHost.requestPermissions(perms) { granted ->
                    if (!granted) {
                        reply.failure(PluginFailure("Denied", "notifications are not allowed"))
                        return@requestPermissions
                    }
                    if (FirebaseApp.getApps(PluginHost.context).isEmpty()) {
                        reply.failure(PluginFailure("NoFirebase", "call push.Configure first"))
                        return@requestPermissions
                    }
                    FirebaseMessaging.getInstance().token.addOnCompleteListener { t ->
                        if (t.isSuccessful) reply.success(JSONObject().put("token", t.result))
                        else reply.failure(PluginFailure("Failed", t.exception?.toString() ?: ""))
                    }
                }
            }
            "listenOpened" -> {
                // What Firebase opens the app with, as the user tapped a notification.
                listening = true
                PluginHost.activity?.intent?.let { opened(it) }
                reply.success(null)
            }
            else -> reply.failure(PluginFailure("NoMethod", method))
        }
    }
}

/** What Firebase hears of the app's messages. */
class PushService : FirebaseMessagingService() {
    override fun onNewToken(token: String) = PluginHost.emit("push", "token", token)

    override fun onMessageReceived(m: RemoteMessage) {
        val o = JSONObject()
        for ((k, v) in m.data) o.put(k, v)
        m.notification?.let { o.put("title", it.title).put("body", it.body) }
        PluginHost.emit("push", "message", o)
    }
}
