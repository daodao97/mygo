package dev.mygo

import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.graphics.Rect
import android.graphics.Typeface
import android.os.Build
import java.io.File
import java.nio.ByteBuffer
import kotlin.math.ceil
import kotlin.math.floor

/**
 * Text of native UI, measured and drawn as a TextView measures and draws
 * it: Minikin lays it out, Skia draws it. Go breaks lines itself and asks
 * how wide words are (getTextRunAdvances) and what a cluster looks like
 * (drawText).
 */
object Text {
    private val registered = HashMap<String, Typeface>()
    private val faces = HashMap<String, Typeface>()
    private val paint = Paint(Paint.ANTI_ALIAS_FLAG or Paint.SUBPIXEL_TEXT_FLAG)
    private val bounds = Rect()
    private var bitmap: Bitmap? = null

    /** The typeface of a CSS-like list of families, at a weight. */
    private fun typeface(families: String, weight: Long, italic: Boolean): Typeface {
        val key = "$families|$weight|$italic"
        faces[key]?.let { return it }
        var base: Typeface? = null
        for (raw in families.split(',')) {
            val f = raw.trim().trim('"', '\'')
            base = registered[f] ?: when (f.lowercase()) {
                "", "system-ui", "sans-serif", "ui-sans-serif", "-apple-system" -> Typeface.SANS_SERIF
                "serif", "ui-serif" -> Typeface.SERIF
                "monospace", "ui-monospace" -> Typeface.MONOSPACE
                else -> Typeface.create(f, Typeface.NORMAL).takeIf { it != Typeface.DEFAULT }
            }
            if (base != null) break
        }
        val face = if (Build.VERSION.SDK_INT >= 28) {
            Typeface.create(base ?: Typeface.SANS_SERIF, weight.toInt().coerceIn(1, 1000), italic)
        } else {
            Typeface.create(base ?: Typeface.SANS_SERIF, (if (weight >= 600) Typeface.BOLD else 0) or (if (italic) Typeface.ITALIC else 0))
        }
        if (faces.size > 256) faces.clear()
        faces[key] = face
        return face
    }

    private fun set(family: String, size: Double, weight: Long, italic: Boolean, noKern: Boolean = false): Paint {
        paint.typeface = typeface(family, weight, italic)
        paint.textSize = size.toFloat()
        // Go draws clusters alone, which ligatures would join: "fl" would
        // take the advance of both, and draw as an f before a gap.
        paint.fontFeatureSettings = if (noKern) "'liga' 0, 'clig' 0, 'kern' 0" else "'liga' 0, 'clig' 0"
        paint.color = Color.WHITE
        return paint
    }

    @Synchronized
    fun metrics(c: TextMetricsCall): TextMetricsReply {
        val p = set(c.family, c.size, c.weight, c.italic)
        val m = p.fontMetrics
        val r = TextMetricsReply()
        r.ascent = -m.ascent.toDouble()
        r.descent = m.descent.toDouble()
        p.getTextBounds("x", 0, 1, bounds)
        r.xHeight = -bounds.top.toDouble()
        if (Build.VERSION.SDK_INT >= 29) {
            r.underlinePosition = p.underlinePosition.toDouble()
            r.underlineThickness = p.underlineThickness.toDouble()
            r.strikePosition = -p.strikeThruPosition.toDouble()
        } else {
            r.underlinePosition = c.size / 10
            r.underlineThickness = maxOf(c.size / 14, 1.0)
            r.strikePosition = r.xHeight / 2
        }
        return r
    }

    @Synchronized
    fun measure(c: MeasureTextCall): MeasureTextReply {
        val p = set(c.family, c.size, c.weight, c.italic, c.noKern)
        val r = MeasureTextReply()
        r.widths = DoubleArray(c.texts.size) { i ->
            val t = c.texts[i]
            p.getRunAdvance(t, 0, t.length, 0, t.length, false, t.length).toDouble()
        }
        return r
    }

    @Synchronized
    fun advances(c: TextAdvancesCall): TextAdvancesReply {
        val p = set(c.family, c.size, c.weight, c.italic, c.noKern)
        val chars = c.text.toCharArray()
        val out = FloatArray(chars.size)
        p.getTextRunAdvances(chars, 0, chars.size, 0, chars.size, c.rtl, out, 0)
        val r = TextAdvancesReply()
        r.advances = DoubleArray(out.size) { out[it].toDouble() }
        return r
    }

    @Synchronized
    fun draw(c: DrawTextCall): DrawTextReply {
        val r = DrawTextReply()
        if (c.text.isBlank()) return r
        // The font at the size text was measured at, the canvas scaled, as
        // the measurements were made.
        val p = set(c.family, c.size, c.weight, c.italic)
        p.getTextBounds(c.text, 0, c.text.length, bounds)
        val s = c.scale
        val pad = 2
        val left = floor(bounds.left * s + c.dx).toInt() - pad
        val right = ceil(bounds.right * s + c.dx).toInt() + pad
        val top = floor(bounds.top * s).toInt() - pad
        val bottom = ceil(bounds.bottom * s).toInt() + pad
        val w = right - left
        val h = bottom - top
        if (w <= 2 * pad || h <= 2 * pad || w > 2048 || h > 2048) return r
        var bm = bitmap
        if (bm == null || bm.width < w || bm.height < h) {
            bm?.recycle()
            bm = Bitmap.createBitmap(maxOf(w, bm?.width ?: 0, 64), maxOf(h, bm?.height ?: 0, 64), Bitmap.Config.ARGB_8888)
            bitmap = bm
        }
        bm!!.eraseColor(Color.TRANSPARENT)
        val canvas = Canvas(bm)
        canvas.translate((c.dx - left).toFloat(), (-top).toFloat())
        canvas.scale(s.toFloat(), s.toFloat())
        canvas.drawText(c.text, 0f, 0f, p)
        val buf = ByteBuffer.allocate(bm.byteCount)
        bm.copyPixelsToBuffer(buf)
        val px = buf.array()
        val stride = bm.width * 4
        // White text is as covered as it is opaque; other colors are an
        // emoji's.
        var color = false
        loop@ for (y in 0 until h) for (x in 0 until w) {
            val i = y * stride + x * 4
            val a = px[i + 3].toInt() and 0xff
            if (a > 32) {
                val far = { v: Int -> v < a - 24 }
                if (far(px[i].toInt() and 0xff) || far(px[i + 1].toInt() and 0xff) || far(px[i + 2].toInt() and 0xff)) {
                    color = true
                    break@loop
                }
            }
        }
        r.left = left.toLong()
        r.top = top.toLong()
        r.width = w.toLong()
        r.height = h.toLong()
        r.color = color
        r.pixels = if (color) {
            ByteArray(w * h * 4).also { out -> for (y in 0 until h) System.arraycopy(px, y * stride, out, y * w * 4, w * 4) }
        } else {
            ByteArray(w * h) { k -> px[(k / w) * stride + (k % w) * 4 + 3] }
        }
        return r
    }

    @Synchronized
    fun register(c: RegisterFontCall): RegisterFontReply {
        val r = RegisterFontReply()
        try {
            val f = File.createTempFile("font", ".ttf", HostImpl.app.cacheDir)
            f.writeBytes(c.data)
            val face = Typeface.createFromFile(f)
            f.delete()
            val family = c.family.ifEmpty { "font${registered.size + 1}" }
            registered[family] = face
            faces.clear()
            r.family = family
        } catch (e: Exception) {
            r.error = "mygo: ${e.message}"
        }
        return r
    }
}
