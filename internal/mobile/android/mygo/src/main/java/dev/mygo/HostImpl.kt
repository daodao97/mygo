// The calls Go makes of the host (HostCalls), on the UI thread.

package dev.mygo

import android.Manifest
import android.app.Activity
import android.app.AlertDialog
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.UiModeManager
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.content.MutableContextWrapper
import android.content.pm.ActivityInfo
import android.content.pm.PackageManager
import android.content.res.Configuration
import android.graphics.BitmapFactory
import android.graphics.Color
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.net.Uri
import android.os.BatteryManager
import android.os.Build
import android.os.PowerManager
import android.provider.OpenableColumns
import android.provider.Settings
import android.system.Os
import android.text.format.DateFormat
import android.util.Log
import android.view.HapticFeedbackConstants
import android.view.View
import android.view.WindowManager
import android.webkit.MimeTypeMap
import android.widget.FrameLayout
import androidx.core.content.FileProvider
import androidx.core.view.ViewCompat
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.core.view.WindowInsetsCompat
import java.io.File
import java.util.Calendar
import java.util.Locale

object HostImpl : HostCalls {
    /** The Activity showing the app, null while none does. */
    var activity: Activity? = null
        private set
    lateinit var app: Context
        private set

    /** The context of the views, which follows the Activity. */
    private lateinit var context: MutableContextWrapper
    lateinit var root: FrameLayout
        private set
    val windows = HashMap<Long, HostWindow>()
    private var started = false
    /** Whether Android made the Activity again after ending the process. */
    var restoring = false

    // What covers the screen's edges, in pixels: the system's bars and
    // the cutout, and the keyboard.
    var bars = androidx.core.graphics.Insets.NONE
        private set
    var ime = androidx.core.graphics.Insets.NONE
        private set

    private const val ASK_NOTIFICATIONS = 0x6d7a
    private var pendingNotes = ArrayList<NotifyCall>()

    /** Readies what outlives Activities, from the first thing to start the process. */
    private fun prepare(c: Context) {
        if (::app.isInitialized) return
        run {
            app = c.applicationContext
            context = MutableContextWrapper(c)
            root = FrameLayout(context)
            ViewCompat.setOnApplyWindowInsetsListener(root) { _, insets ->
                bars = insets.getInsets(WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.displayCutout())
                ime = insets.getInsets(WindowInsetsCompat.Type.ime())
                Navigation.insetsChanged()
                insets
            }
            root.addOnLayoutChangeListener { _, l, t, r, b, ol, ot, or, ob ->
                if (r - l != or - ol || b - t != ob - ot) for (w in windows.values) w.reportSize()
            }
        }
    }

    fun attach(a: Activity) {
        activity = a
        prepare(a)
        context.baseContext = a
        (root.parent as? android.view.ViewGroup)?.removeView(root)
        root.setBackgroundColor(themeColor(android.R.attr.colorBackground))
        a.setContentView(root)
        ViewCompat.requestApplyInsets(root)
    }

    fun detach(a: Activity) {
        if (activity === a) activity = null
    }

    /** Starts Go with no window to show, for what runs in the background. */
    fun startHeadless(c: Context) {
        if (started) return
        DevLibrary.find(c)
        prepare(c)
        startGo(c, null)
    }

    /** Starts Go once per process, with the environment a launch passes (mygo dev). */
    fun startGo(a: Context, intent: Intent?) {
        if (started) return
        started = true
        val files = a.filesDir.absolutePath
        val cache = a.cacheDir.absolutePath
        // Android sets none of these, which os.UserHomeDir, UserConfigDir,
        // UserCacheDir and TempDir read.
        app.registerReceiver(object : android.content.BroadcastReceiver() {
            override fun onReceive(c: Context, i: Intent) {
                // As the system's zone changes, the default of Java keeps the old one.
                java.util.TimeZone.setDefault(null)
                Go.send(Event.timeZoneChanged(java.util.TimeZone.getDefault().id))
            }
        }, android.content.IntentFilter(Intent.ACTION_TIMEZONE_CHANGED))
        Os.setenv("HOME", files, true)
        Os.setenv("XDG_CONFIG_HOME", files, true)
        Os.setenv("XDG_CACHE_HOME", cache, true)
        Os.setenv("TMPDIR", cache, true)
        // Only mygo dev's, in a debug build: the Activity is exported, and any
        // app could otherwise set a proxy for this one's traffic.
        val debuggable = a.applicationInfo.flags and android.content.pm.ApplicationInfo.FLAG_DEBUGGABLE != 0
        if (debuggable) {
            intent?.getStringExtra("mygo.env")?.split('\n')?.forEach { line ->
                val i = line.indexOf('=')
                if (i > 0) Os.setenv(line.substring(0, i), line.substring(i + 1), true)
            }
        }
        registerPlugins()
        // The first launch of a version copies the app's resources, which may
        // be many: off the UI thread, which Android watches. Events sent
        // meanwhile wait for the backend.
        val ctx = a.applicationContext
        Thread {
            val dir = Resources.extract(ctx)
            // Not by a view's post: with no Activity, the root is in no window
            // and would hold it until one comes.
            android.os.Handler(android.os.Looper.getMainLooper()).post {
                Os.setenv("MYGO_RESOURCES", dir, true)
                MyGoNative.start()
            }
        }.start()
    }

    fun themeColor(attr: Int): Int {
        val a = (activity ?: context).obtainStyledAttributes(intArrayOf(attr))
        val c = a.getColor(0, Color.WHITE)
        a.recycle()
        return c
    }

    val density get() = app.resources.displayMetrics.density

    /** The window in front: the visible window highest on screen. */
    fun front(): HostWindow? = windows.values.filter { it.view.isShown }.maxByOrNull { w ->
        var depth = 0
        var v: View? = w.view
        while (v != null && v !== root) {
            val parent = v.parent as? android.view.ViewGroup ?: break
            depth = depth * 64 + parent.indexOfChild(v)
            v = parent
        }
        depth
    }

    /** Hears the system's back, while something in the app takes it. */
    var backCallback: androidx.activity.OnBackPressedCallback? = null

    /** The system's back goes to the app, or leaves it, as predictive back shows beforehand. */
    fun updateBack() {
        val f = front()
        backCallback?.isEnabled = Navigation.canPop() || (f as? SurfaceWindow)?.backAvailable == true || (f as? WebWindow)?.web?.canGoBack() == true
    }

    /** Android's back: the window in front's page goes back, or the app takes it. */
    fun back(): Boolean {
        if (Navigation.back()) return true
        val w = front() ?: return false
        val r = Go.ask(Event.backPressed(w.id)) ?: return false
        return BackPressedReply(r).handled
    }

    override fun timeZone(): TimeZoneReply {
        val r = TimeZoneReply()
        r.id = java.util.TimeZone.getDefault().id
        return r
    }

    /**
     * Whether the app has something on screen in the Activity, which the
     * splash waits for (MyGoActivity.holdSplash).
     */
    @Volatile var drew = false

    /**
     * A new Activity shows the app's windows, and waits anew for something
     * on screen (drew), as for none yet: a surface Go draws on is made anew
     * with it, empty until Go's first frame on it, while a web view keeps
     * the page it showed. The Activity is not attached yet: what shows is
     * what is visible under the root.
     */
    fun activityCreated() {
        val shown = windows.values.filter { w ->
            generateSequence(w.view) { it.parent as? View }.takeWhile { it !== root }.all { it.visibility == View.VISIBLE }
        }
        drew = shown.isNotEmpty() && shown.all { it is WebWindow && it.pageShown }
    }

    /** Go presented its first frame on a window's surface, as it was made or made anew. */
    override fun firstFrame(c: FirstFrameCall) {
        drew = true
        surface(c.id)?.drawn()
    }

    override fun appInfo(): AppInfoReply {
        val r = AppInfoReply()
        val pm = app.packageManager
        val info = pm.getPackageInfo(app.packageName, 0)
        r.name = app.applicationInfo.loadLabel(pm).toString()
        r.version = info.versionName ?: ""
        r.identifier = app.packageName
        r.filesDir = app.filesDir.absolutePath
        r.cacheDir = app.cacheDir.absolutePath
        r.tempDir = app.cacheDir.absolutePath
        r.documentsDir = (app.getExternalFilesDir(null) ?: app.filesDir).absolutePath
        r.resourcesDir = Resources.dir(app).absolutePath
        val locales = app.resources.configuration.locales
        r.languages = (0 until locales.size()).map { locales[it].toLanguageTag() }
        val format = Locale.getDefault(Locale.Category.FORMAT)
        r.regionLocale = format.toLanguageTag()
        r.firstWeekday = (Calendar.getInstance(format).firstDayOfWeek - 1).toLong()
        r.hourCycle = if (DateFormat.is24HourFormat(app)) 24L else 12L
        r.system = "Android"
        r.systemVersion = Build.VERSION.RELEASE
        r.model = Build.MODEL
        r.restoring = restoring
        r.background = activity == null // the system started it for a task
        return r
    }

    override fun theme(): ThemeReply {
        val r = ThemeReply()
        val c = (activity ?: app).resources.configuration
        r.dark = c.uiMode and Configuration.UI_MODE_NIGHT_MASK == Configuration.UI_MODE_NIGHT_YES
        val accent = if (Build.VERSION.SDK_INT >= 31) app.getColor(android.R.color.system_accent1_600) else themeColor(android.R.attr.colorAccent)
        r.accent = argbToRGBA(accent)
        r.reduceMotion = Settings.Global.getFloat(app.contentResolver, Settings.Global.ANIMATOR_DURATION_SCALE, 1f) == 0f
        if (Build.VERSION.SDK_INT >= 34) {
            r.increaseContrast = (app.getSystemService(Context.UI_MODE_SERVICE) as UiModeManager).contrast > 0.5f
        }
        r.textScale = c.fontScale.toDouble()
        return r
    }

    override fun setAppearance(c: SetAppearanceCall) {
        if (Build.VERSION.SDK_INT < 31) return
        val ui = app.getSystemService(Context.UI_MODE_SERVICE) as UiModeManager
        ui.setApplicationNightMode(
            when (c.source) {
                "dark" -> UiModeManager.MODE_NIGHT_YES
                "light" -> UiModeManager.MODE_NIGHT_NO
                else -> UiModeManager.MODE_NIGHT_AUTO
            }
        )
    }

    override fun screen(): ScreenReply {
        val r = ScreenReply()
        val m = app.resources.displayMetrics
        r.width = m.widthPixels / m.density.toDouble()
        r.height = m.heightPixels / m.density.toDouble()
        r.scale = m.density.toDouble()
        return r
    }

    override fun log(c: LogCall) {
        Log.println(if (c.error) Log.ERROR else Log.INFO, "mygo", c.line)
    }

    // Windows.

    override fun createWindow(c: CreateWindowCall): CreateWindowReply {
        val r = CreateWindowReply()
        try {
            val w = when {
                c.surface -> SurfaceWindow(context, c.id)
                c.viewType.isNotEmpty() -> PluginViewWindow(context, c.id, PluginHost.makeView(c.viewType, context, c.id, c.viewArgs)
                    ?: throw IllegalArgumentException("no view ${c.viewType} in this app"))
                else -> WebWindow(context, c)
            }
            if (c.hasBackground) {
                w.fill = rgbaToARGB(c.background)
                w.setBackground(w.fill)
            }
            windows[c.id] = w
            if (c.parent != 0L) surface(c.parent)?.hold(w)
        } catch (e: Throwable) {
            r.error = "mygo: ${e.message ?: e.toString()}"
        }
        return r
    }

    override fun placeViews(c: PlaceViewsCall) { surface(c.id)?.placeViews(c.places) }

    override fun showWindow(c: ShowWindowCall) {
        val w = windows[c.id] ?: return
        Navigation.show(w)
        w.insetsChanged()
        updateBack()
    }

    override fun hideWindow(c: HideWindowCall) {
        val w = windows[c.id] ?: return
        Navigation.hide(w)
        updateBack()
    }

    override fun closeWindow(c: CloseWindowCall) {
        val w = windows.remove(c.id) ?: return
        val holder = w.holder
        if (holder != null) {
            holder.release(w)
            w.close()
            return
        }
        Navigation.hide(w)
        Navigation.forget(w)
        w.close()
    }

    override fun setTitle(c: SetTitleCall) { windows[c.id]?.let { Navigation.setTitle(it, c.title) } }
    override fun createTabs(c: CreateTabsCall) = Navigation.createTabs(c.id, context)
    override fun addTab(c: AddTabCall) { windows[c.window]?.let { Navigation.addTab(c.id, it, c.title, c.icon) } }
    override fun showTabs(c: ShowTabsCall) = Navigation.showTabs(c.id)
    override fun selectTab(c: SelectTabCall) { Navigation.tabs[c.id]?.select(c.index.toInt()) }
    override fun setTabBadge(c: SetTabBadgeCall) = Navigation.setBadge(c.id, c.index.toInt(), c.badge)
    override fun setTabTitle(c: SetTabTitleCall) = Navigation.setTabTitle(c.id, c.index.toInt(), c.title)
    override fun closeTabs(c: CloseTabsCall) = Navigation.closeTabs(c.id)
    override fun pushWindow(c: PushWindowCall) {
        val p = windows[c.parent] ?: return
        val w = windows[c.child] ?: return
        Navigation.push(w, p, c.title, c.animated)
        updateBack()
    }
    override fun setStack(c: SetStackCall): SetStackReply {
        windows[c.id]?.let { Navigation.setStack(it, c.titles) }
        return SetStackReply()
    }
    override fun stackShown(c: StackShownCall) {}
    override fun popWindow(c: PopWindowCall) {
        windows[c.id]?.let { Navigation.pop(it, c.animated) }
        updateBack()
    }

    override fun setBackground(c: SetBackgroundCall) {
        val w = windows[c.id] ?: return
        val argb = rgbaToARGB(c.color)
        w.fill = argb
        w.setBackground(argb)
        Navigation.backgroundChanged(w)
    }

    override fun focus(c: FocusCall) {
        windows[c.id]?.view?.requestFocus()
    }

    // Pages.

    private fun web(id: Long) = windows[id] as? WebWindow

    override fun loadURL(c: LoadURLCall) { web(c.id)?.load(c.url) }
    override fun loadHTML(c: LoadHTMLCall) { web(c.id)?.loadHTML(c.html, c.baseURL) }
    override fun reload(c: ReloadCall) {
        val w = web(c.id) ?: return
        if (c.ignoreCache) w.web.clearCache(false)
        w.web.reload()
    }
    override fun stopLoading(c: StopLoadingCall) { web(c.id)?.web?.stopLoading() }
    override fun goBack(c: GoBackCall) { web(c.id)?.web?.goBack() }
    override fun goForward(c: GoForwardCall) { web(c.id)?.web?.goForward() }
    override fun eval(c: EvalCall) { web(c.id)?.web?.evaluateJavascript(c.js, null) }
    override fun callAsync(c: CallAsyncCall) {
        val w = web(c.id)
        if (w == null) {
            Go.send(Event.asyncResult(c.req, "", "mygo: the window is closed"))
            return
        }
        w.callAsync(c.req, c.body)
    }
    override fun setZoom(c: SetZoomCall) { web(c.id)?.web?.settings?.textZoom = (c.factor * 100).toInt() }
    override fun setUserAgent(c: SetUserAgentCall) { web(c.id)?.setUserAgent(c.userAgent) }
    override fun setUserScripts(c: SetUserScriptsCall) { web(c.id)?.setScripts(c.scripts, c.scriptFlags) }
    override fun capturePage(c: CapturePageCall) {
        val w = web(c.id)
        if (w == null) {
            Go.send(Event.captured(c.req, ByteArray(0), "mygo: the window is closed"))
            return
        }
        w.capture(c.req)
    }
    override fun userAgent(c: UserAgentCall): UserAgentReply {
        val r = UserAgentReply()
        r.userAgent = web(c.id)?.web?.settings?.userAgentString ?: ""
        return r
    }

    override fun schemeRespond(c: SchemeRespondCall) = Schemes.respond(c)
    override fun schemeData(c: SchemeDataCall) = Schemes.data(c.req, c.data)
    override fun schemeFinish(c: SchemeFinishCall) = Schemes.finish(c.req, null)
    override fun schemeFail(c: SchemeFailCall) = Schemes.finish(c.req, c.message)

    // Dialogs.

    override fun messageBox(c: MessageBoxCall) {
        val a = activity ?: run {
            Go.send(Event.messageBoxResult(c.req, c.cancelID))
            return
        }
        val title = if (c.title.isEmpty()) c.message else c.title
        var message = if (c.title.isEmpty()) c.detail else c.message
        if (c.title.isNotEmpty() && c.detail.isNotEmpty()) message += "\n\n" + c.detail
        var answered = false
        fun answer(i: Long) {
            if (!answered) {
                answered = true
                Go.send(Event.messageBoxResult(c.req, i))
            }
        }
        val b = AlertDialog.Builder(a).setTitle(title)
        if (message.isNotEmpty()) b.setMessage(message)
        // Android's dialogs take three buttons: the default one positive,
        // the cancel one negative, another neutral; more make a list.
        val buttons = c.buttons
        if (buttons.size <= 3) {
            val order = buttons.indices.sortedBy { if (it.toLong() == c.defaultID) 0 else if (it.toLong() == c.cancelID) 2 else 1 }
            order.forEachIndexed { slot, i ->
                val l = { _: android.content.DialogInterface, _: Int -> answer(i.toLong()) }
                when {
                    slot == 0 -> b.setPositiveButton(buttons[i], l)
                    i.toLong() == c.cancelID || slot == 2 -> b.setNegativeButton(buttons[i], l)
                    else -> b.setNeutralButton(buttons[i], l)
                }
            }
        } else {
            b.setItems(buttons.toTypedArray()) { _, i -> answer(i.toLong()) }
        }
        b.setOnCancelListener { answer(c.cancelID) }
        b.show()
    }

    override fun openFiles(c: OpenFilesCall) {
        val a = activity ?: run {
            Go.send(Event.filesResult(c.req, emptyList(), ""))
            return
        }
        val code = nextResultCode()
        picks[code] = c.req
        val intent = if (c.directories) {
            Intent(Intent.ACTION_OPEN_DOCUMENT_TREE)
        } else {
            Intent(Intent.ACTION_OPEN_DOCUMENT).apply {
                addCategory(Intent.CATEGORY_OPENABLE)
                val types = c.extensions.mapNotNull { MimeTypeMap.getSingleton().getMimeTypeFromExtension(it.lowercase()) }.distinct()
                type = if (types.size == 1) types[0] else "*/*"
                if (types.size > 1) putExtra(Intent.EXTRA_MIME_TYPES, types.toTypedArray())
                putExtra(Intent.EXTRA_ALLOW_MULTIPLE, c.multiple)
            }
        }
        @Suppress("DEPRECATION")
        a.startActivityForResult(intent, code)
    }

    fun activityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        if (PluginHost.activityResult(requestCode, resultCode, data)) return
        saves.remove(requestCode)?.let { (req, from) ->
            val uri = data?.data
            if (resultCode != Activity.RESULT_OK || uri == null) {
                Go.send(Event.filesResult(req, emptyList(), ""))
                return
            }
            Thread {
                try {
                    app.contentResolver.openOutputStream(uri)!!.use { out -> File(from).inputStream().use { it.copyTo(out) } }
                    Go.send(Event.filesResult(req, listOf(uri.toString()), ""))
                } catch (e: Exception) {
                    Go.send(Event.filesResult(req, emptyList(), e.toString()))
                }
            }.start()
            return
        }
        val req = picks.remove(requestCode)
        if (req != null) {
            if (resultCode != Activity.RESULT_OK || data == null) {
                Go.send(Event.filesResult(req, emptyList(), ""))
                return
            }
            val uris = ArrayList<Uri>()
            data.clipData?.let { clip -> for (i in 0 until clip.itemCount) uris.add(clip.getItemAt(i).uri) }
            if (uris.isEmpty()) data.data?.let { uris.add(it) }
            // Copies the app can read by path: phones give apps no paths
            // of other apps' documents.
            Thread {
                try {
                    val dir = File(app.cacheDir, "picked/${System.nanoTime()}").apply { mkdirs() }
                    val paths = uris.map { copyToFile(it, dir).absolutePath }
                    Go.send(Event.filesResult(req, paths, ""))
                } catch (e: Exception) {
                    Go.send(Event.filesResult(req, emptyList(), e.toString()))
                }
            }.start()
        } else {
            WebWindow.activityResult(requestCode, resultCode, data)
        }
    }

    fun copyToFile(uri: Uri, dir: File): File {
        var name = "file"
        app.contentResolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use {
            if (it.moveToFirst()) name = it.getString(0) ?: name
        }
        val f = File(dir, name.replace('/', '_'))
        app.contentResolver.openInputStream(uri)!!.use { input -> f.outputStream().use { input.copyTo(it) } }
        return f
    }

    override fun shareFile(c: ShareFileCall) {
        val a = activity ?: return
        val f = File(c.path)
        val uri = FileProvider.getUriForFile(app, app.packageName + ".mygo.files", f)
        val mime = MimeTypeMap.getSingleton().getMimeTypeFromExtension(f.extension.lowercase()) ?: "application/octet-stream"
        val send = Intent(Intent.ACTION_SEND).setType(mime).putExtra(Intent.EXTRA_STREAM, uri).addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        a.startActivity(Intent.createChooser(send, null))
    }

    // The system.

    private val clipboard get() = app.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager

    override fun clipboardText(): ClipboardTextReply {
        val r = ClipboardTextReply()
        val clip = clipboard.primaryClip
        if (clip != null && clip.itemCount > 0) {
            r.text = clip.getItemAt(0).coerceToText(app).toString()
            r.ok = true
        }
        return r
    }

    override fun setClipboardText(c: SetClipboardTextCall) {
        clipboard.setPrimaryClip(ClipData.newPlainText(null, c.text))
    }

    override fun openURL(c: OpenURLCall): OpenURLReply {
        val r = OpenURLReply()
        try {
            val i = Intent(Intent.ACTION_VIEW, Uri.parse(c.url)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            (activity ?: app).startActivity(i)
            r.ok = true
        } catch (e: Exception) {
            r.ok = false
        }
        return r
    }

    override fun secretGet(c: SecretGetCall): SecretGetReply = Secrets.get(app, c)
    override fun secretSet(c: SecretSetCall): SecretSetReply = Secrets.set(app, c)
    override fun secretDelete(c: SecretDeleteCall): SecretDeleteReply = Secrets.delete(app, c)

    override fun notify(c: NotifyCall) {
        val a = activity
        if (Build.VERSION.SDK_INT >= 33 && app.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            if (a == null) {
                Go.send(Event.notifyDone(c.req, "mygo: notifications are not allowed"))
                return
            }
            pendingNotes.add(c)
            if (pendingNotes.size == 1) a.requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), ASK_NOTIFICATIONS)
            return
        }
        post(c)
    }

    fun permissionsResult(requestCode: Int, results: IntArray) {
        if (PluginHost.permissionsResult(requestCode, results)) return
        if (requestCode == ASK_NOTIFICATIONS) {
            val granted = results.isNotEmpty() && results[0] == PackageManager.PERMISSION_GRANTED
            val notes = pendingNotes
            pendingNotes = ArrayList()
            for (c in notes) if (granted) post(c) else Go.send(Event.notifyDone(c.req, "mygo: notifications are not allowed"))
        } else {
            WebWindow.permissionsResult(requestCode, results)
        }
    }

    private fun post(c: NotifyCall) {
        val nm = app.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        val channel = if (c.silent) "mygo.silent" else "mygo"
        if (nm.getNotificationChannel(channel) == null) {
            val ch = NotificationChannel(channel, appInfo().name, if (c.silent) NotificationManager.IMPORTANCE_LOW else NotificationManager.IMPORTANCE_DEFAULT)
            nm.createNotificationChannel(ch)
        }
        val id = c.notificationID.ifEmpty { System.nanoTime().toString() }
        val open = Intent(app, MyGoActivity::class.java).putExtra("mygo.notification", id).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP)
        val pi = PendingIntent.getActivity(app, id.hashCode(), open, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        val b = android.app.Notification.Builder(app, channel)
            .setSmallIcon(app.applicationInfo.icon)
            .setContentTitle(c.title)
            .setContentText(c.body)
            .setContentIntent(pi)
            .setAutoCancel(true)
        if (c.subtitle.isNotEmpty()) b.setSubText(c.subtitle)
        if (c.group.isNotEmpty()) b.setGroup(c.group)
        nm.notify(id, 0, b.build())
        Go.send(Event.notifyDone(c.req, ""))
    }

    override fun removeNotification(c: RemoveNotificationCall) {
        (app.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager).cancel(c.notificationID, 0)
    }

    override fun removeAllNotifications() {
        (app.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager).cancelAll()
    }

    override fun keepAwake(c: KeepAwakeCall) {
        val w = activity?.window ?: return
        if (c.on) w.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON) else w.clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
    }

    // Network and power.

    private val connectivity get() = app.getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
    private var watchingNetwork = false

    private fun networkReply(): NetworkStatusReply {
        val r = NetworkStatusReply()
        val caps = connectivity.getNetworkCapabilities(connectivity.activeNetwork)
        r.online = caps?.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET) == true
        r.metered = connectivity.isActiveNetworkMetered
        return r
    }

    override fun networkStatus(): NetworkStatusReply = networkReply()

    override fun watchNetwork() {
        if (watchingNetwork) return
        watchingNetwork = true
        val send = {
            val r = networkReply()
            Go.send(Event.networkChanged(r.online, r.metered))
        }
        connectivity.registerDefaultNetworkCallback(object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(n: Network) = send()
            override fun onLost(n: Network) = send()
            override fun onCapabilitiesChanged(n: Network, caps: NetworkCapabilities) = send()
        })
    }

    override fun battery(): BatteryReply {
        val r = BatteryReply()
        val bm = app.getSystemService(Context.BATTERY_SERVICE) as BatteryManager
        r.level = bm.getIntProperty(BatteryManager.BATTERY_PROPERTY_CAPACITY) / 100.0
        r.charging = bm.isCharging
        r.lowPower = (app.getSystemService(Context.POWER_SERVICE) as PowerManager).isPowerSaveMode
        return r
    }

    // The system's bars and the screen.

    private var barStyle = "auto"
    private var barsHidden = false
    private var orientationAsked = ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED

    override fun setSystemBars(c: SetSystemBarsCall) {
        barStyle = c.style
        barsHidden = c.hidden
        applyBars()
    }

    /** Applies what the app asked of the system's bars, to the Activity showing it. */
    fun applyBars() {
        val a = activity ?: return
        val ctl = WindowCompat.getInsetsController(a.window, a.window.decorView)
        val darkMode = a.resources.configuration.uiMode and Configuration.UI_MODE_NIGHT_MASK == Configuration.UI_MODE_NIGHT_YES
        val darkIcons = when (barStyle) {
            "dark" -> true
            "light" -> false
            else -> !darkMode
        }
        ctl.isAppearanceLightStatusBars = darkIcons
        ctl.isAppearanceLightNavigationBars = darkIcons
        ctl.systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
        if (barsHidden) ctl.hide(WindowInsetsCompat.Type.systemBars()) else ctl.show(WindowInsetsCompat.Type.systemBars())
        a.requestedOrientation = orientationAsked
    }

    override fun setOrientations(c: SetOrientationsCall) {
        val m = c.mask.toInt()
        val portrait = m and 3 != 0
        val landscape = m and 12 != 0
        orientationAsked = when {
            m == 0 -> ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED
            portrait && landscape -> ActivityInfo.SCREEN_ORIENTATION_FULL_USER
            m == 1 -> ActivityInfo.SCREEN_ORIENTATION_PORTRAIT
            m == 2 -> ActivityInfo.SCREEN_ORIENTATION_REVERSE_PORTRAIT
            m == 3 -> ActivityInfo.SCREEN_ORIENTATION_SENSOR_PORTRAIT
            m == 4 -> ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE
            m == 8 -> ActivityInfo.SCREEN_ORIENTATION_REVERSE_LANDSCAPE
            landscape -> ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE
            else -> ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED
        }
        activity?.requestedOrientation = orientationAsked
    }

    override fun share(c: ShareCall) {
        val a = activity ?: run {
            Go.send(Event.shareResult(c.req, "mygo: no window to share from"))
            return
        }
        val text = listOf(c.text, c.url).filter { it.isNotEmpty() }.joinToString("\n")
        val uris = ArrayList(c.paths.map { FileProvider.getUriForFile(app, app.packageName + ".mygo.files", File(it)) })
        val send = when {
            uris.size > 1 -> Intent(Intent.ACTION_SEND_MULTIPLE).setType("*/*").putParcelableArrayListExtra(Intent.EXTRA_STREAM, uris)
            uris.size == 1 -> {
                val mime = MimeTypeMap.getSingleton().getMimeTypeFromExtension(File(c.paths[0]).extension.lowercase()) ?: "application/octet-stream"
                Intent(Intent.ACTION_SEND).setType(mime).putExtra(Intent.EXTRA_STREAM, uris[0])
            }
            else -> Intent(Intent.ACTION_SEND).setType("text/plain")
        }
        if (uris.isNotEmpty()) {
            send.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            send.clipData = ClipData.newRawUri(null, uris[0]).also { d -> uris.drop(1).forEach { d.addItem(ClipData.Item(it)) } }
        }
        if (text.isNotEmpty()) send.putExtra(Intent.EXTRA_TEXT, text)
        try {
            a.startActivity(Intent.createChooser(send, null))
            Go.send(Event.shareResult(c.req, ""))
        } catch (e: Exception) {
            Go.send(Event.shareResult(c.req, e.toString()))
        }
    }

    // What the pickers on screen answer, by their request codes: one tapped
    // twice opens two, each answering its own.
    private val picks = HashMap<Int, Long>()
    private val saves = HashMap<Int, Pair<Long, String>>()
    private var lastResultCode = 0

    private fun nextResultCode(): Int {
        lastResultCode = (lastResultCode + 1) and 0xff
        return 0x6e00 + lastResultCode
    }

    override fun saveFile(c: SaveFileCall) {
        val a = activity ?: run {
            Go.send(Event.filesResult(c.req, emptyList(), "mygo: no window to save from"))
            return
        }
        val code = nextResultCode()
        saves[code] = Pair(c.req, c.path)
        val mime = MimeTypeMap.getSingleton().getMimeTypeFromExtension(File(c.name).extension.lowercase()) ?: "application/octet-stream"
        val i = Intent(Intent.ACTION_CREATE_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE).setType(mime).putExtra(Intent.EXTRA_TITLE, c.name)
        @Suppress("DEPRECATION")
        a.startActivityForResult(i, code)
    }

    override fun clipboardImage(): ClipboardImageReply {
        val r = ClipboardImageReply()
        val clip = clipboard.primaryClip ?: return r
        if (clip.itemCount == 0) return r
        val uri = clip.getItemAt(0).uri ?: return r
        val mime = app.contentResolver.getType(uri) ?: return r
        if (!mime.startsWith("image/")) return r
        try {
            val bmp = app.contentResolver.openInputStream(uri)?.use { BitmapFactory.decodeStream(it) } ?: return r
            val out = java.io.ByteArrayOutputStream()
            bmp.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, out)
            r.png = out.toByteArray()
        } catch (_: Exception) {
        }
        return r
    }

    override fun setClipboardImage(c: SetClipboardImageCall) {
        try {
            val dir = File(app.cacheDir, "clipboard").apply { mkdirs() }
            val f = File(dir, "image.png")
            f.writeBytes(c.png)
            val uri = FileProvider.getUriForFile(app, app.packageName + ".mygo.files", f)
            clipboard.setPrimaryClip(ClipData.newUri(app.contentResolver, "image", uri))
        } catch (_: Exception) {
        }
    }

    /** A menu shown by the app, as a context menu, at a point of the window. */
    override fun popupMenu(c: PopupMenuCall) {
        val w = surface(c.id)
        val a = activity
        val host = w?.view ?: front()?.view
        if (a == null || host == null) {
            Go.send(Event.menuPicked(c.req, 0))
            return
        }
        // Where it shows: a view at the point, gone as the menu goes.
        val anchor = View(a)
        val d = density
        val lp = FrameLayout.LayoutParams(1, 1)
        lp.leftMargin = (c.x * d).toInt()
        lp.topMargin = (c.y * d).toInt()
        root.addView(anchor, lp)
        val menu = android.widget.PopupMenu(a, anchor)
        val r = WireReader(c.items)
        var group = 0
        var order = 0
        while (!r.done()) {
            val id = r.int()
            val label = r.string()
            val enabled = r.bool()
            val checked = r.bool()
            if (id == 0L) {
                group++
                continue
            }
            val item = menu.menu.add(group, id.toInt(), order++, label)
            item.isEnabled = enabled
            if (checked) {
                item.isCheckable = true
                item.isChecked = true
            }
        }
        var picked = 0L
        menu.setOnMenuItemClickListener {
            picked = it.itemId.toLong()
            true
        }
        menu.setOnDismissListener {
            root.removeView(anchor)
            Go.send(Event.menuPicked(c.req, picked))
        }
        root.post { menu.show() }
    }

    override fun setBarItems(c: SetBarItemsCall) {
        windows[c.id]?.let { Navigation.setBarItems(it, c) }
    }

    override fun pluginCall(c: PluginCallCall) = PluginHost.call(c)

    // Sound, called on the audio's thread.
    override fun audioOpen(c: AudioOpenCall): AudioOpenReply = Sound.open(c)
    override fun audioWrite(c: AudioWriteCall): AudioWriteReply = Sound.write(c)
    override fun audioClose(): AudioCloseReply {
        Sound.close()
        return AudioCloseReply()
    }

    override fun haptic(c: HapticCall) {
        val v: View = front()?.view ?: root
        val k = when (c.kind) {
            "selection" -> HapticFeedbackConstants.CLOCK_TICK
            "light" -> HapticFeedbackConstants.KEYBOARD_TAP
            "heavy" -> HapticFeedbackConstants.LONG_PRESS
            "medium" -> HapticFeedbackConstants.CONTEXT_CLICK
            "success" -> if (Build.VERSION.SDK_INT >= 30) HapticFeedbackConstants.CONFIRM else HapticFeedbackConstants.VIRTUAL_KEY
            "warning", "error" -> if (Build.VERSION.SDK_INT >= 30) HapticFeedbackConstants.REJECT else HapticFeedbackConstants.LONG_PRESS
            else -> HapticFeedbackConstants.VIRTUAL_KEY
        }
        v.performHapticFeedback(k)
    }

    // Native UI.

    private fun surface(id: Long) = windows[id] as? SurfaceWindow

    override fun requestFrame(c: RequestFrameCall) { surface(c.id)?.requestFrame() }
    // Choreographer ticks at the display's rate: the content draws nothing
    // on the refreshes between the frames it asked for.
    override fun setFrameRate(c: SetFrameRateCall) {}
    override fun setTextInput(c: SetTextInputCall) { surface(c.id)?.setTextInput(c) }
    override fun setCursor(c: SetCursorCall) { surface(c.id)?.setCursor(c.cursor) }
    override fun accessibilityChanged(c: AccessibilityChangedCall) { surface(c.id)?.accessibilityChanged(c.focus) }
    override fun announce(c: AnnounceCall) { (front()?.view ?: root).announceForAccessibility(c.text) }
    override fun showMenu(c: ShowMenuCall) { surface(c.id)?.showMenu(c) }
    override fun hideMenu(c: HideMenuCall) { surface(c.id)?.hideMenu() }
    override fun setBackAvailable(c: SetBackAvailableCall) {
        surface(c.id)?.backAvailable = c.on
        updateBack()
    }
    override fun showMagnifier(c: ShowMagnifierCall) { surface(c.id)?.showMagnifier(c) }
    override fun hideMagnifier(c: HideMagnifierCall) { surface(c.id)?.hideMagnifier() }
    override fun textMetrics(c: TextMetricsCall): TextMetricsReply = Text.metrics(c)
    override fun measureText(c: MeasureTextCall): MeasureTextReply = Text.measure(c)
    override fun textAdvances(c: TextAdvancesCall): TextAdvancesReply = Text.advances(c)
    override fun measureTextRun(c: MeasureTextRunCall): MeasureTextRunReply = Text.measureRun(c)
    override fun drawText(c: DrawTextCall): DrawTextReply = Text.draw(c)
    override fun registerFont(c: RegisterFontCall): RegisterFontReply = Text.register(c)
}

/** 0xRRGGBBAA, as Go writes colors, to Android's 0xAARRGGBB. */
fun rgbaToARGB(v: Long): Int = (((v and 0xff) shl 24) or ((v ushr 8) and 0xffffff)).toInt()

fun argbToRGBA(c: Int): Long = ((c.toLong() and 0xffffff) shl 8) or ((c.toLong() ushr 24) and 0xff)
