// Plugins' native code. A plugin brings Kotlin that implements MyGoPlugin
// (mygo-plugin.json names the class), which mygo build compiles into the app
// and registers by the plugin's id; Go calls it with mygo.Native(id).Call. It
// runs on the UI thread.

package dev.mygo

import android.app.Activity
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import org.json.JSONArray
import org.json.JSONObject
import org.json.JSONTokener

/** An error of a plugin's native code, with a code Go can tell apart. */
class PluginFailure(val code: String, message: String = "") : Exception(message)

/** The answer of a plugin's method: call success or failure once, from any thread. */
interface PluginReply {
    fun success(result: Any?)
    fun failure(error: PluginFailure)
}

/** A plugin's native code. */
interface MyGoPlugin {
    /** Runs a method with its arguments, from JSON, on the UI thread. It may answer later. */
    fun call(method: String, args: JSONObject, reply: PluginReply)
}

/** What plugins have of the host. */
object PluginHost {
    private val plugins = HashMap<String, MyGoPlugin>()
    private var nextCode = 0x7000
    private val onResults = HashMap<Int, (Int, Intent?) -> Unit>()
    private val onPermissions = HashMap<Int, (Boolean) -> Unit>()

    private val viewMakers = HashMap<String, (Context, Long, JSONObject) -> android.view.View>()

    fun register(id: String, plugin: MyGoPlugin) { plugins[id] = plugin }

    /** Registers a type of view that mygo.NewPluginView makes: make gets the number that calls about the view name it, and the arguments. */
    fun registerView(type: String, make: (context: Context, id: Long, args: JSONObject) -> android.view.View) {
        viewMakers[type] = make
    }

    internal fun makeView(type: String, context: Context, id: Long, args: String): android.view.View? {
        // A native module's view, which ui.NativeView shows: its handle.
        if (type == "mygo.native") return Native.view(args.toLongOrNull() ?: 0)
        val a = (if (args.isEmpty()) null else runCatching { JSONTokener(args).nextValue() as? JSONObject }.getOrNull()) ?: JSONObject()
        return viewMakers[type]?.invoke(context, id, a)
    }

    /** Hears the Intents that open the app, for what a plugin's own open it with. */
    var onIntent: ((Intent) -> Unit)? = null

    /** The app's context, which outlives Activities. */
    val context: Context get() = HostImpl.app

    /** The Activity showing the app, null while none does. */
    val activity: Activity? get() = HostImpl.activity

    /** Sends an event to Go (mygo.Native(id).On), from any thread. */
    fun emit(plugin: String, name: String, data: Any? = null) {
        Go.send(Event.pluginEvent(plugin, name, encode(data)))
    }

    /** Starts an Activity for its result. */
    fun startForResult(intent: Intent, done: (resultCode: Int, data: Intent?) -> Unit) {
        val a = activity
        if (a == null) {
            done(Activity.RESULT_CANCELED, null)
            return
        }
        val code = nextCode++
        onResults[code] = done
        @Suppress("DEPRECATION")
        a.startActivityForResult(intent, code)
    }

    /** Has the system ask the user for permissions the app lacks, and tells whether it has them all. */
    fun requestPermissions(permissions: Array<String>, done: (granted: Boolean) -> Unit) {
        val a = activity
        if (a == null) {
            done(false)
            return
        }
        val missing = permissions.filter { a.checkSelfPermission(it) != PackageManager.PERMISSION_GRANTED }
        if (missing.isEmpty()) {
            done(true)
            return
        }
        val code = nextCode++
        onPermissions[code] = done
        a.requestPermissions(missing.toTypedArray(), code)
    }

    internal fun activityResult(code: Int, result: Int, data: Intent?): Boolean {
        val f = onResults.remove(code) ?: return false
        f(result, data)
        return true
    }

    internal fun permissionsResult(code: Int, results: IntArray): Boolean {
        val f = onPermissions.remove(code) ?: return false
        f(results.isNotEmpty() && results.all { it == PackageManager.PERMISSION_GRANTED })
        return true
    }

    /** JSON of what org.json can write. */
    internal fun encode(v: Any?): String = when (v) {
        null -> "null"
        is JSONObject, is JSONArray -> v.toString()
        is Map<*, *> -> JSONObject(v).toString()
        is Collection<*> -> JSONArray(v).toString()
        else -> JSONObject.wrap(v)?.let { w -> JSONArray().put(w).toString().let { it.substring(1, it.length - 1) } } ?: "null"
    }

    internal fun call(c: PluginCallCall) {
        val p = plugins[c.plugin]
        if (p == null) {
            Go.send(Event.pluginResult(c.req, "", "no plugin ${c.plugin} in this app", "NoPlugin"))
            return
        }
        val args = (if (c.args.isEmpty()) null else runCatching { JSONTokener(c.args).nextValue() as? JSONObject }.getOrNull()) ?: JSONObject()
        var answered = false
        val reply = object : PluginReply {
            override fun success(result: Any?) {
                if (answered) return
                val text = try {
                    encode(result)
                } catch (e: Exception) {
                    failure(PluginFailure("Encoding", e.toString()))
                    return
                }
                answered = true
                Go.send(Event.pluginResult(c.req, text, "", ""))
            }
            override fun failure(error: PluginFailure) {
                if (answered) return
                answered = true
                Go.send(Event.pluginResult(c.req, "", error.message ?: "", error.code))
            }
        }
        try {
            p.call(c.method, args, reply)
        } catch (e: Exception) {
            reply.failure(PluginFailure("Exception", e.toString()))
        }
    }
}

/** A window showing a plugin's view, which a surface holds. */
internal class PluginViewWindow(context: Context, id: Long, override val view: android.view.View) : HostWindow(context, id) {
    override fun setBackground(argb: Int) = view.setBackgroundColor(argb)
}
