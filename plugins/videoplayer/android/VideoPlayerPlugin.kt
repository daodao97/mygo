package dev.mygo

import android.content.Context
import android.graphics.SurfaceTexture
import android.media.MediaPlayer
import android.media.PlaybackParams
import android.net.Uri
import android.os.Handler
import android.os.Looper
import android.view.Surface
import android.view.TextureView
import org.json.JSONObject

private class PlayerView(context: Context, val id: Long, args: JSONObject) : TextureView(context), TextureView.SurfaceTextureListener {
    val player = MediaPlayer()
    var ready = false
    var ended = false
    var error = ""
    private val auto = args.optBoolean("autoPlay")
    private val ticks = Handler(Looper.getMainLooper())
    private val tick = object : Runnable {
        override fun run() {
            if (ready && player.isPlaying) send()
            ticks.postDelayed(this, 250)
        }
    }

    init {
        surfaceTextureListener = this
        player.isLooping = args.optBoolean("loop")
        if (args.optBoolean("muted")) player.setVolume(0f, 0f)
        val source = args.optString("source")
        try {
            if (source.contains("://")) player.setDataSource(context, Uri.parse(source)) else player.setDataSource(source)
        } catch (e: Exception) {
            error = e.toString()
        }
        player.setOnPreparedListener {
            ready = true
            if (auto) it.start()
            send()
        }
        player.setOnCompletionListener {
            ended = true
            send()
        }
        player.setOnVideoSizeChangedListener { _, _, _ ->
            fit()
            send()
        }
        addOnLayoutChangeListener { _, _, _, _, _, _, _, _, _ -> fit() }
        player.setOnErrorListener { _, what, extra ->
            error = "MediaPlayer error $what/$extra"
            send()
            true
        }
        ticks.post(tick)
    }

    /** Shows the video whole in the view, keeping its shape: a texture fills the view. */
    private fun fit() {
        val vw = player.videoWidth.toFloat()
        val vh = player.videoHeight.toFloat()
        if (vw <= 0 || vh <= 0 || width == 0 || height == 0) return
        val k = minOf(width / vw, height / vh)
        val sx = vw * k / width
        val sy = vh * k / height
        val m = android.graphics.Matrix()
        m.setScale(sx, sy, width / 2f, height / 2f)
        setTransform(m)
    }

    fun send() {
        PluginHost.emit("videoplayer", "state", JSONObject().put("id", id).put("ready", ready).put("ended", ended)
            .put("playing", ready && player.isPlaying)
            .put("positionMs", if (ready) player.currentPosition else 0).put("durationMs", if (ready) player.duration else 0)
            .put("width", player.videoWidth).put("height", player.videoHeight).put("error", error))
    }

    fun dispose() {
        ticks.removeCallbacks(tick)
        player.release()
    }

    override fun onSurfaceTextureAvailable(t: SurfaceTexture, w: Int, h: Int) {
        player.setSurface(Surface(t))
        if (error.isEmpty() && !ready) runCatching { player.prepareAsync() }.onFailure { error = it.toString(); send() }
    }

    override fun onSurfaceTextureSizeChanged(t: SurfaceTexture, w: Int, h: Int) {}
    override fun onSurfaceTextureDestroyed(t: SurfaceTexture) = true
    override fun onSurfaceTextureUpdated(t: SurfaceTexture) {}
}

class VideoPlayerPlugin : MyGoPlugin {
    private val views = HashMap<Long, PlayerView>()

    init {
        PluginHost.registerView("videoplayer") { context, id, args ->
            PlayerView(context, id, args).also { views[id] = it }
        }
    }

    override fun call(method: String, args: JSONObject, reply: PluginReply) {
        val v = views[args.optLong("id")]
        if (v == null) {
            reply.failure(PluginFailure("NoView"))
            return
        }
        when (method) {
            "play" -> {
                v.ended = false
                if (v.ready) v.player.start()
                v.send()
            }
            "pause" -> {
                if (v.ready) v.player.pause()
                v.send()
            }
            "seek" -> {
                v.ended = false
                if (v.ready) v.player.seekTo(args.optLong("ms"), MediaPlayer.SEEK_CLOSEST)
            }
            "volume" -> v.player.setVolume(args.optDouble("volume", 1.0).toFloat(), args.optDouble("volume", 1.0).toFloat())
            "rate" -> if (v.ready) v.player.playbackParams = PlaybackParams().setSpeed(args.optDouble("rate", 1.0).toFloat())
            "dispose" -> {
                v.dispose()
                views.remove(args.optLong("id"))
            }
            else -> {
                reply.failure(PluginFailure("NoMethod", method))
                return
            }
        }
        reply.success(null)
    }
}
