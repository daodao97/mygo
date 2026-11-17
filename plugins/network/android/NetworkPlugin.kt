package dev.mygo

import org.json.JSONArray
import org.json.JSONObject
import java.net.NetworkInterface

/**
 * Android's sockets take the system's routes: nothing to prepare. Its apps may
 * not read the interfaces through netlink since Android 11, which Java's
 * NetworkInterface still lists.
 */
class NetworkPlugin : MyGoPlugin {
    override fun call(method: String, args: JSONObject, reply: PluginReply) {
        when (method) {
            "prepare", "cancel" -> reply.success(null)
            "interfaces" -> Thread {
                try {
                    val out = JSONArray()
                    for (i in NetworkInterface.getNetworkInterfaces()?.toList() ?: emptyList()) {
                        val addresses = JSONArray()
                        for (a in i.interfaceAddresses) {
                            val host = a.address?.hostAddress?.substringBefore('%') ?: continue
                            addresses.put("$host/${a.networkPrefixLength}")
                        }
                        out.put(JSONObject().put("name", i.name).put("index", i.index).put("mtu", runCatching { i.mtu }.getOrDefault(0))
                            .put("up", runCatching { i.isUp }.getOrDefault(false)).put("loopback", runCatching { i.isLoopback }.getOrDefault(false))
                            .put("addresses", addresses))
                    }
                    reply.success(JSONObject().put("interfaces", out))
                } catch (e: Exception) {
                    reply.failure(PluginFailure("Failed", e.toString()))
                }
            }.start()
            else -> reply.failure(PluginFailure("NoMethod", method))
        }
    }
}
