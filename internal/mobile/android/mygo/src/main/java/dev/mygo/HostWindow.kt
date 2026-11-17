package dev.mygo

import android.content.Context
import android.view.View

/**
 * A window of the app: a view filling the screen, which shows a page or
 * native UI.
 */
abstract class HostWindow(val context: Context, val id: Long) {
    abstract val view: View
    private var last: DoubleArray? = null
    /** The color the window's content fills it with, 0 while unknown. */
    var fill = 0
    /** The surface showing this window under it, as a web view in native UI. */
    var holder: SurfaceWindow? = null

    /**
     * What covers the window, in pixels: the system's bars and the cutout
     * where they cover it, wherever it is on the screen (under a toolbar,
     * above a bar of tabs), and the keyboard: top, left, bottom, right,
     * keyboard.
     */
    fun covered(): IntArray {
        val root = HostImpl.root
        // Where layout puts it, without the translation of a page sliding
        // in, which ends where layout put it.
        var x = 0
        var y = 0
        var v: View? = view
        while (v != null && v !== root) {
            x += v.left
            y += v.top
            v = v.parent as? View
        }
        val w = if (view.width > 0) view.width else root.width
        val h = if (view.height > 0) view.height else root.height
        val bars = HostImpl.bars
        val below = root.height - (y + h)
        val right = root.width - (x + w)
        return intArrayOf(
            maxOf(0, bars.top - y), maxOf(0, bars.left - x), maxOf(0, bars.bottom - below), maxOf(0, bars.right - right),
            maxOf(0, HostImpl.ime.bottom - below),
        )
    }

    /** Tells Go the window's size and what covers it, when they changed. */
    fun reportSize() {
        val root = HostImpl.root
        if (root.width == 0) return
        val d = HostImpl.density.toDouble()
        val w = if (view.width > 0) view.width else root.width
        val h = if (view.height > 0) view.height else root.height
        val c = if (holder != null) IntArray(5) else covered()
        val size = doubleArrayOf(w / d, h / d, d, c[0] / d, c[1] / d, c[2] / d, c[3] / d, c[4] / d)
        if (last?.contentEquals(size) == true) return
        last = size
        Go.send(Event.resized(id, size[0], size[1], size[2], size[3], size[4], size[5], size[6], size[7]))
    }

    /** The insets changed: lays the window out again and tells Go. */
    open fun insetsChanged() = reportSize()

    abstract fun setBackground(argb: Int)
    open fun close() {}
}
