package dev.mygo

import org.json.JSONObject

/** Android's sockets take the system's routes: nothing to prepare. */
class NetworkPlugin : MyGoPlugin {
    override fun call(method: String, args: JSONObject, reply: PluginReply) {
        when (method) {
            "prepare", "cancel" -> reply.success(null)
            else -> reply.failure(PluginFailure("NoMethod", method))
        }
    }
}
