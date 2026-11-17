package dev.mygo

import android.content.ActivityNotFoundException
import android.net.Uri
import androidx.browser.customtabs.CustomTabsIntent
import org.json.JSONObject

/** A Custom Tab, started for its result so that the answer comes as the user closes it. */
class BrowserPlugin : MyGoPlugin {
    private var showing = 0L
    private var pending: PluginReply? = null

    override fun call(method: String, args: JSONObject, reply: PluginReply) {
        val id = args.optLong("id")
        when (method) {
            "open" -> {
                val uri = Uri.parse(args.optString("url"))
                if (uri.scheme != "http" && uri.scheme != "https") {
                    reply.failure(PluginFailure("Invalid", "not an http or https URL"))
                    return
                }
                if (pending != null) {
                    reply.failure(PluginFailure("Busy", "a page shows"))
                    return
                }
                val intent = CustomTabsIntent.Builder().setShowTitle(true).build().intent.setData(uri)
                pending = reply
                showing = id
                try {
                    PluginHost.startForResult(intent) { _, _ -> if (showing == id) finish() }
                } catch (e: ActivityNotFoundException) {
                    pending = null
                    showing = 0
                    reply.failure(PluginFailure("Unavailable", "no browser"))
                }
            }
            "close" -> {
                // A Custom Tab is the browser's to close: Go stops waiting.
                if (showing == id) finish()
                reply.success(null)
            }
            else -> reply.failure(PluginFailure("NoMethod", method))
        }
    }

    private fun finish() {
        val r = pending ?: return
        pending = null
        showing = 0
        r.success(null)
    }
}
