package dev.mygo

import android.Manifest
import android.annotation.SuppressLint
import android.content.Context
import android.location.Location
import android.location.LocationListener
import android.location.LocationManager
import android.os.Build
import android.os.Looper
import org.json.JSONObject

class LocationPlugin : MyGoPlugin {
    private val manager get() = PluginHost.context.getSystemService(Context.LOCATION_SERVICE) as LocationManager
    private val perms = arrayOf(Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION)
    private var listener: LocationListener? = null

    private fun encode(l: Location) = JSONObject()
        .put("latitude", l.latitude).put("longitude", l.longitude)
        .put("altitude", if (l.hasAltitude()) l.altitude else -1.0)
        .put("speed", if (l.hasSpeed()) l.speed.toDouble() else -1.0)
        .put("heading", if (l.hasBearing()) l.bearing.toDouble() else -1.0)
        .put("accuracy", if (l.hasAccuracy()) l.accuracy.toDouble() else -1.0)
        .put("time", l.time)

    private fun provider(): String? {
        val providers = manager.getProviders(true)
        return when {
            LocationManager.FUSED_PROVIDER in providers -> LocationManager.FUSED_PROVIDER
            LocationManager.GPS_PROVIDER in providers -> LocationManager.GPS_PROVIDER
            LocationManager.NETWORK_PROVIDER in providers -> LocationManager.NETWORK_PROVIDER
            else -> providers.firstOrNull()
        }
    }

    @SuppressLint("MissingPermission")
    override fun call(method: String, args: JSONObject, reply: PluginReply) {
        when (method) {
            "requestPermission" -> PluginHost.requestPermissions(perms) { reply.success(JSONObject().put("granted", it)) }
            "current" -> PluginHost.requestPermissions(perms) { granted ->
                if (!granted) {
                    reply.failure(PluginFailure("Denied", "the app may not know where the device is"))
                    return@requestPermissions
                }
                val p = provider()
                if (p == null) {
                    reply.failure(PluginFailure("Failed", "the device has location turned off"))
                    return@requestPermissions
                }
                if (Build.VERSION.SDK_INT >= 30) {
                    manager.getCurrentLocation(p, null, PluginHost.context.mainExecutor) { l ->
                        if (l != null) reply.success(encode(l)) else reply.failure(PluginFailure("Failed", "no fix"))
                    }
                } else {
                    @Suppress("DEPRECATION")
                    manager.requestSingleUpdate(p, { l: Location -> reply.success(encode(l)) }, Looper.getMainLooper())
                }
            }
            "start" -> PluginHost.requestPermissions(perms) { granted ->
                val p = provider()
                if (!granted || p == null) {
                    reply.failure(PluginFailure("Denied", "the app may not know where the device is"))
                    return@requestPermissions
                }
                listener?.let { manager.removeUpdates(it) }
                val l = LocationListener { loc -> PluginHost.emit("location", "fix", encode(loc)) }
                listener = l
                manager.requestLocationUpdates(p, 1000L, 1f, l, Looper.getMainLooper())
                reply.success(null)
            }
            "stop" -> {
                listener?.let { manager.removeUpdates(it) }
                listener = null
                reply.success(null)
            }
            else -> reply.failure(PluginFailure("NoMethod", method))
        }
    }
}
