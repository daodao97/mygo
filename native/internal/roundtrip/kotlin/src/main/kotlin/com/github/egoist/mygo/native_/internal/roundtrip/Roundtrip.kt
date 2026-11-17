package com.github.egoist.mygo.native_.internal.roundtrip

import android.graphics.RectF
import dev.mygo.NativeBuffer
import dev.mygo.NativeCloser
import dev.mygo.NativeException
import dev.mygo.NativeUnsupportedException
import java.time.Instant
import java.util.Timer
import java.util.concurrent.atomic.AtomicLong
import kotlin.concurrent.fixedRateTimer
import kotlin.time.Duration
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow

// The implementation of package roundtrip on Android: every value it is
// given, back, as the Swift, Rust and Zig ones do.

fun makeEcho(): Echo = Echoer()

private class Echoer : Echo {
    override fun scalars(i: Long, i8: Byte, u16: UShort, i32: Int, u64: ULong, f32: Float, f: Double, b: Boolean): Scalars =
        Scalars(i + 1, (i8 + 1).toByte(), (u16 + 1u).toUShort(), i32 + 1, u64 + 1u, f32 + 1, f + 1, !b)

    override fun add(a: Long, b: Long): Long = a + b

    override fun text(s: String, b: ByteArray): Texts = Texts(s + s, b.reversedArray())

    override fun record(r: Record): Record = r.copy(counts = r.counts.map { it * 2 })

    override fun lists(words: List<String>, nested: List<List<Long>>): Map<String, Long> {
        val m = HashMap<String, Long>()
        for (w in words) m[w] = w.toByteArray(Charsets.UTF_8).size.toLong()
        m["nested"] = nested.sumOf { it.sum() }
        return m
    }

    override fun maybe(p: Long?): Long? = p?.plus(1)

    override fun times(t: Instant, d: Duration): Moment = Moment(t.plusNanos(d.inWholeNanoseconds), d * 2)

    override fun kind(k: Kind, l: Level): Kinds {
        val all = Kind.entries
        return Kinds(all[(all.indexOf(k) + 1) % all.size], if (l == Level.LOW) Level.HIGH else Level.LOW)
    }

    override fun fail(name: String) {
        when (name) {
            "" -> return
            "named" -> throw RoundtripException(RoundtripError.NAMED)
            "unsupported" -> throw NativeUnsupportedException()
            else -> throw NativeException("RoundtripDomain", 7, name)
        }
    }

    override suspend fun wait(d: Duration, s: String): String {
        delay(d)
        return s
    }

    override fun count(n: Long, fail: Long): Flow<Long> = flow {
        for (i in 0 until n) {
            if (i == fail) throw RoundtripException(RoundtripError.NAMED)
            emit(i)
        }
    }

    override fun records(n: Long): Flow<Record> = flow {
        for (i in 0 until n) {
            emit(Record("r$i", listOf(i), mapOf("i" to "$i"), Inner(i.toDouble(), 0.0), null, Kind.KIND_A, byteArrayOf(i.toByte())))
        }
    }

    override fun counter(n: Long): Counter = Counting(n)

    override fun each(words: List<String>, f: (String, Long) -> Unit): Long {
        words.forEachIndexed { i, w -> f(w, i.toLong()) }
        return words.size.toLong()
    }

    override fun ask(ns: List<Long>, f: (Long) -> Long): Long = ns.sumOf { f(it) }

    override fun ticks(interval: Duration, f: (Long) -> Unit): AutoCloseable {
        var n = 0L
        val period = maxOf(interval.inWholeMilliseconds, 1)
        val timer: Timer = fixedRateTimer("roundtrip-ticks", daemon = true, initialDelay = period, period = period) {
            n++
            f(n)
        }
        return NativeCloser { timer.cancel() }
    }

    override suspend fun use(s: Scorer, words: List<String>): Double {
        var sum = words.sumOf { s.score(it) }
        words.firstOrNull()?.let { sum += s.explain(it).toByteArray(Charsets.UTF_8).size }
        return sum
    }

    override fun fill(buf: NativeBuffer, x: Long): Long {
        val b = buf.bytes
        for (i in 0 until buf.size) b.put(i, (i.toLong() xor x).toByte())
        return buf.size.toLong()
    }

    override fun mix(f: Float, d: Double, b: Boolean, i: Byte): Double = f + d + (if (b) 1 else 0) + i

    override fun box(r: RectF): RectF = RectF(r).apply { offset(r.width(), r.height()) }

    override fun nothing(e: List<Empty>): Long = e.size.toLong()

    override fun onMain(r: Record, f: (Inner) -> Unit): Texts {
        f(r.inner)
        return Texts(r.name, r.data)
    }
}

private class Counting(start: Long) : Counter {
    private val n = AtomicLong(start)

    override fun next(): Long = n.getAndIncrement()

    override fun same(): Counter = Counting(n.get())

    override fun close() {}
}
