package dev.mygo

import android.content.Context
import android.graphics.Rect
import android.os.Bundle
import android.text.InputType
import android.view.Choreographer
import android.view.InputDevice
import android.view.ActionMode
import android.view.KeyEvent
import android.widget.Magnifier
import android.view.Menu
import android.view.MenuItem
import android.view.MotionEvent
import android.view.PointerIcon
import android.view.SurfaceHolder
import android.view.SurfaceView
import android.view.View
import android.view.accessibility.AccessibilityEvent
import android.view.accessibility.AccessibilityNodeInfo
import android.view.accessibility.AccessibilityNodeProvider
import android.view.inputmethod.BaseInputConnection
import android.view.inputmethod.EditorInfo
import android.view.inputmethod.InputConnection
import android.view.inputmethod.InputMethodManager

/**
 * A window showing native UI, which Go draws with OpenGL ES into the
 * surface's ANativeWindow. The view gives Go the display's refreshes
 * (Choreographer), the fingers, a hardware keyboard, the soft keyboard
 * (an InputConnection, with the composing text of input methods) and
 * TalkBack's questions.
 */
class SurfaceWindow(context: Context, id: Long) : HostWindow(context, id) {
    /** The window's number, which views' own id would shadow. */
    private val wid: Long = id
    private val surface = Surface(context)
    /** The surface, and under it the views it holds. */
    private val box = android.widget.FrameLayout(context).apply {
        addView(surface, android.widget.FrameLayout.LayoutParams(android.widget.FrameLayout.LayoutParams.MATCH_PARENT, android.widget.FrameLayout.LayoutParams.MATCH_PARENT))
    }
    override val view: View get() = box
    private var native = 0L
    private var framePending = false
    private val frame = Choreographer.FrameCallback {
        framePending = false
        Go.send(Event.frame(wid))
    }

    // The input being edited, as Go last told it (SetTextInput): the text
    // around the selection, from base, and offsets in UTF-16 units of the
    // document, as InputConnection counts.
    private var input = SetTextInputCall(markStart = -1, markEnd = -1)
    private val editing get() = input.active

    private var nodes: List<Node>? = null
    /** Whether the content takes the system's back. */
    var backAvailable = false

    // A SurfaceView's background would draw over its surface, hiding the
    // frames: the page around it takes the color.
    override fun setBackground(argb: Int) {
        box.setBackgroundColor(argb)
    }

    /** A view the surface holds, in a view that clips it. */
    private class Held(val window: HostWindow, val clip: android.widget.FrameLayout) {
        var box = Rect()
        var covers = emptyList<Rect>()
        fun takes(x: Int, y: Int) = clip.visibility == View.VISIBLE && box.contains(x, y) && covers.none { it.contains(x, y) }
    }

    private val held = ArrayList<Held>()

    /**
     * Holds a view under the surface, whose frames are transparent where the
     * content shows it: the surface goes over the window, which it does only as
     * it is made, so it is made again the first time.
     */
    fun hold(w: HostWindow) {
        w.holder = this
        (w.view.parent as? android.view.ViewGroup)?.removeView(w.view)
        val clip = android.widget.FrameLayout(context)
        clip.clipChildren = true
        clip.visibility = View.GONE
        clip.addView(w.view, android.widget.FrameLayout.LayoutParams(0, 0))
        box.addView(clip, 0, android.widget.FrameLayout.LayoutParams(0, 0))
        if (held.isEmpty()) {
            // Not now: Go waits for this call, and the surface going waits
            // for Go.
            box.post {
                surface.setZOrderOnTop(true)
                surface.holder.setFormat(android.graphics.PixelFormat.TRANSLUCENT)
                box.removeView(surface)
                box.addView(surface, android.widget.FrameLayout.LayoutParams(android.widget.FrameLayout.LayoutParams.MATCH_PARENT, android.widget.FrameLayout.LayoutParams.MATCH_PARENT))
            }
        }
        held.add(Held(w, clip))
    }

    fun release(w: HostWindow) {
        val i = held.indexOfFirst { it.window === w }
        if (i < 0) return
        box.removeView(held[i].clip)
        held.removeAt(i)
    }

    fun placeViews(places: ByteArray) {
        val r = WireReader(places)
        val d = density
        fun rect(): Rect {
            val x = r.float()
            val y = r.float()
            val w = r.float()
            val h = r.float()
            return Rect((x * d).toInt(), (y * d).toInt(), ((x + w) * d).toInt(), ((y + h) * d).toInt())
        }
        val shown = HashSet<Held>()
        var order = 0
        while (!r.done()) {
            val id = r.int()
            val frame = rect()
            val clip = rect()
            val covers = List(r.int().toInt()) { rect() }
            val h = held.firstOrNull { it.window.id == id } ?: continue
            val lp = h.clip.layoutParams as android.widget.FrameLayout.LayoutParams
            lp.leftMargin = clip.left
            lp.topMargin = clip.top
            lp.width = clip.width()
            lp.height = clip.height()
            h.clip.layoutParams = lp
            val vp = h.window.view.layoutParams as android.widget.FrameLayout.LayoutParams
            vp.leftMargin = frame.left - clip.left
            vp.topMargin = frame.top - clip.top
            vp.width = frame.width()
            vp.height = frame.height()
            h.window.view.layoutParams = vp
            h.window.view.post { h.window.reportSize() }
            h.box = clip
            h.covers = covers
            h.clip.visibility = View.VISIBLE
            if (box.indexOfChild(h.clip) != order) {
                box.removeView(h.clip)
                box.addView(h.clip, order)
            }
            order++
            shown.add(h)
        }
        for (h in held) if (h !in shown) h.clip.visibility = View.GONE
    }

    /** A finger going down on a view it holds, which gets it: the content hears of it. */
    private fun heldTakes(e: MotionEvent): Boolean {
        val x = e.x.toInt()
        val y = e.y.toInt()
        if (held.asReversed().none { it.takes(x, y) }) return false
        if (e.actionMasked == MotionEvent.ACTION_DOWN) Go.send(Event.viewPressed(wid, e.x / density, e.y / density))
        return true
    }

    fun requestFrame() {
        if (framePending) return
        framePending = true
        Choreographer.getInstance().postFrameCallback(frame)
    }

    override fun close() {
        if (framePending) Choreographer.getInstance().removeFrameCallback(frame)
        releaseNative()
    }

    private fun releaseNative() {
        if (native != 0L) {
            Go.ask(Event.surfaceDestroyed(wid))
            MyGoNative.releaseNativeWindow(native)
            native = 0
        }
    }

    private val density get() = HostImpl.density.toDouble()
    /** The virtual view that stands for the input in focus, to autofill. */
    private val autofillNode = 1

    fun setTextInput(c: SetTextInputCall) {
        val was = input
        input = c
        tellAutofill(was, c)
        val imm = context.getSystemService(Context.INPUT_METHOD_SERVICE) as InputMethodManager
        val traits = was.multiline != c.multiline || was.secure != c.secure || was.keyboard != c.keyboard || was.returnKey != c.returnKey ||
            was.capitalize != c.capitalize || was.autocorrect != c.autocorrect || was.autofill != c.autofill
        when {
            c.active && (!was.active || traits || c.serial != was.serial) -> {
                surface.requestFocus()
                imm.restartInput(surface)
                imm.showSoftInput(surface, 0)
            }
            !c.active && was.active -> {
                imm.hideSoftInputFromWindow(surface.windowToken, 0)
                imm.restartInput(surface)
            }
            c.active -> {
                if (c.show != was.show) imm.showSoftInput(surface, 0)
                if (c.selStart != was.selStart || c.selEnd != was.selEnd || c.markStart != was.markStart || c.markEnd != was.markEnd) {
                    imm.updateSelection(surface, c.selStart.toInt(), c.selEnd.toInt(), c.markStart.toInt(), c.markEnd.toInt())
                }
            }
        }
    }

    /** Sends an edit and takes the state after it, which the keyboard goes on to read at once. */
    private fun edit(msg: ByteArray) {
        val r = Go.ask(msg) ?: return
        val st = TextStateReply(r)
        val c = input
        val changed = st.selStart != c.selStart || st.selEnd != c.selEnd || st.markStart != c.markStart || st.markEnd != c.markEnd
        c.text = st.text
        c.base = st.base
        c.length = st.length
        c.selStart = st.selStart
        c.selEnd = st.selEnd
        c.markStart = st.markStart
        c.markEnd = st.markEnd
        if (autofillHint(c) != null) {
            context.getSystemService(android.view.autofill.AutofillManager::class.java)
                ?.notifyValueChanged(surface, autofillNode, android.view.autofill.AutofillValue.forText(wholeText(c)))
        }
        if (changed) {
            val imm = context.getSystemService(Context.INPUT_METHOD_SERVICE) as InputMethodManager
            imm.updateSelection(surface, c.selStart.toInt(), c.selEnd.toInt(), c.markStart.toInt(), c.markEnd.toInt())
        }
    }

    /** The part of the document from a to b, the host's copy holding it or not. */
    private fun document(a: Int, b: Int): CharSequence {
        val c = input
        val lo = a - c.base.toInt()
        val hi = b - c.base.toInt()
        if (lo >= 0 && hi <= c.text.length && lo <= hi) return c.text.substring(lo, hi)
        val r = Go.ask(Event.textRange(wid, a.toLong(), b.toLong())) ?: return ""
        return TextRangeReply(r).text
    }

    /** A mouse's pointer, on a Chromebook or a tablet with one. */
    fun setCursor(cursor: String) {
        val kind = when (cursor) {
            "text" -> PointerIcon.TYPE_TEXT
            "pointer" -> PointerIcon.TYPE_HAND
            "move" -> PointerIcon.TYPE_ALL_SCROLL
            "resizeX" -> PointerIcon.TYPE_HORIZONTAL_DOUBLE_ARROW
            "resizeY" -> PointerIcon.TYPE_VERTICAL_DOUBLE_ARROW
            "grab" -> PointerIcon.TYPE_GRAB
            "grabbing" -> PointerIcon.TYPE_GRABBING
            "crosshair" -> PointerIcon.TYPE_CROSSHAIR
            "notAllowed" -> PointerIcon.TYPE_NO_DROP
            "none" -> PointerIcon.TYPE_NULL
            else -> PointerIcon.TYPE_DEFAULT
        }
        surface.pointerIcon = PointerIcon.getSystemIcon(context, kind)
    }

    private var actionMode: ActionMode? = null

    /** The system's floating menu of the editing commands asked for. */
    fun showMenu(c: ShowMenuCall) {
        actionMode?.finish()
        val d = density
        val at = Rect((c.x * d).toInt(), (c.y * d).toInt(), (c.x * d).toInt() + 1, (c.y * d).toInt() + 1)
        actionMode = surface.startActionMode(object : ActionMode.Callback2() {
            override fun onCreateActionMode(m: ActionMode, menu: Menu): Boolean {
                val labels = mapOf("cut" to android.R.string.cut, "copy" to android.R.string.copy,
                    "paste" to android.R.string.paste, "selectAll" to android.R.string.selectAll)
                c.actions.forEachIndexed { i, name -> labels[name]?.let { menu.add(0, i, i, it) } }
                return true
            }
            override fun onPrepareActionMode(m: ActionMode, menu: Menu) = false
            override fun onActionItemClicked(m: ActionMode, item: MenuItem): Boolean {
                c.actions.getOrNull(item.itemId)?.let { edit(Event.textCommand(wid, it)) }
                m.finish()
                return true
            }
            override fun onDestroyActionMode(m: ActionMode) { actionMode = null }
            override fun onGetContentRect(m: ActionMode, v: View, out: Rect) { out.set(at) }
        }, ActionMode.TYPE_FLOATING)
    }

    private var magnifier: Magnifier? = null // API 28

    /** The system's magnifier of what is at a point. */
    fun showMagnifier(c: ShowMagnifierCall) {
        if (android.os.Build.VERSION.SDK_INT < 28) return
        val m = magnifier ?: Magnifier.Builder(surface).build().also { magnifier = it }
        m.show((c.x * density).toFloat(), (c.y * density).toFloat())
    }

    fun hideMagnifier() {
        if (android.os.Build.VERSION.SDK_INT >= 28) magnifier?.dismiss()
    }

    fun accessibilityChanged(focus: Long) {
        nodes = null
        surface.sendAccessibilityEvent(AccessibilityEvent.TYPE_WINDOW_CONTENT_CHANGED)
    }

    private fun tree(): List<Node> {
        nodes?.let { return it }
        val r = Go.ask(Event.accessibilityTree(wid)) ?: return emptyList()
        val n = Node.decode(AccessibilityTreeReply(r).nodes)
        nodes = n
        return n
    }

    private inner class Surface(context: Context) : SurfaceView(context), SurfaceHolder.Callback {
        init {
            holder.addCallback(this)
            isFocusable = true
            isFocusableInTouchMode = true
        }

        override fun surfaceCreated(h: SurfaceHolder) {
            native = MyGoNative.nativeWindow(h.surface)
            val hz = display?.supportedModes?.maxOfOrNull { it.refreshRate }?.toDouble() ?: display?.refreshRate?.toDouble() ?: 60.0
            Go.send(Event.surfaceChanged(wid, native, width / density, height / density, density, hz))
        }

        override fun surfaceChanged(h: SurfaceHolder, format: Int, w: Int, ht: Int) {
            preferFastest(h)
            reportSize()
            requestFrame()
        }

        /** Asks the display for its fastest refresh, which Choreographer then gives. */
        private fun preferFastest(h: SurfaceHolder) {
            if (android.os.Build.VERSION.SDK_INT < 30) return
            val modes = display?.supportedModes ?: return
            val hz = modes.maxOfOrNull { it.refreshRate } ?: return
            runCatching { h.surface.setFrameRate(hz, android.view.Surface.FRAME_RATE_COMPATIBILITY_DEFAULT) }
        }

        // A mouse's pointer hovers.
        override fun onHoverEvent(e: MotionEvent): Boolean {
            val d = density
            val phase = if (e.actionMasked == MotionEvent.ACTION_HOVER_EXIT) "cancel" else "move"
            Go.send(Event.touch(wid, 0, phase, "mouse", e.x / d, e.y / d, 0.0, 0, e.eventTime / 1000.0))
            return true
        }

        override fun surfaceDestroyed(h: SurfaceHolder) = releaseNative()

        // A finger on a view it holds passes through to it, under this view.
        override fun dispatchTouchEvent(e: MotionEvent): Boolean {
            if (e.actionMasked == MotionEvent.ACTION_DOWN && heldTakes(e)) return false
            return super.dispatchTouchEvent(e)
        }

        override fun onTouchEvent(e: MotionEvent): Boolean {
            val d = density
            fun send(i: Int, phase: String) {
                val kind = when (e.getToolType(i)) {
                    MotionEvent.TOOL_TYPE_MOUSE -> "mouse"
                    MotionEvent.TOOL_TYPE_STYLUS -> "pen"
                    else -> "touch"
                }
                Go.send(Event.touch(wid, e.getPointerId(i).toLong(), phase, kind, e.getX(i) / d, e.getY(i) / d,
                    e.getPressure(i).toDouble(), e.buttonState.toLong().coerceAtLeast(1), e.eventTime / 1000.0))
            }
            when (e.actionMasked) {
                MotionEvent.ACTION_DOWN, MotionEvent.ACTION_POINTER_DOWN -> send(e.actionIndex, "down")
                MotionEvent.ACTION_MOVE -> for (i in 0 until e.pointerCount) send(i, "move")
                MotionEvent.ACTION_UP, MotionEvent.ACTION_POINTER_UP -> send(e.actionIndex, "up")
                MotionEvent.ACTION_CANCEL -> for (i in 0 until e.pointerCount) send(i, "cancel")
            }
            return true
        }

        override fun onGenericMotionEvent(e: MotionEvent): Boolean {
            if (e.isFromSource(InputDevice.SOURCE_CLASS_POINTER) && e.actionMasked == MotionEvent.ACTION_SCROLL) {
                val d = density
                val line = 48.0
                Go.send(Event.scroll(wid, e.x / d, e.y / d, -e.getAxisValue(MotionEvent.AXIS_HSCROLL) * line,
                    -e.getAxisValue(MotionEvent.AXIS_VSCROLL) * line, false, ""))
                return true
            }
            return super.onGenericMotionEvent(e)
        }

        private fun key(code: Int, e: KeyEvent, down: Boolean): Boolean {
            val name = keyNames[code] ?: e.unicodeChar.takeIf { it > 0 }?.let { String(Character.toChars(it)) } ?: return false
            var mods = 0L
            if (e.isShiftPressed) mods = mods or 1
            if (e.isCtrlPressed) mods = mods or 2
            if (e.isAltPressed) mods = mods or 4
            if (e.isMetaPressed) mods = mods or 8
            val text = if (e.unicodeChar > 0) String(Character.toChars(e.unicodeChar)) else ""
            val r = Go.ask(Event.key(wid, down, name, text, mods, e.repeatCount > 0)) ?: return false
            val handled = KeyReply(r).handled
            // Keys typing text type it, which the content takes as text.
            if (!handled && down && editing && text.isNotEmpty() && keyNames[code] == null) {
                edit(Event.textReplace(wid, false, 0, 0, text))
                return true
            }
            return handled
        }

        override fun onKeyDown(code: Int, e: KeyEvent) = key(code, e, true) || super.onKeyDown(code, e)
        override fun onKeyUp(code: Int, e: KeyEvent) = key(code, e, false) || super.onKeyUp(code, e)

        override fun onCheckIsTextEditor() = editing

        override fun onCreateInputConnection(info: EditorInfo): InputConnection? {
            if (!editing) return null
            val c = input
            info.inputType = inputType(c)
            info.imeOptions = EditorInfo.IME_FLAG_NO_FULLSCREEN or when (c.returnKey) {
                "go" -> EditorInfo.IME_ACTION_GO
                "next" -> EditorInfo.IME_ACTION_NEXT
                "search" -> EditorInfo.IME_ACTION_SEARCH
                "send" -> EditorInfo.IME_ACTION_SEND
                "done" -> EditorInfo.IME_ACTION_DONE
                else -> if (c.multiline) EditorInfo.IME_ACTION_NONE else EditorInfo.IME_ACTION_DONE
            }
            if (c.multiline && c.returnKey.isEmpty()) info.imeOptions = info.imeOptions or EditorInfo.IME_FLAG_NO_ENTER_ACTION
            info.initialSelStart = c.selStart.toInt()
            info.initialSelEnd = c.selEnd.toInt()
            info.packageName = context.packageName
            return Connection(this)
        }

        override fun getAccessibilityNodeProvider(): AccessibilityNodeProvider = Provider()

        // Autofill: to the system, the input in focus is the one field of a
        // form of its own. The window's surface draws it: only this says what
        // it is, so that saved passwords and codes from SMS reach it.
        override fun onProvideAutofillVirtualStructure(s: android.view.ViewStructure, flags: Int) {
            s.setClassName(javaClass.name)
            val c = input
            val hint = autofillHint(c)
            if (!editing || hint == null) return
            val child = s.newChild(s.addChildCount(1))
            child.setAutofillId(s.autofillId!!, autofillNode)
            child.setClassName("android.widget.EditText")
            child.setAutofillType(AUTOFILL_TYPE_TEXT)
            child.setAutofillHints(arrayOf(hint))
            child.setInputType(inputType(c))
            child.setDataIsSensitive(c.secure)
            child.setVisibility(VISIBLE)
            child.setFocused(true)
            child.setFocusable(true)
            child.setEnabled(true)
            child.setDimens((c.x * density).toInt(), (c.y * density).toInt(), 0, 0, (c.width * density).toInt(), (c.height * density).toInt())
            child.setAutofillValue(android.view.autofill.AutofillValue.forText(wholeText(c)))
        }

        override fun autofill(values: android.util.SparseArray<android.view.autofill.AutofillValue>) {
            val v = values.get(autofillNode) ?: return
            if (!editing || !v.isText) return
            edit(Event.textReplace(wid, true, 0, input.length, v.textValue.toString()))
        }
    }

    /** The hint of Android's autofill for an input's traits, null for none. */
    private fun autofillHint(c: SetTextInputCall): String? = when (c.autofill) {
        "username" -> View.AUTOFILL_HINT_USERNAME
        "password", "newPassword" -> View.AUTOFILL_HINT_PASSWORD
        "oneTimeCode" -> "smsOTPCode" // androidx.autofill's HintConstants
        "email" -> View.AUTOFILL_HINT_EMAIL_ADDRESS
        "name" -> View.AUTOFILL_HINT_NAME
        "phone" -> View.AUTOFILL_HINT_PHONE
        "address" -> View.AUTOFILL_HINT_POSTAL_ADDRESS
        "postalCode" -> View.AUTOFILL_HINT_POSTAL_CODE
        "creditCard" -> View.AUTOFILL_HINT_CREDIT_CARD_NUMBER
        else -> null
    }

    /** The input's whole text, which the host's copy may hold only part of. */
    private fun wholeText(c: SetTextInputCall): String =
        if (c.base == 0L && c.text.length.toLong() == c.length) c.text else document(0, c.length.toInt()).toString()

    /** Tells the system's autofill what the input did, for those it knows of. */
    private fun tellAutofill(was: SetTextInputCall, c: SetTextInputCall) {
        val am = context.getSystemService(android.view.autofill.AutofillManager::class.java) ?: return
        val had = was.active && autofillHint(was) != null
        val has = c.active && autofillHint(c) != null
        val another = c.serial != was.serial || !was.active
        if (had && (!c.active || another)) {
            am.notifyViewExited(surface, autofillNode)
            // A form sent or left: the system may offer to save what it holds.
            if (was.secure && was.length > 0 && !has) am.commit()
        }
        if (has && (!had || another)) {
            val at = IntArray(2)
            surface.getLocationOnScreen(at)
            val x = at[0] + (c.x * density).toInt()
            val y = at[1] + (c.y * density).toInt()
            val r = Rect(x, y, x + (c.width * density).toInt(), y + (c.height * density).toInt())
            surface.importantForAutofill = View.IMPORTANT_FOR_AUTOFILL_YES
            am.notifyViewEntered(surface, autofillNode, r)
        }
    }

    /** The inputType of EditorInfo for an input's traits. */
    private fun inputType(c: SetTextInputCall): Int {
        var t = when (c.keyboard) {
            "number" -> InputType.TYPE_CLASS_NUMBER
            "decimal" -> InputType.TYPE_CLASS_NUMBER or InputType.TYPE_NUMBER_FLAG_DECIMAL or InputType.TYPE_NUMBER_FLAG_SIGNED
            "phone" -> InputType.TYPE_CLASS_PHONE
            "email" -> InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_VARIATION_EMAIL_ADDRESS
            "url" -> InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_VARIATION_URI
            else -> InputType.TYPE_CLASS_TEXT
        }
        if (c.secure) {
            t = if (t and InputType.TYPE_MASK_CLASS == InputType.TYPE_CLASS_NUMBER) {
                InputType.TYPE_CLASS_NUMBER or InputType.TYPE_NUMBER_VARIATION_PASSWORD
            } else {
                InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_VARIATION_PASSWORD
            }
            return t
        }
        if (t and InputType.TYPE_MASK_CLASS != InputType.TYPE_CLASS_TEXT) return t
        if (c.multiline) t = t or InputType.TYPE_TEXT_FLAG_MULTI_LINE
        val plain = c.keyboard.isEmpty() || c.keyboard == "search"
        t = t or when (c.capitalize) {
            "characters" -> InputType.TYPE_TEXT_FLAG_CAP_CHARACTERS
            "words" -> InputType.TYPE_TEXT_FLAG_CAP_WORDS
            "sentences" -> InputType.TYPE_TEXT_FLAG_CAP_SENTENCES
            "none" -> 0
            else -> if (plain) InputType.TYPE_TEXT_FLAG_CAP_SENTENCES else 0
        }
        t = t or when (c.autocorrect) {
            "off" -> InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS
            "on" -> InputType.TYPE_TEXT_FLAG_AUTO_CORRECT
            else -> if (plain) InputType.TYPE_TEXT_FLAG_AUTO_CORRECT else 0
        }
        return t
    }

    /** The soft keyboard's edits, to Go. */
    private inner class Connection(v: View) : BaseInputConnection(v, false) {
        override fun commitText(t: CharSequence, newCursorPosition: Int): Boolean {
            edit(Event.textReplace(wid, false, 0, 0, t.toString()))
            placeCursor(t.length, newCursorPosition)
            return true
        }

        override fun commitCompletion(c: android.view.inputmethod.CompletionInfo): Boolean {
            c.text?.let { edit(Event.textReplace(wid, false, 0, 0, it.toString())) }
            return true
        }

        override fun commitCorrection(c: android.view.inputmethod.CorrectionInfo): Boolean {
            val at = c.offset.toLong()
            edit(Event.textReplace(wid, true, at, at + c.oldText.length, c.newText.toString()))
            return true
        }

        override fun setComposingText(t: CharSequence, newCursorPosition: Int): Boolean {
            // The selection in the text composed, from the cursor Android
            // places relative to it: past it, the common case, or before.
            val n = t.length.toLong()
            val at = if (newCursorPosition > 0) n else 0L
            edit(Event.textMark(wid, false, 0, 0, t.toString(), at, at))
            return true
        }

        override fun setComposingRegion(start: Int, end: Int): Boolean {
            val a = minOf(start, end).coerceAtLeast(0)
            val b = maxOf(start, end).coerceAtMost(input.length.toInt())
            val text = document(a, b).toString()
            val n = text.length.toLong()
            edit(Event.textMark(wid, true, a.toLong(), b.toLong(), text, n, n))
            return true
        }

        override fun finishComposingText(): Boolean {
            edit(Event.textUnmark(wid))
            return true
        }

        override fun deleteSurroundingText(before: Int, after: Int): Boolean {
            edit(Event.textDelete(wid, before.toLong(), after.toLong()))
            return true
        }

        override fun deleteSurroundingTextInCodePoints(before: Int, after: Int): Boolean {
            // Units of the text around the selection.
            val a = minOf(input.selStart, input.selEnd).toInt()
            val b = maxOf(input.selStart, input.selEnd).toInt()
            val head = document(maxOf(0, a - 2 * before), a).toString()
            val tail = document(b, minOf(input.length.toInt(), b + 2 * after)).toString()
            val units = head.length - head.offsetByCodePoints(head.length, -minOf(before, head.codePointCount(0, head.length)))
            val unitsAfter = tail.offsetByCodePoints(0, minOf(after, tail.codePointCount(0, tail.length)))
            edit(Event.textDelete(wid, units.toLong(), unitsAfter.toLong()))
            return true
        }

        override fun setSelection(start: Int, end: Int): Boolean {
            edit(Event.textSelect(wid, start.toLong(), end.toLong()))
            return true
        }

        private fun placeCursor(committed: Int, newCursorPosition: Int) {
            // Past the text, as nearly all input methods ask, is where the
            // insertion leaves it.
            if (newCursorPosition == 1) return
            val end = input.selEnd.toInt()
            val at = if (newCursorPosition > 0) end + newCursorPosition - 1 else end - committed + newCursorPosition
            val p = at.coerceIn(0, input.length.toInt()).toLong()
            edit(Event.textSelect(wid, p, p))
        }

        override fun sendKeyEvent(e: KeyEvent): Boolean {
            if (e.action == KeyEvent.ACTION_DOWN) when (e.keyCode) {
                KeyEvent.KEYCODE_DEL -> edit(Event.textCommand(wid, "deleteBackward"))
                KeyEvent.KEYCODE_FORWARD_DEL -> edit(Event.textCommand(wid, "deleteForward"))
                KeyEvent.KEYCODE_ENTER, KeyEvent.KEYCODE_NUMPAD_ENTER -> edit(Event.textCommand(wid, "newline"))
                KeyEvent.KEYCODE_TAB -> edit(Event.textCommand(wid, "tab"))
                KeyEvent.KEYCODE_DPAD_LEFT -> edit(Event.textCommand(wid, "left"))
                KeyEvent.KEYCODE_DPAD_RIGHT -> edit(Event.textCommand(wid, "right"))
                KeyEvent.KEYCODE_DPAD_UP -> edit(Event.textCommand(wid, "up"))
                KeyEvent.KEYCODE_DPAD_DOWN -> edit(Event.textCommand(wid, "down"))
                else -> if (e.unicodeChar > 0) edit(Event.textReplace(wid, false, 0, 0, String(Character.toChars(e.unicodeChar))))
            }
            return true
        }

        override fun performEditorAction(action: Int): Boolean {
            edit(Event.textCommand(wid, "newline"))
            return true
        }

        override fun performContextMenuAction(action: Int): Boolean {
            when (action) {
                android.R.id.selectAll -> edit(Event.textCommand(wid, "selectAll"))
                android.R.id.cut -> edit(Event.textCommand(wid, "cut"))
                android.R.id.copy -> edit(Event.textCommand(wid, "copy"))
                android.R.id.paste -> edit(Event.textCommand(wid, "paste"))
                else -> return false
            }
            return true
        }

        override fun getTextBeforeCursor(n: Int, flags: Int): CharSequence {
            val a = minOf(input.selStart, input.selEnd).toInt()
            return document(maxOf(0, a - n), a)
        }

        override fun getTextAfterCursor(n: Int, flags: Int): CharSequence {
            val b = maxOf(input.selStart, input.selEnd).toInt()
            return document(b, minOf(input.length.toInt(), b + n))
        }

        override fun getSelectedText(flags: Int): CharSequence? {
            val a = minOf(input.selStart, input.selEnd).toInt()
            val b = maxOf(input.selStart, input.selEnd).toInt()
            return if (a == b) null else document(a, b)
        }

        override fun getExtractedText(req: android.view.inputmethod.ExtractedTextRequest?, flags: Int): android.view.inputmethod.ExtractedText {
            val c = input
            return android.view.inputmethod.ExtractedText().apply {
                text = c.text
                startOffset = c.base.toInt()
                selectionStart = c.selStart.toInt() - c.base.toInt()
                selectionEnd = c.selEnd.toInt() - c.base.toInt()
            }
        }

        override fun getCursorCapsMode(reqModes: Int): Int {
            val a = minOf(input.selStart, input.selEnd).toInt()
            return android.text.TextUtils.getCapsMode(document(maxOf(0, a - 64), a).toString(), minOf(a, 64), reqModes)
        }
    }

    /** The element TalkBack's cursor is on. */
    private var accessFocus = View.NO_ID

    private fun sendVirtual(id: Int, type: Int) {
        val e = AccessibilityEvent(type)
        e.packageName = context.packageName
        e.setSource(surface, id)
        (surface.parent as? android.view.ViewGroup)?.requestSendAccessibilityEvent(surface, e)
    }

    /** TalkBack's view of the elements of native UI, as virtual views. */
    private inner class Provider : AccessibilityNodeProvider() {
        override fun createAccessibilityNodeInfo(virtualId: Int): AccessibilityNodeInfo? {
            val nodes = tree()
            if (virtualId == View.NO_ID) {
                val info = AccessibilityNodeInfo.obtain(surface)
                surface.onInitializeAccessibilityNodeInfo(info)
                for (n in nodes) info.addChild(surface, n.id.toInt())
                return info
            }
            val n = nodes.firstOrNull { it.id.toInt() == virtualId } ?: return null
            val info = AccessibilityNodeInfo.obtain(surface, virtualId)
            info.setParent(surface)
            info.packageName = context.packageName
            info.className = n.className
            if (n.role == 9L) {
                // A text field: its text, under its name's hint, as EditText.
                info.isEditable = true
                info.text = n.value.ifEmpty { null }
                info.hintText = n.label
                info.isShowingHintText = n.value.isEmpty()
                info.isPassword = n.states and 128 != 0L
            } else {
                info.text = n.label.ifEmpty { null }
                if (n.value.isNotEmpty()) info.stateDescription = n.value
            }
            if (n.hint.isNotEmpty()) info.tooltipText = n.hint
            val d = density
            val box = Rect((n.x * d).toInt(), (n.y * d).toInt(), ((n.x + n.w) * d).toInt(), ((n.y + n.h) * d).toInt())
            val loc = IntArray(2)
            surface.getLocationOnScreen(loc)
            box.offset(loc[0], loc[1])
            info.setBoundsInScreen(box)
            info.isEnabled = n.states and 2 == 0L
            info.isChecked = n.states and 4 != 0L
            info.isCheckable = n.role in longArrayOf(4, 5, 6, 31)
            info.isSelected = n.states and 16 != 0L
            info.isHeading = n.role == 43L
            info.isVisibleToUser = true
            info.isFocusable = true
            info.isAccessibilityFocused = accessFocus == virtualId
            info.addAction(if (accessFocus == virtualId) AccessibilityNodeInfo.AccessibilityAction.ACTION_CLEAR_ACCESSIBILITY_FOCUS
                else AccessibilityNodeInfo.AccessibilityAction.ACTION_ACCESSIBILITY_FOCUS)
            if (n.actions and 1 != 0L) {
                info.isClickable = true
                info.addAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_CLICK)
            }
            if (n.actions and 4 != 0L) info.addAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_SCROLL_FORWARD)
            if (n.actions and 8 != 0L) info.addAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_SCROLL_BACKWARD)
            return info
        }

        override fun performAction(virtualId: Int, action: Int, args: Bundle?): Boolean {
            val name = when (action) {
                AccessibilityNodeInfo.ACTION_CLICK -> "press"
                AccessibilityNodeInfo.ACTION_SCROLL_FORWARD -> "increment"
                AccessibilityNodeInfo.ACTION_SCROLL_BACKWARD -> "decrement"
                AccessibilityNodeInfo.ACTION_ACCESSIBILITY_FOCUS -> {
                    if (accessFocus != virtualId) {
                        val old = accessFocus
                        accessFocus = virtualId
                        if (old != View.NO_ID) sendVirtual(old, AccessibilityEvent.TYPE_VIEW_ACCESSIBILITY_FOCUS_CLEARED)
                        sendVirtual(virtualId, AccessibilityEvent.TYPE_VIEW_ACCESSIBILITY_FOCUSED)
                    }
                    return true
                }
                AccessibilityNodeInfo.ACTION_CLEAR_ACCESSIBILITY_FOCUS -> {
                    if (accessFocus == virtualId) {
                        accessFocus = View.NO_ID
                        sendVirtual(virtualId, AccessibilityEvent.TYPE_VIEW_ACCESSIBILITY_FOCUS_CLEARED)
                    }
                    return true
                }
                else -> return false
            }
            val r = Go.ask(Event.accessibilityAction(wid, virtualId.toLong(), name)) ?: return false
            return AccessibilityActionReply(r).handled
        }
    }

    companion object {
        private val keyNames = mapOf(
            KeyEvent.KEYCODE_ENTER to "Enter", KeyEvent.KEYCODE_ESCAPE to "Escape", KeyEvent.KEYCODE_DEL to "Backspace",
            KeyEvent.KEYCODE_TAB to "Tab", KeyEvent.KEYCODE_SPACE to "Space", KeyEvent.KEYCODE_FORWARD_DEL to "Delete",
            KeyEvent.KEYCODE_MOVE_HOME to "Home", KeyEvent.KEYCODE_MOVE_END to "End", KeyEvent.KEYCODE_PAGE_UP to "PageUp",
            KeyEvent.KEYCODE_PAGE_DOWN to "PageDown", KeyEvent.KEYCODE_DPAD_LEFT to "ArrowLeft",
            KeyEvent.KEYCODE_DPAD_RIGHT to "ArrowRight", KeyEvent.KEYCODE_DPAD_UP to "ArrowUp", KeyEvent.KEYCODE_DPAD_DOWN to "ArrowDown",
        )
    }
}

/** An element of native UI, as Go encodes it (AccessibilityTree). */
class Node(
    val id: Long, val parent: Long, val role: Long, val label: String, val value: String, val hint: String,
    val x: Double, val y: Double, val w: Double, val h: Double, val states: Long, val actions: Long,
) {
    val className: String
        get() = when (role) {
            2L, 31L, 40L, 18L -> "android.widget.Button"
            4L -> "android.widget.CheckBox"
            5L -> "android.widget.RadioButton"
            6L -> "android.widget.Switch"
            7L -> "android.widget.SeekBar"
            8L -> "android.widget.ProgressBar"
            9L -> "android.widget.EditText"
            10L -> "android.widget.ImageView"
            else -> "android.view.View"
        }

    companion object {
        fun decode(b: ByteArray): List<Node> {
            val r = WireReader(b)
            val out = ArrayList<Node>()
            while (true) {
                val n = Node(r.int(), r.int(), r.int(), r.string(), r.string(), r.string(),
                    r.float(), r.float(), r.float(), r.float(), r.int(), r.int())
                r.int() // the level
                if (n.id == 0L) break
                // Groups without a name hold others, which TalkBack reads.
                if (n.role == 0L && n.label.isEmpty() && n.value.isEmpty()) continue
                out.add(n)
            }
            return out
        }
    }
}
