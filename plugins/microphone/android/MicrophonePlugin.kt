package dev.mygo

import android.Manifest
import android.annotation.SuppressLint
import android.media.AudioFormat
import android.media.AudioRecord
import android.media.MediaRecorder
import android.os.Handler
import android.os.Looper
import android.util.Base64
import org.json.JSONObject
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicReference

/** 16 kHz mono signed 16-bit PCM from AudioRecord. No recorder call waits on the UI thread. */
class MicrophonePlugin : MyGoPlugin {
    private class Capture(val token: Long) {
        val mode = AtomicInteger(0) // 0 recording, 1 finishing, 2 dropped
        val lock = Any()
        var recorder: AudioRecord? = null
    }

    private val reader = Executors.newSingleThreadExecutor()
    private val control = Executors.newSingleThreadExecutor()
    private val active = AtomicReference<Capture?>(null)
    private val main = Handler(Looper.getMainLooper())

    override fun call(method: String, args: JSONObject, reply: PluginReply) {
        val token = args.optLong("token")
        when (method) {
            "start" -> PluginHost.requestPermissions(arrayOf(Manifest.permission.RECORD_AUDIO)) { granted ->
                if (granted) start(token, reply)
                else reply.failure(PluginFailure("Denied", "microphone access is not allowed"))
            }
            "stop" -> {
                stop(token, args.optBoolean("cancel"))
                reply.success(null)
            }
            else -> reply.failure(PluginFailure("NoMethod", method))
        }
    }

    @SuppressLint("MissingPermission")
    private fun start(token: Long, reply: PluginReply) {
        val capture = Capture(token)
        if (!active.compareAndSet(null, capture)) {
            reply.failure(PluginFailure("Busy", "a capture runs"))
            return
        }
        reader.execute {
            var failure: String? = null
            var started = false
            try {
                val minimum = AudioRecord.getMinBufferSize(16000, AudioFormat.CHANNEL_IN_MONO, AudioFormat.ENCODING_PCM_16BIT)
                check(minimum > 0) { "Microphone format unavailable" }
                val recorder = AudioRecord.Builder().setAudioSource(MediaRecorder.AudioSource.VOICE_RECOGNITION)
                    .setAudioFormat(AudioFormat.Builder().setSampleRate(16000).setChannelMask(AudioFormat.CHANNEL_IN_MONO).setEncoding(AudioFormat.ENCODING_PCM_16BIT).build())
                    .setBufferSizeInBytes(maxOf(minimum, 6400)).build()
                synchronized(capture.lock) { capture.recorder = recorder }
                check(recorder.state == AudioRecord.STATE_INITIALIZED) { "Cannot initialize microphone" }
                if (capture.mode.get() == 0) recorder.startRecording()
                started = true
                main.post { reply.success(null) }
                val buffer = ByteArray(3200) // 100 ms, little-endian PCM16
                while (capture.mode.get() == 0) {
                    val n = recorder.read(buffer, 0, buffer.size, AudioRecord.READ_BLOCKING)
                    if (n > 0 && capture.mode.get() != 2) {
                        PluginHost.emit("microphone", "audio", JSONObject().put("token", token).put("pcm", Base64.encodeToString(buffer, 0, n, Base64.NO_WRAP)))
                    } else if (n < 0 && capture.mode.get() == 0) {
                        throw IllegalStateException("Microphone read failed: $n")
                    }
                }
            } catch (e: Exception) {
                if (capture.mode.get() == 0) failure = e.message ?: "Audio capture failed"
                if (!started) {
                    val message = failure ?: "capture canceled"
                    main.post { reply.failure(PluginFailure("Failed", message)) }
                }
            } finally {
                synchronized(capture.lock) {
                    capture.recorder?.let { r ->
                        try { r.stop() } catch (_: Exception) {}
                        r.release()
                    }
                    capture.recorder = null
                }
                active.compareAndSet(capture, null)
                if (started && capture.mode.get() != 2) {
                    PluginHost.emit("microphone", "audio", JSONObject().put("token", token).put("end", true).put("error", failure ?: ""))
                }
            }
        }
    }

    private fun stop(token: Long, cancel: Boolean) {
        val capture = active.get() ?: return
        if (capture.token != token) return
        if (cancel) capture.mode.set(2) else capture.mode.compareAndSet(0, 1)
        // From a worker of its own, to release a blocking read even as the
        // input device goes; release is the reader's.
        control.execute { synchronized(capture.lock) { try { capture.recorder?.stop() } catch (_: Exception) {} } }
    }
}
