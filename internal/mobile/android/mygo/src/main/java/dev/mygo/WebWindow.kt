// Windows showing web pages, in Android's WebView.

package dev.mygo

import android.Manifest
import android.app.Activity
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Bitmap
import android.graphics.Canvas
import android.net.Uri
import android.os.Handler
import android.os.Looper
import android.os.Message
import android.view.View
import android.view.ViewGroup
import android.webkit.JavascriptInterface
import android.webkit.PermissionRequest
import android.webkit.RenderProcessGoneDetail
import android.webkit.ValueCallback
import android.webkit.WebChromeClient
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.FrameLayout
import androidx.webkit.ScriptHandler
import androidx.webkit.WebViewCompat
import androidx.webkit.WebViewFeature
import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.io.InputStream
import java.util.concurrent.CountDownLatch
import java.util.concurrent.LinkedBlockingQueue
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicLong

class WebWindow(context: Context, c: CreateWindowCall) : HostWindow(context, c.id) {
    private val frame = FrameLayout(context)
    override val view: View get() = frame
    val web = WebView(context)
    private val schemes = c.schemes.toSet()
    private var scripts = ArrayList<ScriptHandler>()
    private var defaultUA = ""
    private var suffix = c.userAgentSuffix
    private val ui = Handler(Looper.getMainLooper())

    init {
        val s = web.settings
        s.javaScriptEnabled = true
        s.domStorageEnabled = true
        s.databaseEnabled = true
        s.mediaPlaybackRequiresUserGesture = !c.autoplay
        s.setSupportMultipleWindows(true)
        s.javaScriptCanOpenWindowsAutomatically = true
        s.setSupportZoom(c.pinchZoom)
        s.builtInZoomControls = c.pinchZoom
        s.displayZoomControls = false
        defaultUA = s.userAgentString
        setUserAgent(c.userAgent)
        WebView.setWebContentsDebuggingEnabled(c.inspectable)
        web.addJavascriptInterface(Bridge(this), "mygoAndroid")
        web.webViewClient = Client()
        web.webChromeClient = Chrome()
        setScripts(c.scripts, c.scriptFlags)
        if (WebViewFeature.isFeatureSupported(WebViewFeature.DOCUMENT_START_SCRIPT)) {
            WebViewCompat.addDocumentStartJavaScript(web, Schemes.bodyScript, setOf("*"))
        }
        // The web view's own background, which the bars around it take,
        // until the app gives one.
        val dark = context.resources.configuration.uiMode and android.content.res.Configuration.UI_MODE_NIGHT_MASK ==
            android.content.res.Configuration.UI_MODE_NIGHT_YES
        fill = if (dark) 0xFF121212.toInt() else android.graphics.Color.WHITE
        frame.addView(web, FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT))
    }

    /** Lays the page out inside the system's bars and above the keyboard. */
    override fun insetsChanged() {
        val c = covered()
        val lp = web.layoutParams as FrameLayout.LayoutParams
        if (lp.leftMargin != c[1] || lp.topMargin != c[0] || lp.rightMargin != c[3] || lp.bottomMargin != maxOf(c[2], c[4])) {
            lp.setMargins(c[1], c[0], c[3], maxOf(c[2], c[4]))
            web.layoutParams = lp
        }
        reportSize()
    }

    override fun setBackground(argb: Int) {
        frame.setBackgroundColor(argb)
        web.setBackgroundColor(argb)
    }

    fun setUserAgent(ua: String) {
        var v = ua.ifEmpty { defaultUA }
        if (suffix.isNotEmpty()) v += " $suffix"
        web.settings.userAgentString = v
    }

    fun setScripts(sources: List<String>, flags: List<String>) {
        for (h in scripts) h.remove()
        scripts = ArrayList()
        if (!WebViewFeature.isFeatureSupported(WebViewFeature.DOCUMENT_START_SCRIPT)) return
        for ((i, src) in sources.withIndex()) {
            val f = flags.getOrElse(i) { "" }
            var js = src
            if (f.contains('e')) js = "document.readyState==='loading'?document.addEventListener('DOMContentLoaded',function(){$js},{once:true}):(function(){$js})();"
            if (!f.contains('a')) js = "if(window===window.top){$js}"
            scripts.add(WebViewCompat.addDocumentStartJavaScript(web, js, setOf("*")))
        }
    }

    fun load(url: String) {
        web.loadUrl(url)
    }

    fun loadHTML(html: String, base: String) {
        web.loadDataWithBaseURL(base.ifEmpty { null }, html, "text/html", "utf-8", null)
    }

    fun callAsync(req: Long, body: String) {
        val js = "(function(){var r=$req;try{var F=Object.getPrototypeOf(async function(){}).constructor;" +
            "new F(${JSONObject.quote(body)})().then(function(v){mygoAndroid.asyncResult(r,String(v),'')}," +
            "function(e){mygoAndroid.asyncResult(r,'',String(e&&e.message||e))})}catch(e){mygoAndroid.asyncResult(r,'',String(e&&e.message||e))}})()"
        web.evaluateJavascript(js, null)
    }

    fun capture(req: Long) {
        if (web.width == 0 || web.height == 0) {
            Go.send(Event.captured(req, ByteArray(0), "mygo: the page shows nothing"))
            return
        }
        val bmp = Bitmap.createBitmap(web.width, web.height, Bitmap.Config.ARGB_8888)
        web.draw(Canvas(bmp))
        val out = ByteArrayOutputStream()
        bmp.compress(Bitmap.CompressFormat.PNG, 100, out)
        bmp.recycle()
        Go.send(Event.captured(req, out.toByteArray(), ""))
    }

    override fun close() {
        web.stopLoading()
        web.removeJavascriptInterface("mygoAndroid")
        web.destroy()
    }

    private fun reportState() {
        Go.send(Event.pageState(id, web.url ?: "", web.title ?: "", web.progress < 100, web.canGoBack(), web.canGoForward()))
        HostImpl.updateBack()
    }

    /** Whether url is one of the app's schemes, served at https://<scheme>.localhost. */
    private fun appHost(url: Uri): Boolean {
        val host = url.host ?: return false
        if (url.scheme != "https" || !host.endsWith(".localhost")) return false
        val rest = host.removeSuffix(".localhost")
        return schemes.any { rest == it || rest.endsWith(".$it") }
    }

    private class Bridge(val w: WebWindow) {
        @JavascriptInterface
        fun postMessage(text: String) = Go.send(Event.message(w.id, text))

        @JavascriptInterface
        fun asyncResult(req: Long, result: String, error: String) = Go.send(Event.asyncResult(req, result, error))

        /** The body of a request to the app's schemes, which Android does not give shouldInterceptRequest. */
        @JavascriptInterface
        fun putBody(id: String, base64: String) = Schemes.putBody(id, base64)
    }

    private inner class Client : WebViewClient() {
        override fun shouldOverrideUrlLoading(view: WebView, r: WebResourceRequest): Boolean {
            val url = r.url
            val scheme = url.scheme?.lowercase()
            if (scheme != "http" && scheme != "https" && scheme != "about" && scheme != "data" && scheme != "blob" && scheme != "javascript") {
                // mailto:, tel:, other apps' links.
                try {
                    (HostImpl.activity ?: context).startActivity(Intent(Intent.ACTION_VIEW, url))
                } catch (_: Exception) {
                }
                return true
            }
            if (!r.isForMainFrame) return false
            val reply = Go.ask(Event.willNavigate(id, url.toString(), true, r.hasGesture(), r.isRedirect, false))
            return reply != null && !WillNavigateReply(reply).allow
        }

        // Android's web view starts a page as it commits it: the page's
        // scripts run after, and the core trusts them by the committed URL.
        override fun onPageStarted(view: WebView, url: String, favicon: Bitmap?) {
            Go.send(Event.navigationStarted(id, url))
            Go.send(Event.navigationCommitted(id, url))
            reportState()
        }

        override fun onPageCommitVisible(view: WebView, url: String) {
            HostImpl.drew = true
        }

        override fun onPageFinished(view: WebView, url: String) {
            reportState()
            Go.send(Event.loadFinished(id))
        }

        override fun doUpdateVisitedHistory(view: WebView, url: String, isReload: Boolean) = reportState()

        override fun onReceivedError(view: WebView, r: WebResourceRequest, e: WebResourceError) {
            if (r.isForMainFrame) Go.send(Event.loadFailed(id, r.url.toString(), e.errorCode.toLong(), e.description.toString()))
        }

        override fun onRenderProcessGone(view: WebView, detail: RenderProcessGoneDetail): Boolean {
            Go.send(Event.processGone(id, if (detail.didCrash()) "crashed" else "killed"))
            return true
        }

        override fun shouldInterceptRequest(view: WebView, r: WebResourceRequest): WebResourceResponse? {
            if (!appHost(r.url)) return null
            return Schemes.request(id, r)
        }
    }

    private inner class Chrome : WebChromeClient() {
        override fun onReceivedTitle(view: WebView, title: String?) = reportState()

        // The page's console, in the app's log, as the inspector shows it.
        override fun onConsoleMessage(m: android.webkit.ConsoleMessage): Boolean {
            val level = when (m.messageLevel()) {
                android.webkit.ConsoleMessage.MessageLevel.ERROR -> android.util.Log.ERROR
                android.webkit.ConsoleMessage.MessageLevel.WARNING -> android.util.Log.WARN
                else -> android.util.Log.INFO
            }
            android.util.Log.println(level, "mygo", "console: ${m.message()} (${m.sourceId()}:${m.lineNumber()})")
            return true
        }

        override fun onCreateWindow(view: WebView, dialog: Boolean, gesture: Boolean, msg: Message): Boolean {
            // The page's new window: its URL comes with the first load of a
            // web view made for it, which the app's window then takes.
            val probe = WebView(view.context)
            probe.webViewClient = object : WebViewClient() {
                override fun shouldOverrideUrlLoading(v: WebView, r: WebResourceRequest): Boolean {
                    val url = r.url.toString()
                    val reply = Go.ask(Event.newWindow(id, url, gesture))
                    if (reply == null || !NewWindowReply(reply).handled) {
                        if (gesture) try {
                            (HostImpl.activity ?: context).startActivity(Intent(Intent.ACTION_VIEW, r.url))
                        } catch (_: Exception) {
                        }
                    }
                    v.destroy()
                    return true
                }
            }
            (msg.obj as WebView.WebViewTransport).webView = probe
            msg.sendToTarget()
            return true
        }

        override fun onCloseWindow(window: WebView) {
            Go.send(Event.closedByPage(id))
        }

        override fun onPermissionRequest(request: PermissionRequest) {
            val kinds = request.resources.mapNotNull {
                when (it) {
                    PermissionRequest.RESOURCE_VIDEO_CAPTURE -> "camera"
                    PermissionRequest.RESOURCE_AUDIO_CAPTURE -> "microphone"
                    else -> null
                }
            }
            val reply = Go.ask(Event.permissionRequested(id, kinds, request.origin.toString()))
            if (reply == null || !PermissionRequestedReply(reply).allow) {
                request.deny()
                return
            }
            // The system's permissions too, which the app's manifest declares.
            val need = kinds.map { if (it == "camera") Manifest.permission.CAMERA else Manifest.permission.RECORD_AUDIO }
                .filter { context.checkSelfPermission(it) != PackageManager.PERMISSION_GRANTED }
            val a = HostImpl.activity
            if (need.isEmpty() || a == null) {
                if (need.isEmpty()) request.grant(request.resources) else request.deny()
                return
            }
            pendingPermission = request
            a.requestPermissions(need.toTypedArray(), ASK_MEDIA)
        }

        // navigator.geolocation: the app's handler says, then the system's
        // permission, which the app's manifest declares.
        override fun onGeolocationPermissionsShowPrompt(origin: String, callback: android.webkit.GeolocationPermissions.Callback) {
            val reply = Go.ask(Event.permissionRequested(id, listOf("geolocation"), origin))
            if (reply == null || !PermissionRequestedReply(reply).allow) {
                callback.invoke(origin, false, false)
                return
            }
            PluginHost.requestPermissions(arrayOf(Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION)) {
                callback.invoke(origin, it, false)
            }
        }

        override fun onShowFileChooser(view: WebView, callback: ValueCallback<Array<Uri>>, params: FileChooserParams): Boolean {
            val a = HostImpl.activity ?: return false
            fileCallback?.onReceiveValue(null)
            fileCallback = callback
            return try {
                @Suppress("DEPRECATION")
                a.startActivityForResult(params.createIntent().putExtra(Intent.EXTRA_ALLOW_MULTIPLE, params.mode == FileChooserParams.MODE_OPEN_MULTIPLE), CHOOSE_FILE)
                true
            } catch (e: Exception) {
                fileCallback = null
                false
            }
        }
    }

    companion object {
        private const val CHOOSE_FILE = 0x6d7b
        private const val ASK_MEDIA = 0x6d7c
        private var fileCallback: ValueCallback<Array<Uri>>? = null
        private var pendingPermission: PermissionRequest? = null

        fun activityResult(requestCode: Int, resultCode: Int, data: Intent?) {
            if (requestCode != CHOOSE_FILE) return
            fileCallback?.onReceiveValue(WebChromeClient.FileChooserParams.parseResult(resultCode, data))
            fileCallback = null
        }

        fun permissionsResult(requestCode: Int, results: IntArray) {
            if (requestCode != ASK_MEDIA) return
            val r = pendingPermission ?: return
            pendingPermission = null
            if (results.isNotEmpty() && results.all { it == PackageManager.PERMISSION_GRANTED }) r.grant(r.resources) else r.deny()
        }
    }
}

/**
 * The app's schemes: Android's web view loads none of its own, so they
 * are served at https://<scheme>.localhost, whose requests go to Go
 * (shouldInterceptRequest, on a thread of the web view's), which answers
 * with SchemeRespond, SchemeData and SchemeFinish. Android gives no body
 * of a request: fetch and XMLHttpRequest put theirs aside (bodyScript).
 */
object Schemes {
    private val next = AtomicLong()
    private val pending = java.util.concurrent.ConcurrentHashMap<Long, Pending>()
    private val bodies = java.util.concurrent.ConcurrentHashMap<String, ByteArray>()
    private const val BODY_HEADER = "X-MyGo-Body"

    fun putBody(id: String, base64: String) {
        if (bodies.size > 64) bodies.clear() // bodies of requests that never came
        bodies[id] = android.util.Base64.decode(base64, android.util.Base64.NO_WRAP)
    }

    /**
     * Puts the bodies of fetch's and XMLHttpRequest's requests to the app's
     * schemes (https URLs of the hosts under localhost) aside through putBody, and has the
     * requests name them in a header, which request takes them back by.
     */
    val bodyScript = """
(() => {
  if (!window.mygoAndroid || window.__mygoBodies) return;
  window.__mygoBodies = true;
  const app = (u) => { try { const x = new URL(u, location.href); return x.protocol === 'https:' && x.hostname.endsWith('.localhost'); } catch (e) { return false; } };
  const put = (buf) => {
    const b = new Uint8Array(buf);
    let s = '';
    for (let i = 0; i < b.length; i += 0x8000) s += String.fromCharCode.apply(null, b.subarray(i, i + 0x8000));
    const id = Date.now().toString(36) + Math.random().toString(36).slice(2);
    window.mygoAndroid.putBody(id, btoa(s));
    return id;
  };
  const fetch0 = window.fetch;
  window.fetch = async function (input, init) {
    const req = new Request(input, init);
    if (!app(req.url) || req.method === 'GET' || req.method === 'HEAD' || req.body === null) return fetch0.call(this, input, init);
    const headers = new Headers(req.headers);
    headers.set('$BODY_HEADER', put(await req.arrayBuffer()));
    return fetch0.call(this, req.url, { method: req.method, headers, credentials: req.credentials, cache: req.cache,
      redirect: req.redirect, referrer: req.referrer, signal: req.signal, mode: req.mode });
  };
  const open0 = XMLHttpRequest.prototype.open, send0 = XMLHttpRequest.prototype.send;
  XMLHttpRequest.prototype.open = function (method, url, async) {
    this.__mygo = { method: String(method).toUpperCase(), url: String(url), async: async !== false };
    return open0.apply(this, arguments);
  };
  XMLHttpRequest.prototype.send = function (body) {
    const m = this.__mygo;
    if (!m || !m.async || body == null || m.method === 'GET' || m.method === 'HEAD' || !app(m.url)) return send0.call(this, body);
    const r = new Response(body), type = r.headers.get('content-type');
    r.arrayBuffer().then((buf) => {
      if (type) try { this.setRequestHeader('Content-Type', type); } catch (e) {}
      this.setRequestHeader('$BODY_HEADER', put(buf));
      send0.call(this);
    });
  };
})();
"""

    private class Pending {
        val responded = CountDownLatch(1)
        var status = 200
        var header = HashMap<String, String>()
        val body = BodyStream()
    }

    fun request(window: Long, r: WebResourceRequest): WebResourceResponse {
        val req = next.incrementAndGet()
        val p = Pending()
        pending[req] = p
        val header = ArrayList<String>()
        var body: ByteArray? = null
        for ((k, v) in r.requestHeaders) {
            if (k.equals(BODY_HEADER, true)) {
                body = bodies.remove(v)
                continue
            }
            header.add(k)
            header.add(v)
        }
        Go.send(Event.schemeRequest(window, req, r.method, r.url.toString(), header, body ?: ByteArray(0), body != null))
        if (!p.responded.await(60, TimeUnit.SECONDS)) {
            pending.remove(req)
            Go.send(Event.schemeCancel(req))
            return WebResourceResponse("text/plain", "utf-8", 504, "Gateway Timeout", emptyMap(), "".byteInputStream())
        }
        val type = p.header.entries.firstOrNull { it.key.equals("Content-Type", true) }?.value ?: "application/octet-stream"
        val mime = type.substringBefore(';').trim()
        val charset = Regex("charset=([^;]+)", RegexOption.IGNORE_CASE).find(type)?.groupValues?.get(1)?.trim()
        // A WebResourceResponse takes no redirect.
        val status = if (p.status in 300..399) 200 else p.status
        return WebResourceResponse(mime, charset, status, reason(status), p.header, p.body)
    }

    private fun reason(status: Int) = when (status) {
        200 -> "OK"; 204 -> "No Content"; 206 -> "Partial Content"; 304 -> "Not Modified"
        400 -> "Bad Request"; 403 -> "Forbidden"; 404 -> "Not Found"; 500 -> "Internal Server Error"
        else -> "Status $status"
    }

    fun respond(c: SchemeRespondCall) {
        val p = pending[c.req] ?: return
        p.status = c.status.toInt()
        var i = 0
        while (i + 1 < c.header.size) {
            val k = c.header[i]
            p.header[k] = p.header[k]?.let { it + ", " + c.header[i + 1] } ?: c.header[i + 1]
            i += 2
        }
        p.responded.countDown()
    }

    fun data(req: Long, data: ByteArray) {
        pending[req]?.body?.queue?.put(data)
    }

    fun finish(req: Long, error: String?) {
        val p = pending.remove(req) ?: return
        if (p.responded.count > 0) {
            p.status = 500
            p.responded.countDown()
        }
        p.body.queue.put(ByteArray(0))
    }

    /** The body of a response, as Go writes it; an empty piece ends it. */
    private class BodyStream : InputStream() {
        val queue = LinkedBlockingQueue<ByteArray>()
        private var cur = ByteArray(0)
        private var at = 0
        private var done = false

        private fun fill(): Boolean {
            while (at >= cur.size) {
                if (done) return false
                cur = queue.take()
                at = 0
                if (cur.isEmpty()) {
                    done = true
                    return false
                }
            }
            return true
        }

        override fun read(): Int = if (fill()) cur[at++].toInt() and 0xff else -1

        override fun read(b: ByteArray, off: Int, len: Int): Int {
            if (len == 0) return 0
            if (!fill()) return -1
            val n = minOf(len, cur.size - at)
            System.arraycopy(cur, at, b, off, n)
            at += n
            return n
        }
    }
}
