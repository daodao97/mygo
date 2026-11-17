package dev.mygo

import android.Manifest
import android.app.Activity
import android.app.AlertDialog
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.BitmapFactory
import android.os.Handler
import android.os.Looper
import android.provider.MediaStore
import androidx.core.content.FileProvider
import com.google.zxing.BarcodeFormat
import com.google.zxing.BinaryBitmap
import com.google.zxing.DecodeHintType
import com.google.zxing.MultiFormatReader
import com.google.zxing.NotFoundException
import com.google.zxing.RGBLuminanceSource
import com.google.zxing.common.HybridBinarizer
import org.json.JSONObject
import java.io.File
import java.io.InputStream
import java.util.Locale

/** A QR code from a photo the camera takes or the library holds, decoded with ZXing. */
class ScannerPlugin : MyGoPlugin {
    private var scanning = 0L
    private var pending: PluginReply? = null
    private var dialog: AlertDialog? = null
    private val main = Handler(Looper.getMainLooper())

    override fun call(method: String, args: JSONObject, reply: PluginReply) {
        val id = args.optLong("id")
        when (method) {
            "scan" -> scan(id, args, reply)
            "cancel" -> {
                if (scanning == id) {
                    dialog?.dismiss()
                    finish(id, null, "Canceled")
                }
                reply.success(null)
            }
            else -> reply.failure(PluginFailure("NoMethod", method))
        }
    }

    /** Answers scan id once, on the UI thread; later answers of it do nothing. */
    private fun finish(id: Long, value: String?, code: String?, message: String = "") {
        if (scanning != id) return
        val r = pending ?: return
        pending = null
        scanning = 0
        dialog = null
        if (code == null) r.success(JSONObject().put("value", value ?: "")) else r.failure(PluginFailure(code, message))
    }

    private fun scan(id: Long, args: JSONObject, reply: PluginReply) {
        val a = PluginHost.activity
        if (a == null || a.isFinishing) {
            reply.failure(PluginFailure("Unavailable", "the app is not in front"))
            return
        }
        if (pending != null) {
            reply.failure(PluginFailure("Busy", "a scan runs"))
            return
        }
        pending = reply
        scanning = id
        val zh = Locale.getDefault().language == "zh"
        val title = args.optString("prompt").ifBlank { if (zh) "扫描二维码" else "Scan a QR code" }
        val cancel = args.optString("cancelLabel").ifBlank { a.getString(android.R.string.cancel) }
        val camera = Intent(MediaStore.ACTION_IMAGE_CAPTURE).resolveActivity(a.packageManager) != null
        val takePhoto = if (zh) "拍照" else "Take a photo"
        val fromLibrary = if (zh) "从相册选择" else "Choose from library"
        val choices = if (camera) arrayOf(takePhoto, fromLibrary) else arrayOf(fromLibrary)
        dialog = AlertDialog.Builder(a).setTitle(title)
            .setItems(choices) { _, which ->
                dialog = null
                if (camera && which == 0) takePhoto(id) else pick(id)
            }
            .setNegativeButton(cancel) { _, _ -> finish(id, null, "Canceled") }
            .setOnCancelListener { finish(id, null, "Canceled") }
            .show()
    }

    private fun takePhoto(id: Long) {
        val ctx = PluginHost.context
        // An app that declares the camera's permission needs it granted for
        // the system's camera too.
        val declared = runCatching {
            ctx.packageManager.getPackageInfo(ctx.packageName, PackageManager.GET_PERMISSIONS).requestedPermissions?.contains(Manifest.permission.CAMERA) == true
        }.getOrDefault(false)
        if (!declared) {
            capture(id)
            return
        }
        PluginHost.requestPermissions(arrayOf(Manifest.permission.CAMERA)) { granted ->
            if (granted) capture(id) else finish(id, null, "Denied", "camera access is not allowed")
        }
    }

    private fun capture(id: Long) {
        val ctx = PluginHost.context
        val f = File(File(ctx.cacheDir, "scan").apply { mkdirs() }, "${System.nanoTime()}.jpg")
        try {
            val uri = FileProvider.getUriForFile(ctx, ctx.packageName + ".mygo.files", f)
            val i = Intent(MediaStore.ACTION_IMAGE_CAPTURE).putExtra(MediaStore.EXTRA_OUTPUT, uri).addFlags(Intent.FLAG_GRANT_WRITE_URI_PERMISSION)
            PluginHost.startForResult(i) { code, _ ->
                if (code == Activity.RESULT_OK && f.length() > 0) decode(id, f) { f.inputStream() }
                else { f.delete(); finish(id, null, "Canceled") }
            }
        } catch (e: Exception) {
            finish(id, null, "Unavailable", e.toString())
        }
    }

    private fun pick(id: Long) {
        val i = Intent(Intent.ACTION_GET_CONTENT).setType("image/*").addCategory(Intent.CATEGORY_OPENABLE)
        try {
            PluginHost.startForResult(i) { code, data ->
                val uri = data?.data
                if (code == Activity.RESULT_OK && uri != null) decode(id, null) { PluginHost.context.contentResolver.openInputStream(uri)!! }
                else finish(id, null, "Canceled")
            }
        } catch (e: Exception) {
            finish(id, null, "Unavailable", e.toString())
        }
    }

    private fun decode(id: Long, file: File?, open: () -> InputStream) {
        Thread {
            val value = try { read(open) } catch (_: Exception) { null }
            file?.delete()
            main.post {
                if (value != null) finish(id, value, null)
                else finish(id, null, "NotFound", "no QR code was found in the picture")
            }
        }.start()
    }

    private fun read(open: () -> InputStream): String? {
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        open().use { BitmapFactory.decodeStream(it, null, bounds) }
        var sample = 1
        while (maxOf(bounds.outWidth, bounds.outHeight) / sample > 1600) sample *= 2
        val bitmap = open().use { BitmapFactory.decodeStream(it, null, BitmapFactory.Options().apply { inSampleSize = sample }) } ?: return null
        val pixels = IntArray(bitmap.width * bitmap.height)
        bitmap.getPixels(pixels, 0, bitmap.width, 0, 0, bitmap.width, bitmap.height)
        val source = RGBLuminanceSource(bitmap.width, bitmap.height, pixels)
        bitmap.recycle()
        val hints = mapOf(DecodeHintType.POSSIBLE_FORMATS to listOf(BarcodeFormat.QR_CODE), DecodeHintType.TRY_HARDER to true)
        return try {
            MultiFormatReader().decode(BinaryBitmap(HybridBinarizer(source)), hints).text
        } catch (_: NotFoundException) {
            null
        }
    }
}
