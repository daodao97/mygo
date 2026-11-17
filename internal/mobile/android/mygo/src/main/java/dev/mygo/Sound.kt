package dev.mygo

import android.media.AudioAttributes
import android.media.AudioFormat
import android.media.AudioTrack
import java.nio.ByteBuffer
import java.nio.ByteOrder

/**
 * The sound of package audio, through AudioTrack: Go's output writes from a
 * thread of its own, which AudioTrack makes wait while it holds enough.
 */
object Sound {
    private var track: AudioTrack? = null

    fun open(c: AudioOpenCall): AudioOpenReply {
        val r = AudioOpenReply()
        try {
            val format = AudioFormat.Builder().setSampleRate(c.rate.toInt()).setChannelMask(AudioFormat.CHANNEL_OUT_STEREO)
                .setEncoding(AudioFormat.ENCODING_PCM_FLOAT).build()
            val least = AudioTrack.getMinBufferSize(c.rate.toInt(), AudioFormat.CHANNEL_OUT_STEREO, AudioFormat.ENCODING_PCM_FLOAT)
            val asked = (c.rate * c.ahead / 1000 * 8).toInt()
            val t = AudioTrack.Builder()
                .setAudioAttributes(AudioAttributes.Builder().setUsage(AudioAttributes.USAGE_MEDIA).setContentType(AudioAttributes.CONTENT_TYPE_MUSIC).build())
                .setAudioFormat(format).setBufferSizeInBytes(maxOf(least, asked)).setTransferMode(AudioTrack.MODE_STREAM)
                .setPerformanceMode(AudioTrack.PERFORMANCE_MODE_LOW_LATENCY).build()
            t.play()
            track = t
        } catch (e: Exception) {
            r.error = "audio: $e"
        }
        return r
    }

    fun write(c: AudioWriteCall): AudioWriteReply {
        val r = AudioWriteReply()
        val t = track
        if (t == null) {
            r.error = "audio: closed"
            return r
        }
        val b = ByteBuffer.wrap(c.samples).order(ByteOrder.LITTLE_ENDIAN).asFloatBuffer()
        val a = FloatArray(b.remaining())
        b.get(a)
        val n = t.write(a, 0, a.size, AudioTrack.WRITE_BLOCKING)
        if (n < 0) r.error = "audio: AudioTrack error $n"
        return r
    }

    fun close() {
        track?.run {
            stop()
            release()
        }
        track = null
    }
}
