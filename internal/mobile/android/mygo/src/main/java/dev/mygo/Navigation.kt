// The navigation of phones on Android: each window in a page of a stack,
// whose pages above the first have a toolbar with a back arrow and the
// window's title, and bars of tabs (Material's bottom navigation), a
// stack in each tab, all in the root view, the one shown last in front.
// A window's router may show its pages as a stack of its own (SetStack):
// the window's toolbar then shows the title of the page shown, with a back
// arrow past the first, and the router moves the pages, as Android's apps
// move theirs.

package dev.mygo

import android.content.Context
import android.graphics.BitmapFactory
import android.graphics.drawable.BitmapDrawable
import android.view.View
import android.view.ViewGroup
import android.widget.FrameLayout
import android.widget.LinearLayout
import androidx.appcompat.view.ContextThemeWrapper
import com.google.android.material.bottomnavigation.BottomNavigationView
import com.google.android.material.appbar.MaterialToolbar
import com.google.android.material.color.DynamicColors

object Navigation {
    /** A window's page in a stack: its toolbar, when it has one, over it. */
    class Page(val window: HostWindow, var toolbar: Bar?, val view: LinearLayout)

    /**
     * A page's toolbar. It reaches under the status bar, and under the
     * navigation bar or a cutout at a side (in landscape), padded by their
     * room, and keeps its own height (Material's 64dp) below the status
     * bar. A toolbar's least height counts its padding, and only that least
     * height makes room for its title's margins: a padding eating it cut the
     * title. (The least height stays: the toolbar aligns its buttons to it.)
     */
    class Bar(context: Context) : MaterialToolbar(context) {
        fun inset(b: androidx.core.graphics.Insets) = setPadding(b.left, b.top, b.right, 0)

        override fun onMeasure(widthSpec: Int, heightSpec: Int) {
            super.onMeasure(widthSpec, heightSpec)
            val least = minimumHeight + paddingTop + paddingBottom
            if (measuredHeight in 1 until least) setMeasuredDimension(measuredWidth, resolveSize(least, heightSpec))
        }
    }

    /** A stack of pages: the first is the window it shows, the others pushed over it. */
    class Stack(context: Context) : FrameLayout(context) {
        val pages = ArrayList<Page>()
        var tabs: Tabs? = null
        var barAtRoot = ""
    }

    /** A bar of tabs, a stack in each. */
    class Tabs(context: Context, val id: Long) : LinearLayout(context) {
        val content = FrameLayout(context)
        val stacks = ArrayList<Stack>()
        val bar = BottomNavigationView(material(context))
        var selected = 0

        init {
            orientation = VERTICAL
            addView(content, LayoutParams(LayoutParams.MATCH_PARENT, 0, 1f))
            addView(bar, LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.WRAP_CONTENT))
            bar.labelVisibilityMode = BottomNavigationView.LABEL_VISIBILITY_LABELED
            bar.setOnItemSelectedListener { item ->
                val i = item.itemId
                if (i != selected) {
                    select(i)
                    Go.send(Event.tabSelected(id, i.toLong()))
                }
                true
            }
        }

        fun select(i: Int) {
            if (i !in stacks.indices) return
            selected = i
            stacks[i].pages.lastOrNull()?.window?.let { tint(it) }
            for ((k, s) in stacks.withIndex()) s.visibility = if (k == i) View.VISIBLE else View.GONE
            if (bar.selectedItemId != i) bar.selectedItemId = i
            for (p in stacks[i].pages) p.window.reportSize()
            HostImpl.updateBack() // the stack in front changed
        }
    }

    private val stacks = HashMap<Long, Stack>() // by window
    val tabs = HashMap<Long, Tabs>()
    private val root get() = HostImpl.root

    /** A context with Material 3's theme, and the wallpaper's colors where the system has them. */
    fun material(context: Context): Context =
        DynamicColors.wrapContextIfAvailable(ContextThemeWrapper(context, com.google.android.material.R.style.Theme_Material3_DayNight_NoActionBar))

    private fun toolbar(w: HostWindow, title: String): Bar {
        val t = Bar(material(w.context))
        t.title = title
        t.setBackgroundColor(if (w.fill != 0) w.fill else HostImpl.themeColor(android.R.attr.colorBackground))
        return t
    }

    /** back gives a toolbar the back arrow, which runs fn, or (null) takes it away. */
    private fun back(t: MaterialToolbar, fn: (() -> Unit)?) {
        if (fn == null) {
            t.navigationIcon = null
            t.setNavigationOnClickListener(null)
            return
        }
        t.setNavigationIcon(androidx.appcompat.R.drawable.abc_ic_ab_back_material)
        t.setNavigationContentDescription(androidx.appcompat.R.string.abc_action_bar_up_description)
        t.setNavigationOnClickListener { fn() }
    }

    private fun page(w: HostWindow, title: String?, back: Boolean): Page {
        (w.view.parent as? ViewGroup)?.removeView(w.view)
        val v = LinearLayout(w.context).apply { orientation = LinearLayout.VERTICAL }
        var toolbar: Bar? = null
        if (title != null) {
            val t = toolbar(w, title)
            if (back) back(t) { pop(w, true) }
            v.addView(t, LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT))
            toolbar = t
        }
        v.addView(w.view, LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, 1f))
        w.view.addOnLayoutChangeListener { _, _, _, _, _, _, _, _, _ -> w.reportSize() }
        return Page(w, toolbar, v)
    }

    private fun front(v: View) {
        (v.parent as? ViewGroup)?.let { if (it !== root) it.removeView(v) }
        if (v.parent == null) {
            root.addView(v, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT))
        } else {
            v.bringToFront()
        }
        insetsChanged()
        arrange()
    }

    /**
     * Only what is in front stays on screen: the surfaces of two windows have
     * no order of their own, whatever the order of their views. Not now: Go
     * may be waiting for this call, and a surface going waits for Go.
     */
    private fun arrange() {
        root.post {
            val top = root.childCount - 1
            for (k in 0..top) {
                val v = root.getChildAt(k)
                if (v is Stack || v is Tabs) v.visibility = if (k == top) View.VISIBLE else View.INVISIBLE
            }
        }
    }

    /** show brings a window to the front: its tab, its stack, or a stack of its own. */
    fun show(w: HostWindow) {
        val s = stacks[w.id]
        if (s != null) {
            val t = s.tabs
            if (t != null) {
                t.select(t.stacks.indexOf(s))
                front(t)
            } else {
                front(s)
            }
            return
        }
        val stack = Stack(w.context)
        val p = page(w, null, false)
        stack.pages.add(p)
        stack.addView(p.view, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT))
        stacks[w.id] = stack
        front(stack)
    }

    /** hide takes a window off the screen: its stack, or the pages above it in its stack. */
    fun hide(w: HostWindow) {
        val s = stacks[w.id] ?: return
        val i = s.pages.indexOfFirst { it.window === w }
        if (i == 0) {
            if (s.tabs == null) {
                root.removeView(s)
                for (p in s.pages) stacks.remove(p.window.id)
                arrange()
            }
        } else if (i > 0) {
            truncate(s, i, false)
        }
    }

    fun forget(w: HostWindow) {
        routed.remove(w.id)
        barItems.remove(w.id)
        val s = stacks.remove(w.id) ?: return
        s.pages.removeAll { it.window === w }
    }

    fun push(w: HostWindow, parent: HostWindow, title: String, animated: Boolean) {
        val s = stacks[parent.id] ?: run {
            show(parent)
            stacks[parent.id]!!
        }
        val at = s.pages.indexOfFirst { it.window === parent }
        if (at >= 0 && at < s.pages.size - 1) truncate(s, at + 1, false)
        val below = s.pages.lastOrNull()
        val p = page(w, title, true)
        s.pages.add(p)
        stacks[w.id] = s
        s.addView(p.view, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT))
        insetsChanged()
        tint(w)
        if (animated && s.width > 0) {
            p.view.translationX = s.width.toFloat()
            p.view.animate().translationX(0f).setDuration(250).withEndAction { below?.view?.visibility = View.INVISIBLE }.start()
            below?.view?.animate()?.translationX(-s.width * 0.25f)?.setDuration(250)?.start()
        } else {
            below?.view?.visibility = View.INVISIBLE
        }
    }

    fun pop(w: HostWindow, animated: Boolean) {
        val s = stacks[w.id] ?: return
        val i = s.pages.indexOfFirst { it.window === w }
        if (i > 0) truncate(s, i, animated)
    }

    /** truncate takes the pages from i up off the stack, which tells Go they went. */
    private fun truncate(s: Stack, i: Int, animated: Boolean) {
        val gone = s.pages.subList(i, s.pages.size).toList()
        s.pages.subList(i, s.pages.size).clear()
        val below = s.pages.lastOrNull()
        below?.view?.visibility = View.VISIBLE
        below?.view?.translationX = 0f
        below?.window?.let { tint(it) }
        for ((k, p) in gone.withIndex()) {
            stacks.remove(p.window.id)
            val last = k == gone.size - 1
            if (animated && last && s.width > 0) {
                below?.view?.translationX = -s.width * 0.25f
                below?.view?.animate()?.translationX(0f)?.setDuration(220)?.start()
                p.view.animate().translationX(s.width.toFloat()).setDuration(220).withEndAction { s.removeView(p.view) }.start()
            } else {
                s.removeView(p.view)
            }
            Go.send(Event.popped(p.window.id))
        }
        HostImpl.updateBack()
    }

    /** Whether the window in front can go off its stack. */
    fun canPop(): Boolean {
        for (k in root.childCount - 1 downTo 0) {
            val s = when (val v = root.getChildAt(k)) {
                is Tabs -> v.stacks.getOrNull(v.selected)
                is Stack -> v
                else -> null
            } ?: continue
            return s.pages.size > 1
        }
        return false
    }

    /** back pops the window in front off its stack, and reports whether there was one to pop. */
    fun back(): Boolean {
        for (k in root.childCount - 1 downTo 0) {
            val v = root.getChildAt(k)
            val s = when (v) {
                is Tabs -> v.stacks.getOrNull(v.selected)
                is Stack -> v
                else -> null
            } ?: continue
            if (s.pages.size > 1) {
                pop(s.pages.last().window, true)
                return true
            }
            return false
        }
        return false
    }

    // The pages of routers.

    /** A window's router showing its pages in its stack: how deep, the window's title, and whether the toolbar is the router's. */
    private class Routed(var depth: Int, val title: String, val added: Boolean)
    private val routed = HashMap<Long, Routed>()

    /**
     * setStack shows the pages of w's router as a stack: the toolbar shows the
     * title of the page shown, with a back arrow past the first, which takes
     * the router back. No titles take it away.
     */
    fun setStack(w: HostWindow, titles: List<String>) {
        val s = stacks[w.id] ?: return
        val i = s.pages.indexOfFirst { it.window === w }
        if (i < 0) return
        val p = s.pages[i]
        val r = routed[w.id]
        if (titles.isEmpty()) {
            if (r == null) return
            routed.remove(w.id)
            val t = p.toolbar ?: return
            if (r.added) {
                p.view.removeView(t)
                p.toolbar = null
            } else {
                t.title = r.title
                arrow(t, w, i)
            }
            return
        }
        var t = p.toolbar
        if (r == null) {
            val added = t == null
            if (t == null) {
                t = toolbar(w, titles.last())
                p.view.addView(t, 0, LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT))
                p.toolbar = t
                insetsChanged()
            }
            routed[w.id] = Routed(titles.size, t.title?.toString() ?: "", added)
        }
        routed[w.id]?.depth = titles.size
        t!!.title = titles.last()
        arrow(t, w, i)
    }

    /**
     * arrow gives the toolbar of w, the i-th page of its stack, the back arrow
     * it has: the router's, past its first page, or the stack's, past the first.
     */
    private fun arrow(t: MaterialToolbar, w: HostWindow, i: Int) {
        when {
            (routed[w.id]?.depth ?: 0) > 1 -> back(t) {
                val d = routed[w.id]?.depth ?: 1
                if (d > 1) Go.send(Event.stackPopped(w.id, (d - 1).toLong()))
            }
            i > 0 -> back(t) { pop(w, true) }
            else -> back(t, null)
        }
    }

    /**
     * The system's theme or size of text changed. A toolbar takes its colors
     * and the size of its title from the theme when it is made, so each is
     * made again, in the theme of now, with what the old one showed.
     */
    fun themeChanged() {
        for (s in stacks.values.toSet()) for ((i, p) in s.pages.withIndex()) {
            val old = p.toolbar ?: continue
            val t = toolbar(p.window, old.title?.toString() ?: "")
            arrow(t, p.window, i)
            val at = p.view.indexOfChild(old)
            p.view.removeView(old)
            p.view.addView(t, at, old.layoutParams)
            p.toolbar = t
            barItems[p.window.id]?.let { setBarItems(p.window, it) }
        }
        insetsChanged()
    }

    // Tabs.

    fun createTabs(id: Long, context: Context) {
        tabs[id] = Tabs(context, id)
    }

    fun addTab(id: Long, w: HostWindow, title: String, icon: ByteArray) {
        val t = tabs[id] ?: return
        stacks[w.id]?.let { old ->
            if (old.tabs == null) {
                root.removeView(old)
                arrange()
            }
            stacks.remove(w.id)
        }
        val stack = Stack(w.context)
        stack.tabs = t
        val p = page(w, title.ifEmpty { null }, false)
        stack.pages.add(p)
        stack.addView(p.view, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT))
        stacks[w.id] = stack
        val i = t.stacks.size
        t.stacks.add(stack)
        t.content.addView(stack, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT))
        val item = t.bar.menu.add(0, i, i, title)
        if (icon.isNotEmpty()) {
            BitmapFactory.decodeByteArray(icon, 0, icon.size)?.let { item.icon = BitmapDrawable(w.context.resources, it) }
        }
    }

    fun showTabs(id: Long) {
        val t = tabs[id] ?: return
        t.select(0)
        front(t)
    }

    fun setBadge(id: Long, i: Int, badge: String) {
        val t = tabs[id] ?: return
        if (badge.isEmpty()) {
            t.bar.removeBadge(i)
            return
        }
        val b = t.bar.getOrCreateBadge(i)
        badge.toIntOrNull()?.let { b.number = it } ?: run { b.text = badge }
    }

    /**
     * setTabTitle changes what tab i's item reads, and the toolbar the tab's
     * stack shows while its window is at the root.
     */
    fun setTabTitle(id: Long, i: Int, title: String) {
        val t = tabs[id] ?: return
        t.bar.menu.findItem(i)?.title = title
        val s = t.stacks.getOrNull(i) ?: return
        val p = s.pages.firstOrNull() ?: return
        if (p.window.id in routed) return // the router's page has the toolbar's title
        val tb = p.toolbar
        if (title.isEmpty()) {
            if (tb != null) {
                p.view.removeView(tb)
                p.toolbar = null
                insetsChanged()
            }
            return
        }
        if (tb != null) {
            tb.title = title
            return
        }
        val nt = toolbar(p.window, title)
        p.view.addView(nt, 0, LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT))
        p.toolbar = nt
        insetsChanged()
    }

    fun closeTabs(id: Long) {
        val t = tabs.remove(id) ?: return
        root.removeView(t)
        arrange()
    }

    /**
     * The bars around a window take the color its content fills it with:
     * its toolbar, and the bar of tabs while it is in front of its tab.
     */
    fun backgroundChanged(w: HostWindow) {
        val s = stacks[w.id] ?: return
        s.pages.firstOrNull { it.window === w }?.toolbar?.setBackgroundColor(w.fill)
        val t = s.tabs ?: return
        if (t.stacks.getOrNull(t.selected) === s && s.pages.lastOrNull()?.window === w) tint(w)
    }

    private fun tint(w: HostWindow) {
        if (w.fill == 0) return
        val s = stacks[w.id] ?: return
        s.tabs?.bar?.setBackgroundColor(w.fill)
    }

    private val barItems = HashMap<Long, SetBarItemsCall>() // by window, for toolbars made again

    /** Buttons at the end of the window's toolbar. */
    fun setBarItems(w: HostWindow, c: SetBarItemsCall) {
        barItems[w.id] = c
        val t = stacks[w.id]?.pages?.firstOrNull { it.window === w }?.toolbar ?: return
        t.menu.clear()
        c.titles.forEachIndexed { i, title ->
            val item = t.menu.add(0, i, i, title)
            val icon = android.util.Base64.decode(c.icons[i], android.util.Base64.DEFAULT)
            if (icon.isNotEmpty()) {
                BitmapFactory.decodeByteArray(icon, 0, icon.size)?.let { item.icon = BitmapDrawable(w.context.resources, it) }
            }
            item.setShowAsAction(android.view.MenuItem.SHOW_AS_ACTION_IF_ROOM)
        }
        t.setOnMenuItemClickListener {
            Go.send(Event.barItemClicked(c.id, it.itemId.toLong()))
            true
        }
    }

    fun setTitle(w: HostWindow, title: String) {
        if (w.id in routed) return // the router's page has the toolbar's title
        stacks[w.id]?.pages?.firstOrNull { it.window === w }?.toolbar?.title = title
    }

    /**
     * The toolbars at the top of the screen take the status bar's room, and
     * the room of what covers the screen's sides: the navigation bar of three
     * buttons in landscape, a cutout.
     */
    fun insetsChanged() {
        val b = HostImpl.bars
        for (s in stacks.values.toSet()) for (p in s.pages) p.toolbar?.inset(b)
        for (w in HostImpl.windows.values) w.insetsChanged()
    }
}
