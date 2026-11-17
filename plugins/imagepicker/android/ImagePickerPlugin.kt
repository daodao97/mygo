package dev.mygo

import android.app.Activity
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.provider.MediaStore
import androidx.core.content.FileProvider
import org.json.JSONArray
import org.json.JSONObject
import java.io.File

class ImagePickerPlugin : MyGoPlugin {
    override fun call(method: String, args: JSONObject, reply: PluginReply) {
        when (method) {
            "pick" -> pick(args, reply)
            "capture" -> capture(reply)
            else -> reply.failure(PluginFailure("NoMethod", method))
        }
    }

    private fun pick(args: JSONObject, reply: PluginReply) {
        val type = when (args.optString("kind")) {
            "videos" -> "video/*"
            "any" -> "*/*"
            else -> "image/*"
        }
        val limit = args.optInt("limit", 1)
        // The Photo Picker, where the system has it; Google Play brings it to
        // older ones.
        val photo = Build.VERSION.SDK_INT >= 33 || MediaStore.getPickImagesMaxLimit() > 0
        val i = if (photo) {
            Intent(MediaStore.ACTION_PICK_IMAGES).apply {
                if (type != "*/*") setType(type)
                if (limit != 1) putExtra(MediaStore.EXTRA_PICK_IMAGES_MAX, if (limit < 0) MediaStore.getPickImagesMaxLimit() else limit)
            }
        } else {
            Intent(Intent.ACTION_GET_CONTENT).setType(type).addCategory(Intent.CATEGORY_OPENABLE).putExtra(Intent.EXTRA_ALLOW_MULTIPLE, limit != 1)
        }
        PluginHost.startForResult(i) { code, data ->
            if (code != Activity.RESULT_OK || data == null) {
                reply.success(JSONObject().put("paths", JSONArray()))
                return@startForResult
            }
            val uris = ArrayList<Uri>()
            data.clipData?.let { c -> for (k in 0 until c.itemCount) uris.add(c.getItemAt(k).uri) }
            if (uris.isEmpty()) data.data?.let { uris.add(it) }
            Thread {
                try {
                    val dir = File(PluginHost.context.cacheDir, "picked/${System.nanoTime()}").apply { mkdirs() }
                    val out = JSONArray()
                    for (u in uris) out.put(HostImpl.copyToFile(u, dir).absolutePath)
                    reply.success(JSONObject().put("paths", out))
                } catch (e: Exception) {
                    reply.failure(PluginFailure("Failed", e.toString()))
                }
            }.start()
        }
    }

    private fun capture(reply: PluginReply) {
        val ctx = PluginHost.context
        val dir = File(ctx.cacheDir, "camera").apply { mkdirs() }
        val f = File(dir, "${System.nanoTime()}.jpg")
        val uri = FileProvider.getUriForFile(ctx, ctx.packageName + ".mygo.files", f)
        val i = Intent(MediaStore.ACTION_IMAGE_CAPTURE).putExtra(MediaStore.EXTRA_OUTPUT, uri).addFlags(Intent.FLAG_GRANT_WRITE_URI_PERMISSION)
        if (i.resolveActivity(ctx.packageManager) == null) {
            reply.failure(PluginFailure("NoCamera", "the device has no camera app"))
            return
        }
        PluginHost.startForResult(i) { code, _ ->
            reply.success(JSONObject().put("path", if (code == Activity.RESULT_OK && f.length() > 0) f.absolutePath else ""))
        }
    }
}
