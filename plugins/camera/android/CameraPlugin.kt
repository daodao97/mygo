package dev.mygo

import android.Manifest
import android.content.Context
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageCapture
import androidx.camera.core.ImageCaptureException
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.core.content.ContextCompat
import androidx.lifecycle.LifecycleOwner
import org.json.JSONObject
import java.io.File

class CameraPlugin : MyGoPlugin {
    private class Cam(val view: PreviewView) {
        val capture = ImageCapture.Builder().build()
        var bound = false
        var error = "the camera is not running"
    }

    private val cams = HashMap<Long, Cam>()

    init {
        PluginHost.registerView("camera") { context, id, args ->
            val v = PreviewView(context)
            // A texture, for it to show under the surface that draws over it.
            v.implementationMode = PreviewView.ImplementationMode.COMPATIBLE
            val c = Cam(v)
            cams[id] = c
            start(c, args.optBoolean("front"))
            v
        }
    }

    private fun start(c: Cam, front: Boolean) {
        PluginHost.requestPermissions(arrayOf(Manifest.permission.CAMERA)) { granted ->
            if (!granted) {
                c.error = "the app may not use the camera"
                return@requestPermissions
            }
            val ctx = PluginHost.context
            val future = ProcessCameraProvider.getInstance(ctx)
            future.addListener({
                val owner = PluginHost.activity as? LifecycleOwner
                if (owner == null) {
                    c.error = "no window to show the camera in"
                    return@addListener
                }
                val preview = Preview.Builder().build().also { it.surfaceProvider = c.view.surfaceProvider }
                val which = if (front) CameraSelector.DEFAULT_FRONT_CAMERA else CameraSelector.DEFAULT_BACK_CAMERA
                runCatching {
                    future.get().bindToLifecycle(owner, which, preview, c.capture)
                    c.bound = true
                }.onFailure {
                    c.error = it.toString()
                    android.util.Log.e("mygo", "camera: $it")
                }
            }, ContextCompat.getMainExecutor(ctx))
        }
    }

    override fun call(method: String, args: JSONObject, reply: PluginReply) {
        val id = args.optLong("id")
        val c = cams[id]
        if (c == null) {
            reply.failure(PluginFailure("NoView"))
            return
        }
        when (method) {
            "takePhoto" -> {
                if (!c.bound) {
                    reply.failure(PluginFailure("NoCamera", c.error))
                    return
                }
                val ctx = PluginHost.context
                val f = File(ctx.cacheDir, "${System.nanoTime()}.jpg")
                c.capture.takePicture(ImageCapture.OutputFileOptions.Builder(f).build(), ContextCompat.getMainExecutor(ctx),
                    object : ImageCapture.OnImageSavedCallback {
                        override fun onImageSaved(r: ImageCapture.OutputFileResults) = reply.success(JSONObject().put("path", f.absolutePath))
                        override fun onError(e: ImageCaptureException) = reply.failure(PluginFailure("Failed", e.toString()))
                    })
            }
            "dispose" -> {
                cams.remove(id)
                runCatching { ProcessCameraProvider.getInstance(PluginHost.context).get().unbindAll() }
                reply.success(null)
            }
            else -> reply.failure(PluginFailure("NoMethod", method))
        }
    }
}
