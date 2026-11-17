package dev.mygo

import android.content.Context
import android.hardware.Sensor
import android.hardware.SensorEvent
import android.hardware.SensorEventListener
import android.hardware.SensorManager
import org.json.JSONObject

class SensorsPlugin : MyGoPlugin {
    private val manager get() = PluginHost.context.getSystemService(Context.SENSOR_SERVICE) as SensorManager
    private val listeners = HashMap<String, SensorEventListener>()

    private fun type(name: String) = when (name) {
        "accelerometer" -> Sensor.TYPE_ACCELEROMETER
        "gyroscope" -> Sensor.TYPE_GYROSCOPE
        "magnetometer" -> Sensor.TYPE_MAGNETIC_FIELD
        else -> -1
    }

    override fun call(method: String, args: JSONObject, reply: PluginReply) {
        val name = args.optString("sensor")
        when (method) {
            "start" -> {
                val s = manager.getDefaultSensor(type(name))
                if (s == null) {
                    reply.failure(PluginFailure("NoSensor", name))
                    return
                }
                listeners.remove(name)?.let { manager.unregisterListener(it) }
                val l = object : SensorEventListener {
                    override fun onAccuracyChanged(s: Sensor?, a: Int) {}
                    override fun onSensorChanged(e: SensorEvent) {
                        PluginHost.emit("sensors", name, JSONObject().put("X", e.values[0].toDouble()).put("Y", e.values[1].toDouble())
                            .put("Z", e.values[2].toDouble()).put("T", System.currentTimeMillis()))
                    }
                }
                listeners[name] = l
                manager.registerListener(l, s, args.optInt("intervalMs", 100) * 1000)
                reply.success(null)
            }
            "stop" -> {
                listeners.remove(name)?.let { manager.unregisterListener(it) }
                reply.success(null)
            }
            else -> reply.failure(PluginFailure("NoMethod", method))
        }
    }
}
