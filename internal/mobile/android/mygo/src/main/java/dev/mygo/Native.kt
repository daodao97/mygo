// Native modules on Android: Go interfaces implemented in Kotlin (see
// docs/native-modules.md). Go calls the module's generated object
// (MygoModule, a NativeModule) through JNI (internal/mobile's
// jni_android.c), on the calling goroutine's thread, as it calls a Swift
// library's functions elsewhere; the module tells Go what happens later
// through Native.send, the callback every module shares.
//
// Values cross in package native's binary encoding: every integer as 8
// bytes, little-endian, floats as a Double's bits, bools as a byte,
// strings, byte arrays, lists and maps as their count then their contents,
// optionals as a bool then the value, structs as their fields, enums as
// their values.

package dev.mygo

import android.os.Handler
import android.os.Looper
import java.lang.ref.PhantomReference
import java.lang.ref.ReferenceQueue
import java.nio.ByteBuffer
import java.time.Instant
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.CountDownLatch
import java.util.concurrent.atomic.AtomicLong
import kotlin.coroutines.resume
import kotlin.time.Duration
import kotlin.time.Duration.Companion.nanoseconds
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.launch
import kotlinx.coroutines.suspendCancellableCoroutine

// What the implementation uses.

/** What the system lacks, such as an API of a newer Android: Go gets errors.ErrUnsupported. */
class NativeUnsupportedException(message: String = "unsupported") : Exception(message)

/** A call of an object that was closed: Go gets native.ErrClosed. */
class NativeClosedException(message: String = "native: object closed") : Exception(message)

/** An error a Go function or method returned. */
class NativeGoException(message: String) : Exception(message)

/**
 * An error of a domain of the platform's, as an NSError: Go gets a
 * *native.Error with its domain, number and message. Any other exception
 * reaches Go with its class as the domain.
 */
open class NativeException(val domain: String, val number: Long, message: String) : Exception(message)

/** A value Go sent that the implementation's types do not have. */
class NativeWireException(message: String) : Exception(message)

/** An error a module declares (native.NewError): Go gets that error. Each module's exception implements it. */
interface NativeCodedError {
    val nativeCode: String
}

/** An AutoCloseable that runs a function, once: what a method returning io.Closer returns. */
class NativeCloser(fn: () -> Unit) : AutoCloseable {
    private var fn: (() -> Unit)? = fn

    override fun close() {
        val f = synchronized(this) { fn.also { fn = null } }
        f?.invoke()
    }
}

/** Memory of Go's that the implementation reads and writes, valid while the call runs: a *native.Buffer. */
class NativeBuffer internal constructor(
    /** The address of the memory. */
    val address: Long,
    /** Its size in bytes. */
    val size: Int,
) {
    /** The memory, as a direct buffer. */
    val bytes: ByteBuffer by lazy { Native.buffer(address, size.toLong()).order(java.nio.ByteOrder.nativeOrder()) }
}

// The encoding.

/** Writes values for Go, after 8 bytes for the status of an answer. */
class NativeWriter {
    private var b = ByteArray(64)
    private var n = HEAD

    private fun room(k: Int) {
        if (n + k > b.size) b = b.copyOf(maxOf(b.size * 2, n + k))
    }

    fun u64(v: Long) {
        room(8)
        for (i in 0 until 8) b[n + i] = (v ushr (8 * i)).toByte()
        n += 8
    }

    fun i64(v: Long) = u64(v)
    fun bool(v: Boolean) { room(1); b[n++] = if (v) 1 else 0 }
    fun u8(v: Int) { room(1); b[n++] = v.toByte() }
    fun double(v: Double) = u64(v.toRawBits())
    fun count(c: Int) = i64(c.toLong())
    fun string(v: String) = data(v.toByteArray(Charsets.UTF_8))

    fun data(v: ByteArray) {
        count(v.size)
        room(v.size)
        System.arraycopy(v, 0, b, n, v.size)
        n += v.size
    }

    /** A time: seconds and nanoseconds since 1970. */
    fun time(v: Instant) { i64(v.epochSecond); i64(v.nano.toLong()) }
    fun duration(v: Duration) = i64(v.inWholeNanoseconds)
    fun rect(v: android.graphics.RectF) {
        double(v.left.toDouble()); double(v.top.toDouble()); double(v.width().toDouble()); double(v.height().toDouble())
    }

    fun reset() { n = HEAD }

    /** The values written. */
    fun values(): ByteArray? = if (n == HEAD) null else b.copyOfRange(HEAD, n)

    /** The values as an answer of status. */
    fun answer(status: Long): ByteArray {
        for (i in 0 until 8) b[i] = (status ushr (8 * i)).toByte()
        return b.copyOf(n)
    }

    private companion object {
        const val HEAD = 8
    }
}

/** Reads the values Go wrote; past them, it throws. */
class NativeReader(private val b: ByteArray, private var p: Int = 0) {
    private fun take(k: Int): Int {
        if (k < 0 || b.size - p < k) throw NativeWireException("native: a short message")
        val at = p
        p += k
        return at
    }

    fun u64(): Long {
        val at = take(8)
        var v = 0L
        for (i in 7 downTo 0) v = (v shl 8) or (b[at + i].toLong() and 0xff)
        return v
    }

    fun i64(): Long = u64()
    fun u8(): Int = b[take(1)].toInt() and 0xff
    fun bool(): Boolean = u8() != 0
    fun double(): Double = Double.fromBits(u64())

    /** The count of a list or a map, whose elements take a byte each at least. */
    fun count(): Int {
        val c = i64()
        if (c < 0 || c > b.size - p) throw NativeWireException("native: a short message")
        return c.toInt()
    }

    fun string(): String {
        val c = count()
        val at = take(c)
        return String(b, at, c, Charsets.UTF_8)
    }

    fun data(): ByteArray {
        val c = count()
        val at = take(c)
        return b.copyOfRange(at, at + c)
    }

    fun time(): Instant {
        val s = i64()
        val ns = i64()
        return Instant.ofEpochSecond(s, ns)
    }

    fun duration(): Duration = i64().nanoseconds
    fun rect(): android.graphics.RectF {
        val x = double().toFloat()
        val y = double().toFloat()
        val w = double().toFloat()
        val h = double().toFloat()
        return android.graphics.RectF(x, y, x + w, y + h)
    }

    fun buffer(): NativeBuffer {
        val p = u64()
        return NativeBuffer(p, i64().toInt())
    }
}

/**
 * A module's Kotlin, as mygo generates it (its MygoModule object), which
 * Go loads by its class's name.
 */
abstract class NativeModule {
    /** The module's index, from Go, in the high bits of every id, which makes the ids of continuations unique among modules. */
    @Volatile internal var idBase = 0L
    private val lastID = AtomicLong()

    /** Calls a function: an object's method, a factory (new_T), or close. */
    abstract fun invoke(sym: String, h: Long, args: ByteArray, call: Long): ByteArray

    /** Calls a method of numbers. */
    open fun direct(sym: String, h: Long, args: LongArray): Long = 0

    /** The exception of an error the module declares, by its code. */
    open fun error(code: String): Throwable? = null

    protected fun nextID(): Long = idBase or lastID.incrementAndGet()

    /** Runs what returns at once, which writes its result to the writer. */
    protected inline fun answer(body: (NativeWriter) -> Unit): ByteArray {
        val w = NativeWriter()
        return try {
            body(w)
            w.answer(0)
        } catch (e: Throwable) {
            w.reset()
            Native.writeError(w, e)
            w.answer(1)
        }
    }

    /** Runs what starts an asynchronous call or a stream: Go waits for its results through the callback. */
    protected inline fun start(body: () -> Unit): ByteArray {
        val w = NativeWriter()
        return try {
            body()
            w.answer(2)
        } catch (e: Throwable) {
            Native.writeError(w, e)
            w.answer(1)
        }
    }

    /** Starts the task of an asynchronous call, on the main thread when main. */
    protected fun task(call: Long, main: Boolean, body: suspend (NativeWriter) -> Unit) {
        val job = Native.scope.launch(if (main) Dispatchers.Main else Dispatchers.Default) {
            val w = NativeWriter()
            var status = 0L
            try {
                body(w)
            } catch (e: Throwable) {
                w.reset()
                Native.writeError(w, e)
                status = 1
            }
            Native.tasks.finish(call)
            Native.send(KIND_RESULT, call, status, w.values())
        }
        Native.tasks.put(call, job)
    }

    /** Starts a stream: each value Go asks for (next), then its end or its error. */
    protected fun <T> stream(call: Long, main: Boolean, make: () -> Flow<T>, enc: (NativeWriter, T) -> Unit) {
        val demand = Channel<Unit>(Channel.UNLIMITED)
        Native.demands[call] = demand
        val job = Native.scope.launch(if (main) Dispatchers.Main else Dispatchers.Default) {
            val w = NativeWriter()
            try {
                demand.receive()
                make().collect { v ->
                    w.reset()
                    enc(w, v)
                    Native.send(KIND_ITEM, call, 0, w.values() ?: ByteArray(0))
                    demand.receive()
                }
                Native.send(KIND_ITEM, call, 3, null)
            } catch (e: CancellationException) {
                // Go stopped the stream, and hears no more of it.
            } catch (e: Throwable) {
                w.reset()
                Native.writeError(w, e)
                Native.send(KIND_ITEM, call, 1, w.values())
            } finally {
                Native.demands.remove(call)
                Native.tasks.finish(call)
            }
        }
        Native.tasks.put(call, job)
    }

    /** The object of a handle Go passed. */
    @Suppress("UNCHECKED_CAST")
    protected fun <T> obj(h: Long): T = (Native.objects[h] ?: throw NativeClosedException()) as T

    /** Keeps an object for Go: its handle. */
    protected fun retain(v: Any): Long = Native.retain(v)

    /** Closes an object Go closes, as io.Closer's Close. */
    protected fun close(h: Long) {
        when (val o = Native.objects[h]) {
            is AutoCloseable -> o.close()
            is android.view.View -> (o.parent as? android.view.ViewGroup)?.removeView(o)
        }
    }

    /** The view of a window Go passed: the window's own. */
    protected fun window(h: Long): android.view.View =
        HostImpl.windows[h]?.view ?: throw NativeClosedException("native: the window is closed")

    /** A rectangle of native UI, in points from the window's top-left, in the pixels of the window's view. */
    protected fun pixels(r: android.graphics.RectF, w: android.view.View): android.graphics.RectF {
        val d = w.resources.displayMetrics.density
        return android.graphics.RectF(r.left * d, r.top * d, r.right * d, r.bottom * d)
    }

    protected fun unknown(sym: String): ByteArray = answer { throw NativeUnsupportedException("native: no function $sym: generate the module's Kotlin again") }

    /** Calls the method of a value of Go's, which answers at once. */
    fun <R> invokeGo(id: Long, w: NativeWriter, read: (NativeReader) -> R): R =
        Native.answerOf(this, Native.send(KIND_INVOKE, id, 0, w.values()), read)

    /** Calls the method of a value of Go's that takes a context, which the coroutine's cancellation cancels. */
    suspend fun <R> invokeGoAsync(id: Long, w: NativeWriter, read: (NativeReader) -> R): R {
        val cont = nextID()
        val b = try {
            suspendCancellableCoroutine<ByteArray?> { c ->
                Native.conts[cont] = { b -> c.resume(b) }
                c.invokeOnCancellation { Native.send(KIND_CANCEL, cont, 0, null) }
                Native.send(KIND_INVOKE_ASYNC, id, cont, w.values())
            }
        } finally {
            Native.conts.remove(cont)
        }
        return Native.answerOf(this, b, read)
    }

    /** A Go function the implementation calls, released once nothing holds it. */
    fun goFunc(id: Long): GoFunc = GoFunc(this, id)

    /** Ties the release of the value of Go's id to the life of holder. */
    fun releaseWith(holder: Any, id: Long) = Native.releaseWith(holder, id)

    companion object {
        const val KIND_RESULT = 1L
        const val KIND_ITEM = 2L
        const val KIND_NOTIFY = 3L
        const val KIND_ASK = 4L
        const val KIND_RELEASE = 5L
        const val KIND_INVOKE = 6L
        const val KIND_INVOKE_ASYNC = 7L
        const val KIND_CANCEL = 8L
    }
}

/** A Go function the implementation calls: a notification, or a question Go answers at once. */
class GoFunc internal constructor(private val module: NativeModule, val id: Long) {
    init {
        if (id != 0L) Native.releaseWith(this, id)
    }

    fun notify(w: NativeWriter) {
        if (id != 0L) Native.send(NativeModule.KIND_NOTIFY, id, 0, w.values())
    }

    fun <R> ask(w: NativeWriter, read: (NativeReader) -> R): R {
        if (id == 0L) throw NativeClosedException()
        return Native.answerOf(module, Native.send(NativeModule.KIND_ASK, id, 0, w.values()), read)
    }
}

/** The runtime of native modules' Kotlin, which Go reaches through JNI. */
object Native {
    private val modules = ConcurrentHashMap<String, NativeModule>()

    /** The objects Go holds, by handle. */
    internal val objects = ConcurrentHashMap<Long, Any>()
    private val lastHandle = AtomicLong()
    internal val tasks = Tasks()
    internal val demands = ConcurrentHashMap<Long, Channel<Unit>>()
    internal val conts = ConcurrentHashMap<Long, (ByteArray?) -> Unit>()

    /** Where the tasks of asynchronous calls and streams run. */
    @JvmField val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    private val main = Handler(Looper.getMainLooper())

    /** The application's context, for a module's Android APIs, from any thread. */
    val context: android.content.Context get() = HostImpl.app

    /** The activity showing the app, if one is, for what needs one (a dialog, a permission request). Main thread. */
    val activity: android.app.Activity? get() = HostImpl.activity

    /** Runs body on the main thread, now if this is it, else waiting for it: to make an object there. */
    fun <T> onMain(body: () -> T): T {
        if (Looper.myLooper() == Looper.getMainLooper()) return body()
        var result: Result<T>? = null
        val latch = CountDownLatch(1)
        main.post {
            result = runCatching(body)
            latch.countDown()
        }
        latch.await()
        return result!!.getOrThrow()
    }

    // What Go calls, through JNI.

    /** Loads a module's Kotlin: its generated object's class. Returns what went wrong, or null. */
    @JvmStatic fun init(name: String, cls: String, idBase: Long): String? = try {
        val m = Class.forName(cls).getField("INSTANCE").get(null) as NativeModule
        m.idBase = idBase
        modules[name] = m
        null
    } catch (e: ReflectiveOperationException) {
        "the app has no Kotlin of the native module $name ($cls): build it with mygo build"
    } catch (e: Throwable) {
        "loading the Kotlin of the native module $name: $e"
    }

    @JvmStatic fun invoke(name: String, sym: String, h: Long, args: ByteArray, call: Long): ByteArray {
        val m = modules[name] ?: return NativeWriter().also { writeError(it, NativeClosedException("native: $name is not loaded")) }.answer(1)
        return m.invoke(sym, h, args, call)
    }

    @JvmStatic fun direct(name: String, sym: String, h: Long, args: LongArray): Long = modules[name]?.direct(sym, h, args) ?: 0

    @JvmStatic fun op(name: String, op: String, a: Long, status: Long, data: ByteArray?) {
        when (op) {
            "release" -> objects.remove(a)
            "cancel" -> tasks.cancel(a)
            "next" -> demands[a]?.trySend(Unit)
            "resume" -> conts.remove(a)?.invoke(answer(status, data))
        }
    }

    /** Tells Go something; its answer is the status and then the data, null when there is none. */
    @JvmStatic external fun send(kind: Long, id: Long, status: Long, data: ByteArray?): ByteArray?

    /** The n bytes at p, which Go owns, as a direct buffer. Public, as JNI finds it by its name. */
    @JvmStatic external fun buffer(p: Long, n: Long): ByteBuffer

    /** The view of a handle, for ui.NativeView. */
    internal fun view(h: Long): android.view.View? = objects[h] as? android.view.View

    internal fun retain(v: Any): Long {
        val h = lastHandle.incrementAndGet()
        objects[h] = v
        return h
    }

    private fun answer(status: Long, data: ByteArray?): ByteArray {
        val b = ByteArray(8 + (data?.size ?: 0))
        for (i in 0 until 8) b[i] = (status ushr (8 * i)).toByte()
        data?.copyInto(b, 8)
        return b
    }

    /** Reads an answer of Go's. */
    internal fun <R> answerOf(m: NativeModule, b: ByteArray?, read: (NativeReader) -> R): R {
        if (b == null || b.size < 8) throw NativeClosedException()
        val r = NativeReader(b)
        val status = r.u64()
        if (status == 1L) throw readError(m, r)
        return read(r)
    }

    // The kinds of errors, as they cross both ways: the kind, then the
    // code, domain, number and message.
    private const val ERR_OTHER = 0
    private const val ERR_CODE = 1
    private const val ERR_UNSUPPORTED = 2
    private const val ERR_CLOSED = 3
    private const val ERR_CANCELED = 4
    private const val ERR_GO = 5

    fun writeError(w: NativeWriter, e: Throwable) {
        var kind = ERR_OTHER
        var code = ""
        var domain = ""
        var number = 0L
        var message = e.message ?: e.toString()
        when (e) {
            is NativeException -> { domain = e.domain; number = e.number }
            is NativeCodedError -> { kind = ERR_CODE; code = e.nativeCode }
            is NativeUnsupportedException -> kind = ERR_UNSUPPORTED
            is NativeClosedException -> kind = ERR_CLOSED
            is NativeGoException -> kind = ERR_GO
            is CancellationException -> kind = ERR_CANCELED
            else -> domain = e.javaClass.name
        }
        w.u8(kind)
        w.string(code)
        w.string(domain)
        w.i64(number)
        w.string(message)
    }

    private fun readError(m: NativeModule, r: NativeReader): Throwable = try {
        val kind = r.u8()
        val code = r.string()
        r.string()
        r.i64()
        val message = r.string()
        when (kind) {
            ERR_CODE -> m.error(code) ?: NativeGoException(message)
            ERR_UNSUPPORTED -> NativeUnsupportedException(message)
            ERR_CLOSED -> NativeClosedException(message)
            ERR_CANCELED -> CancellationException(message)
            else -> NativeGoException(message)
        }
    } catch (e: NativeWireException) {
        NativeGoException("an error of Go's that does not decode")
    }

    // Values of Go's that Kotlin holds, released once it no longer does.

    private val collected = ReferenceQueue<Any>()
    private val held = ConcurrentHashMap.newKeySet<Held>()

    private class Held(holder: Any, val id: Long) : PhantomReference<Any>(holder, collected)

    internal fun releaseWith(holder: Any, id: Long) {
        held.add(Held(holder, id))
        releaser
    }

    private val releaser: Thread by lazy {
        Thread({
            while (true) {
                val r = collected.remove() as Held
                held.remove(r)
                send(NativeModule.KIND_RELEASE, r.id, 0, null)
            }
        }, "mygo-native-release").apply {
            isDaemon = true
            start()
        }
    }

    /** The tasks of asynchronous calls and streams, by their call, which Go cancels. */
    internal class Tasks {
        private val jobs = HashMap<Long, Job>()
        private val done = HashSet<Long>()
        private val canceled = HashSet<Long>()

        @Synchronized fun put(call: Long, job: Job) {
            if (done.remove(call)) return
            if (canceled.remove(call)) job.cancel()
            jobs[call] = job
        }

        @Synchronized fun finish(call: Long) {
            if (jobs.remove(call) == null) done.add(call)
            canceled.remove(call)
        }

        fun cancel(call: Long) {
            val job = synchronized(this) {
                jobs.remove(call).also { if (it == null && !done.remove(call)) canceled.add(call) }
            }
            job?.cancel()
        }
    }
}
