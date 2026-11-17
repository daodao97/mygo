package dev.mygo

import android.content.Context
import android.os.Handler
import android.os.Looper
import androidx.work.Constraints
import androidx.work.Data
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequest
import androidx.work.PeriodicWorkRequest
import androidx.work.WorkManager
import androidx.work.Worker
import androidx.work.WorkerParameters
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import org.json.JSONObject

/** WorkManager, each of the app's tasks a unique work of its name. */
class BackgroundPlugin : MyGoPlugin {
    init {
        instance = this
    }

    override fun call(method: String, args: JSONObject, reply: PluginReply) {
        val wm = WorkManager.getInstance(PluginHost.context)
        when (method) {
            "ready" -> {
                ready = true
                for (e in waiting) PluginHost.emit("background", "run", e)
                waiting.clear()
                reply.success(null)
            }
            "schedule" -> {
                val name = args.optString("name")
                // The same again, as each launch asks: what's scheduled stands.
                // Replacing it would put it off, or stop the task that started
                // this very process.
                val options = args.toString()
                if (prefs().getString(name, null) == options) {
                    reply.success(null)
                    return
                }
                val every = args.optDouble("every", 0.0)
                val delay = (args.optDouble("delay", 0.0) * 1000).toLong()
                val c = Constraints.Builder()
                    .setRequiredNetworkType(if (args.optBoolean("network")) NetworkType.CONNECTED else NetworkType.NOT_REQUIRED)
                    .setRequiresCharging(args.optBoolean("charging"))
                    .build()
                val data = Data.Builder().putString("name", name).build()
                if (every > 0) {
                    val r = PeriodicWorkRequest.Builder(MyGoWorker::class.java, (every * 1000).toLong(), TimeUnit.MILLISECONDS)
                        .setConstraints(c).setInputData(data).setInitialDelay(delay, TimeUnit.MILLISECONDS)
                        .addTag(MyGoWorker.PERIODIC).build()
                    wm.enqueueUniquePeriodicWork(prefix + name, ExistingPeriodicWorkPolicy.UPDATE, r)
                } else {
                    val r = OneTimeWorkRequest.Builder(MyGoWorker::class.java)
                        .setConstraints(c).setInputData(data).setInitialDelay(delay, TimeUnit.MILLISECONDS).build()
                    wm.enqueueUniqueWork(prefix + name, ExistingWorkPolicy.REPLACE, r)
                }
                prefs().edit().putString(name, options).apply()
                reply.success(null)
            }
            "cancel" -> {
                wm.cancelUniqueWork(prefix + args.optString("name"))
                prefs().edit().remove(args.optString("name")).apply()
                reply.success(null)
            }
            "done" -> {
                val id = args.optLong("run")
                val d = runs.remove(id)
                d?.invoke(args.optBoolean("ok"))
                reply.success(null)
            }
            else -> reply.failure(PluginFailure("UnknownMethod", method))
        }
    }

    companion object {
        private const val prefix = "mygo."
        internal var instance: BackgroundPlugin? = null
        private var ready = false
        private val waiting = ArrayList<Map<String, Any>>()
        private val runs = HashMap<Long, (Boolean) -> Unit>()
        private var nextRun = 1L

        /** Runs a task, on the UI thread: Go hears of it, once it listens. */
        internal fun run(name: String, done: (Boolean) -> Unit): Long {
            val id = nextRun++
            runs[id] = done
            val e = mapOf<String, Any>("run" to id, "name" to name)
            if (ready) PluginHost.emit("background", "run", e) else waiting.add(e)
            return id
        }

        /** The system's time ran out: what never began stays unbegun. */
        internal fun stop(id: Long) {
            runs.remove(id)
            if (!waiting.removeAll { it["run"] == id }) PluginHost.emit("background", "stop", mapOf("run" to id))
        }

        internal fun prefs() = PluginHost.context.getSharedPreferences("mygo.background", Context.MODE_PRIVATE)

        /** A task that runs once is done with: it may be scheduled again. */
        internal fun finished(name: String) {
            prefs().edit().remove(name).apply()
        }
    }
}

/** A task's turn, which may be what starts the process: Go starts with no Activity. */
class MyGoWorker(context: Context, params: WorkerParameters) : Worker(context, params) {
    @Volatile private var run = 0L
    private val main = Handler(Looper.getMainLooper())
    private val latch = CountDownLatch(1)

    override fun doWork(): Result {
        val name = inputData.getString("name") ?: return Result.failure()
        var ok = false
        main.post {
            HostImpl.startHeadless(applicationContext)
            run = BackgroundPlugin.run(name) {
                ok = it
                latch.countDown()
            }
        }
        // WorkManager's limit is 10 minutes.
        if (!latch.await(9, TimeUnit.MINUTES)) {
            onStopped()
            return Result.retry()
        }
        if (!ok) return Result.retry()
        // What repeats stays on the list, of which this run is one.
        if (!tags.contains(MyGoWorker.PERIODIC)) main.post { BackgroundPlugin.finished(name) }
        return Result.success()
    }

    override fun onStopped() {
        val id = run
        if (id != 0L) main.post { BackgroundPlugin.stop(id) }
        latch.countDown() // WorkManager no longer waits for this thread
    }

    companion object {
        const val PERIODIC = "mygo.periodic"
    }
}
